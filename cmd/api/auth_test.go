package main

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

// Erstellt eine minimale App-Instanz für Unit Tests
// Das ausreichend lange, feste Test-Secret macht die JWT-Prüfungen reproduzierbar;
// eine DB-Verbindung wird für diese Tests nicht benötigt
func newTestApplication() *application {
	return &application{
		jwtSecret: strings.Repeat("a", 32),
	}
}

// Prüft, ob ein Access Token erzeugt und
// anschließend mit den erwarteten Claims wieder eingelesen werden kann
func TestGenerateAndParseAccessToken(t *testing.T) {
	app := newTestApplication()

	const userID = 123

	token, err := app.generateAccessToken(userID)
	if err != nil {
		t.Fatalf("generateAccessToken() returned an error: %v", err)
	}

	if token == "" {
		t.Fatal("generateAccessToken() returned an empty token")
	}

	claims, err := app.parseAuthToken(token, accessTokenType)
	if err != nil {
		t.Fatalf("parseAuthToken() returned an error: %v", err)
	}

	if claims.UserID != userID {
		t.Errorf("expected UserID %d, got %d", userID, claims.UserID)
	}

	if claims.TokenType != accessTokenType {
		t.Errorf(
			"expected TokenType %q, got %q",
			accessTokenType,
			claims.TokenType,
		)
	}
}

// Prüft die Erzeugung und das anschließende
// Einlesen eines Refresh Tokens einschließlich User-ID und Token-Typ
func TestGenerateAndParseRefreshToken(t *testing.T) {
	app := newTestApplication()

	const userID = 456

	token, err := app.generateRefreshToken(userID)
	if err != nil {
		t.Fatalf("generateRefreshToken() returned an error: %v", err)
	}

	claims, err := app.parseAuthToken(token, refreshTokenType)
	if err != nil {
		t.Fatalf("parseAuthToken() returned an error: %v", err)
	}

	if claims.UserID != userID {
		t.Errorf("expected UserID %d, got %d", userID, claims.UserID)
	}

	if claims.TokenType != refreshTokenType {
		t.Errorf(
			"expected TokenType %q, got %q",
			refreshTokenType,
			claims.TokenType,
		)
	}
}

// Stellt sicher, dass ein Access Token
// nicht an einer Stelle akzeptiert wird, an der ein Refresh Token erwartet wird
func TestParseAuthTokenRejectsWrongTokenType(t *testing.T) {
	app := newTestApplication()

	token, err := app.generateAccessToken(123)
	if err != nil {
		t.Fatalf("generateAccessToken() returned an error: %v", err)
	}

	// Der erwartete Token-Typ wird bewusst auf Refresh gesetzt,
	// um die Trennung der beiden Token-Arten zu überprüfen
	_, err = app.parseAuthToken(token, refreshTokenType)

	if err == nil {
		t.Fatal("expected an error when parsing an Access Token as a Refresh Token")
	}
}

// Prüft, dass ein Token mit einem anderen Signatur-Secret
// nicht von einer App-Instanz akzeptiert wird
func TestParseAuthTokenRejectsInvalidSignature(t *testing.T) {
	app := newTestApplication()
	anotherApp := &application{
		// Ein abweichendes Secret simuliert einen unbekannten bzw. falschen Signaturschlüssel
		jwtSecret: strings.Repeat("b", 32),
	}

	token, err := app.generateAccessToken(123)
	if err != nil {
		t.Fatalf("generateAccessToken() returned an error: %v", err)
	}

	// The second application uses a different signing secret.
	_, err = anotherApp.parseAuthToken(token, accessTokenType)

	if err == nil {
		t.Fatal("expected an error for a token with an invalid signature")
	}
}

// Prüft, ob ein bereits abgelaufener JWT beim Einlesen zurückgewiesen wird
func TestParseAuthTokenRejectsExpiredToken(t *testing.T) {
	app := newTestApplication()

	// Eine negative Laufzeit erzeugt gezielt einen Token mit Ablaufzeit in der Vergangenheit
	token, err := app.generateToken(
		123,
		accessTokenType,
		-time.Minute,
	)
	if err != nil {
		t.Fatalf("generateToken() returned an error: %v", err)
	}

	_, err = app.parseAuthToken(token, accessTokenType)

	if err == nil {
		t.Fatal("expected an error for an expired token")
	}
}

// Stellt sicher, dass User-IDs kleiner oder gleich null nicht
// zur Erstellung eines Tokens verwendet werden können
func TestGenerateTokenRejectsInvalidUserID(t *testing.T) {
	app := newTestApplication()

	for _, userID := range []int{0, -1} {
		token, err := app.generateAccessToken(userID)

		if err == nil {
			t.Errorf("expected an error for UserID %d", userID)
		}

		if token != "" {
			t.Errorf("expected an empty token for UserID %d", userID)
		}
	}
}

// --------------------------------
// Tests der Refresh-Token-Cookies
// --------------------------------

// Erstellt einen künstlichen Gin-Request samt Response Recorder.
// Über die Parameter lassen sich Origin, Reverse-Proxy-Header und eine direkte TLS-Verbindung simulieren,
// ohne einen laufenden HTTP-Server zu starten
func newCookieTestContext(
	clientOrigin string,
	requestOrigin string,
	forwardedProto string,
	useTLS bool,
) (*application, *gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	req := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/auth/refresh",
		nil,
	)

	if requestOrigin != "" {
		req.Header.Set("Origin", requestOrigin)
	}

	if forwardedProto != "" {
		req.Header.Set("X-Forwarded-Proto", forwardedProto)
	}

	if useTLS {
		req.TLS = &tls.ConnectionState{}
	}

	c.Request = req

	app := &application{
		clientOrigin: clientOrigin,
	}

	return app, c, recorder
}

// Prüft die sicherheitsrelevanten Attribute des Refresh-Cookies
// für lokale HTTP- und verschiedene HTTPS-Szenarien
func TestSetRefreshTokenCookieAttributes(t *testing.T) {
	tests := []struct {
		name           string
		clientOrigin   string
		requestOrigin  string
		forwardedProto string
		useTLS         bool
		wantSecure     bool
		wantSameSite   http.SameSite
	}{
		{
			name:          "HTTP local development",
			clientOrigin:  "http://localhost:5173",
			requestOrigin: "http://localhost:5173",
			wantSecure:    false,
			wantSameSite:  http.SameSiteLaxMode,
		},
		{
			name:          "HTTPS configured origin",
			clientOrigin:  "https://frontend.example.com",
			requestOrigin: "https://frontend.example.com",
			wantSecure:    true,
			wantSameSite:  http.SameSiteNoneMode,
		},
		{
			name:           "HTTPS forwarded by reverse proxy",
			clientOrigin:   "http://localhost:5173",
			requestOrigin:  "http://localhost:5173",
			forwardedProto: "https",
			wantSecure:     true,
			wantSameSite:   http.SameSiteNoneMode,
		},
		{
			name:          "HTTPS direct TLS connection",
			clientOrigin:  "http://localhost:5173",
			requestOrigin: "http://localhost:5173",
			useTLS:        true,
			wantSecure:    true,
			wantSameSite:  http.SameSiteNoneMode,
		},
		{
			name:          "Untrusted HTTPS origin",
			clientOrigin:  "http://localhost:5173",
			requestOrigin: "https://unknown.example.com",
			wantSecure:    false,
			wantSameSite:  http.SameSiteLaxMode,
		},
		{
			name:         "HTTPS client origin without request origin",
			clientOrigin: "https://frontend.example.com",
			wantSecure:   true,
			wantSameSite: http.SameSiteNoneMode,
		},
	}

	// Tabellengesteuerte Tests halten die Prüfung einheitlich
	// Decken mehrere Kombinationen der Origin- und Transportinformationen ab
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app, c, recorder := newCookieTestContext(
				tt.clientOrigin,
				tt.requestOrigin,
				tt.forwardedProto,
				tt.useTLS,
			)

			app.setRefreshTokenCookie(c, "test-refresh-token")

			cookies := recorder.Result().Cookies()

			if len(cookies) != 1 {
				t.Fatalf("expected 1 cookie, got %d", len(cookies))
			}

			cookie := cookies[0]

			if cookie.Name != "refreshToken" {
				t.Errorf("expected cookie name refreshToken, got %q", cookie.Name)
			}

			if cookie.Value != "test-refresh-token" {
				t.Errorf("unexpected cookie value: %q", cookie.Value)
			}

			if cookie.Path != "/api/v1/auth" {
				t.Errorf("expected path /api/v1/auth, got %q", cookie.Path)
			}

			if cookie.Domain != "" {
				t.Errorf("expected host-only cookie without Domain, got %q", cookie.Domain)
			}

			if !cookie.HttpOnly {
				t.Error("expected HttpOnly to be true")
			}

			if cookie.Secure != tt.wantSecure {
				t.Errorf("expected Secure=%t, got %t", tt.wantSecure, cookie.Secure)
			}

			if cookie.SameSite != tt.wantSameSite {
				t.Errorf(
					"expected SameSite=%v, got %v",
					tt.wantSameSite,
					cookie.SameSite,
				)
			}

			expectedMaxAge := int(refreshTokenTTL.Seconds())
			if cookie.MaxAge != expectedMaxAge {
				t.Errorf(
					"expected MaxAge=%d, got %d",
					expectedMaxAge,
					cookie.MaxAge,
				)
			}
		})
	}
}

// Prüft, ob das Refresh-Cookie mit denselben relevanten Attributen gelöscht wird,
// mit denen es gesetzt wurde
func TestClearRefreshTokenCookieAttributes(t *testing.T) {
	app, c, recorder := newCookieTestContext(
		"https://frontend.example.com",
		"https://frontend.example.com",
		"",
		false,
	)

	app.clearRefreshTokenCookie(c)

	cookies := recorder.Result().Cookies()

	if len(cookies) != 1 {
		t.Fatalf("expected 1 cookie, got %d", len(cookies))
	}

	cookie := cookies[0]

	if cookie.Name != "refreshToken" {
		t.Errorf("expected cookie name refreshToken, got %q", cookie.Name)
	}

	if cookie.Value != "" {
		t.Errorf("expected empty cookie value, got %q", cookie.Value)
	}

	if cookie.MaxAge != -1 {
		t.Errorf("expected MaxAge=-1 to delete cookie, got %d", cookie.MaxAge)
	}

	if cookie.Path != "/api/v1/auth" {
		t.Errorf("expected path /api/v1/auth, got %q", cookie.Path)
	}

	if !cookie.HttpOnly {
		t.Error("expected HttpOnly to be true")
	}

	if !cookie.Secure {
		t.Error("expected Secure to be true for HTTPS")
	}

	if cookie.SameSite != http.SameSiteNoneMode {
		t.Errorf("expected SameSite=None, got %v", cookie.SameSite)
	}

	if cookie.Domain != "" {
		t.Errorf("expected host-only cookie without Domain, got %q", cookie.Domain)
	}
}
