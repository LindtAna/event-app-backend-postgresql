package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// Erstellt einen isolierten Gin-Router, auf dem die CORS-Middleware
// vor dem angegebenen Handler ausgeführt wird (Testen ohne den vollständigen Anwendungsserver)
func newCORSTestRouter(app *application, handler gin.HandlerFunc) *gin.Engine {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(app.CORSMiddleware())
	router.GET("/test", handler)

	return router
}

// Prüft die Normalisierung und den Abgleich eingehender Origins mit der konfigurierten Allowlist
func TestIsAllowedOrigin(t *testing.T) {
	app := &application{
		clientOrigin: "https://event-app.vercel.app, http://localhost:5173",
	}

	tests := []struct {
		name   string
		origin string
		want   bool
	}{
		{
			name:   "Allowed production origin",
			origin: "https://event-app.vercel.app",
			want:   true,
		},
		{
			name:   "Allowed local origin",
			origin: "http://localhost:5173",
			want:   true,
		},
		{
			name:   "Origin with trailing slash",
			origin: "https://event-app.vercel.app/",
			want:   true,
		},
		{
			name:   "Origin with different letter case",
			origin: "HTTPS://EVENT-APP.VERCEL.APP",
			want:   true,
		},
		{
			name:   "Unconfigured origin",
			origin: "https://unknown.example.com",
			want:   false,
		},
		{
			name:   "Empty origin",
			origin: "",
			want:   false,
		},
	}

	// Alle Normalisierungs- und Allowlist-Fälle verwenden dieselbe Erwartungsprüfung
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := app.isAllowedOrigin(tt.origin)

			if got != tt.want {
				t.Errorf(
					"isAllowedOrigin(%q) = %v, want %v",
					tt.origin,
					got,
					tt.want,
				)
			}
		})
	}
}

// Prüft, ob eine erlaubte Origin die erwarteten
// CORS-Header erhält und der eigentliche Handler ausgeführt wird
func TestCORSMiddlewareAllowsOrigin(t *testing.T) {
	app := &application{
		clientOrigin: "https://event-app.vercel.app",
	}

	router := newCORSTestRouter(app, func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Origin", "https://event-app.vercel.app")

	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf(
			"expected status %d, got %d",
			http.StatusOK,
			recorder.Code,
		)
	}

	if got := recorder.Header().Get("Access-Control-Allow-Origin"); got != "https://event-app.vercel.app" {
		t.Errorf("unexpected Access-Control-Allow-Origin: %q", got)
	}

	if got := recorder.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Errorf("expected Allow-Credentials to be true, got %q", got)
	}

	if got := recorder.Header().Get("Vary"); !strings.Contains(got, "Origin") {
		t.Errorf("expected Vary header to contain Origin, got %q", got)
	}
}

// Prüft, ob nicht konfigurierte Origins mit HTTP 403 abgewiesen werden,
// bevor der nachgelagerte Handler ausgeführt wird
func TestCORSMiddlewareRejectsOrigin(t *testing.T) {
	app := &application{
		clientOrigin: "https://event-app.vercel.app",
	}

	handlerCalled := false

	router := newCORSTestRouter(app, func(c *gin.Context) {
		handlerCalled = true
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Origin", "https://unknown.example.com")

	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf(
			"expected status %d, got %d",
			http.StatusForbidden,
			recorder.Code,
		)
	}

	// Zusätzlich zum Status wird geprüft, dass die Middleware die Request-Kette abbricht
	if handlerCalled {
		t.Error("handler must not be called for a rejected origin")
	}
}

// TestCORSMiddlewareHandlesPreflight prüft die Behandlung einer CORS-Preflight-Anfrage
// Eine OPTIONS-Anfrage muss die benötigten Header liefern, ohne den regulären Handler aufzurufen
func TestCORSMiddlewareHandlesPreflight(t *testing.T) {
	app := &application{
		clientOrigin: "https://event-app.vercel.app",
	}

	handlerCalled := false

	router := newCORSTestRouter(app, func(c *gin.Context) {
		handlerCalled = true
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodOptions, "/test", nil)
	req.Header.Set("Origin", "https://event-app.vercel.app")
	req.Header.Set("Access-Control-Request-Method", "POST")
	req.Header.Set("Access-Control-Request-Headers", "Authorization, Content-Type")

	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf(
			"expected status %d, got %d",
			http.StatusNoContent,
			recorder.Code,
		)
	}

	if got := recorder.Header().Get("Access-Control-Allow-Origin"); got != "https://event-app.vercel.app" {
		t.Errorf("unexpected Allow-Origin header: %q", got)
	}

	if got := recorder.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Errorf("expected Allow-Credentials to be true, got %q", got)
	}

	if got := recorder.Header().Get("Access-Control-Allow-Methods"); !strings.Contains(got, "POST") {
		t.Errorf("expected Allow-Methods to contain POST, got %q", got)
	}

	if handlerCalled {
		t.Error("regular handler must not be called for an OPTIONS preflight request")
	}
}

// stellt sicher, dass Requests ohne Origin-Header
// nicht durch CORS blockiert werden
func TestCORSMiddlewareAllowsRequestsWithoutOrigin(t *testing.T) {
	app := &application{
		clientOrigin: "https://event-app.vercel.app",
	}

	handlerCalled := false

	router := newCORSTestRouter(app, func(c *gin.Context) {
		handlerCalled = true
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)

	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}

	if !handlerCalled {
		t.Error("handler should be called when Origin is absent")
	}
}

// Prüft verschiedene fehlerhafte
// Authorization-Header. In keinem dieser Fälle darf der geschützte Handler laufen
func TestAuthMiddlewareRejectsInvalidAuthorization(t *testing.T) {
	app := newTestApplication()

	tests := []struct {
		name   string
		header string
	}{
		{
			name:   "Missing authorization",
			header: "",
		},
		{
			name:   "Wrong authorization scheme",
			header: "Basic some-token",
		},
		{
			name:   "Missing token",
			header: "Bearer",
		},
		{
			name:   "Too many authorization parts",
			header: "Bearer token extra",
		},
		{
			name:   "Invalid JWT",
			header: "Bearer invalid-token",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handlerCalled := false

			gin.SetMode(gin.TestMode)

			router := gin.New()
			router.Use(app.AuthMiddleware())
			router.GET("/protected", func(c *gin.Context) {
				handlerCalled = true
				c.Status(http.StatusOK)
			})

			req := httptest.NewRequest(http.MethodGet, "/protected", nil)

			if tt.header != "" {
				req.Header.Set("Authorization", tt.header)
			}

			recorder := httptest.NewRecorder()

			router.ServeHTTP(recorder, req)

			if recorder.Code != http.StatusUnauthorized {
				t.Errorf(
					"expected status %d, got %d",
					http.StatusUnauthorized,
					recorder.Code,
				)
			}

			if handlerCalled {
				t.Error("protected handler must not be called")
			}
		})
	}
}

// Prüft verschiedene fehlerhafte Authorization-Header
// In keinem dieser Fälle darf der geschützte Handler laufen
func TestAuthMiddlewareRejectsRefreshToken(t *testing.T) {
	app := newTestApplication()

	refreshToken, err := app.generateRefreshToken(123)
	if err != nil {
		t.Fatalf("generateRefreshToken() returned an error: %v", err)
	}

	handlerCalled := false

	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(app.AuthMiddleware())
	router.GET("/protected", func(c *gin.Context) {
		handlerCalled = true
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+refreshToken)

	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusUnauthorized {
		t.Errorf(
			"expected status %d, got %d",
			http.StatusUnauthorized,
			recorder.Code,
		)
	}

	if handlerCalled {
		t.Error("protected handler must not be called with a Refresh Token")
	}
}

// ----------------------------------------
// Tests der Owner-Autorisierungsmiddleware
// ----------------------------------------

// Prüft, dass ausschließlich Benutzer mit der Rolle owner
// auf geschützte Verwaltungsrouten zugreifen können
func TestOwnerOnlyMiddleware(t *testing.T) {
	tests := []struct {
		name                string
		setRole             bool
		roleValue           interface{}
		expectedStatus      int
		expectedError       string
		expectedHandlerCall bool
	}{
		{
			name:                "Owner is allowed",
			setRole:             true,
			roleValue:           "owner",
			expectedStatus:      http.StatusNoContent,
			expectedHandlerCall: true,
		},
		{
			name:           "Member is forbidden",
			setRole:        true,
			roleValue:      "member",
			expectedStatus: http.StatusForbidden,
			expectedError:  "Only the tenant owner can manage invitations",
		},
		{
			name:           "Empty role is unauthorized",
			setRole:        true,
			roleValue:      "",
			expectedStatus: http.StatusUnauthorized,
			expectedError:  "Unauthorized",
		},
		{
			name:           "Missing role is unauthorized",
			setRole:        false,
			expectedStatus: http.StatusUnauthorized,
			expectedError:  "Unauthorized",
		},
		{
			name:           "Role with incorrect type is unauthorized",
			setRole:        true,
			roleValue:      123,
			expectedStatus: http.StatusUnauthorized,
			expectedError:  "Unauthorized",
		},
		{
			name:           "Unknown role is forbidden",
			setRole:        true,
			roleValue:      "admin",
			expectedStatus: http.StatusForbidden,
			expectedError:  "Only the tenant owner can manage invitations",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)

			app := &application{}
			router := gin.New()

			router.Use(func(c *gin.Context) {
				if tt.setRole {
					c.Set("role", tt.roleValue)
				}
				c.Next()
			})

			router.Use(app.OwnerOnlyMiddleware())

			handlerCalled := false

			router.GET("/protected", func(c *gin.Context) {
				handlerCalled = true
				c.Status(http.StatusNoContent)
			})

			req := httptest.NewRequest(http.MethodGet, "/protected", nil)
			recorder := httptest.NewRecorder()

			router.ServeHTTP(recorder, req)

			if recorder.Code != tt.expectedStatus {
				t.Errorf(
					"expected status %d, got %d",
					tt.expectedStatus,
					recorder.Code,
				)
			}

			if handlerCalled != tt.expectedHandlerCall {
				t.Errorf(
					"expected handler called=%t, got %t",
					tt.expectedHandlerCall,
					handlerCalled,
				)
			}

			if tt.expectedError != "" &&
				!strings.Contains(recorder.Body.String(), tt.expectedError) {
				t.Errorf(
					"expected response body to contain %q, got %q",
					tt.expectedError,
					recorder.Body.String(),
				)
			}
		})
	}
}

// --------------------------------------------------------
// Tests des Event-Request-Kontexts und der ID-Validierung
// --------------------------------------------------------

// Prüft, ob die User- und Tenant-ID korrekt und mit
// dem erwarteten Datentyp aus dem Gin-Kontext gelesen werden können
func TestEventRequestScope(t *testing.T) {
	tests := []struct {
		name         string
		userID       interface{}
		tenantID     interface{}
		setUserID    bool
		setTenantID  bool
		wantUserID   int
		wantTenantID int
		wantOK       bool
	}{
		{
			name:         "Valid user and tenant IDs",
			userID:       10,
			tenantID:     20,
			setUserID:    true,
			setTenantID:  true,
			wantUserID:   10,
			wantTenantID: 20,
			wantOK:       true,
		},
		{
			name:        "Missing user ID",
			tenantID:    20,
			setTenantID: true,
		},
		{
			name:      "Missing tenant ID",
			userID:    10,
			setUserID: true,
		},
		{
			name:        "User ID has incorrect type",
			userID:      "10",
			tenantID:    20,
			setUserID:   true,
			setTenantID: true,
		},
		{
			name:        "Tenant ID has incorrect type",
			userID:      10,
			tenantID:    "20",
			setUserID:   true,
			setTenantID: true,
		},
		{
			name:        "User ID is zero",
			userID:      0,
			tenantID:    20,
			setUserID:   true,
			setTenantID: true,
		},
		{
			name:        "Tenant ID is zero",
			userID:      10,
			tenantID:    0,
			setUserID:   true,
			setTenantID: true,
		},
		{
			name:        "Negative user ID",
			userID:      -10,
			tenantID:    20,
			setUserID:   true,
			setTenantID: true,
		},
		{
			name:        "Negative tenant ID",
			userID:      10,
			tenantID:    -20,
			setUserID:   true,
			setTenantID: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)

			c, _ := gin.CreateTestContext(httptest.NewRecorder())

			// Die Flags unterscheiden zwischen einem fehlenden Kontextwert
			// und einem vorhandenen Wert, der beispielsweise den falschen Typ hat
			if tt.setUserID {
				c.Set("userId", tt.userID)
			}
			if tt.setTenantID {
				c.Set("tenantId", tt.tenantID)
			}

			userID, tenantID, ok := eventRequestScope(c)

			if userID != tt.wantUserID {
				t.Errorf("expected userID %d, got %d", tt.wantUserID, userID)
			}
			if tenantID != tt.wantTenantID {
				t.Errorf("expected tenantID %d, got %d", tt.wantTenantID, tenantID)
			}
			if ok != tt.wantOK {
				t.Errorf("expected ok=%t, got %t", tt.wantOK, ok)
			}
		})
	}
}

// Prüft, ob ausschließlich positive Ganzzahlen als IDs
// akzeptiert werden und ungültige Eingaben einen Fehler zurückgeben
func TestParsePositiveID(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantID  int
		wantErr bool
	}{
		{
			name:   "Valid positive ID",
			value:  "123",
			wantID: 123,
		},
		{
			name:    "Zero",
			value:   "0",
			wantErr: true,
		},
		{
			name:    "Negative ID",
			value:   "-123",
			wantErr: true,
		},
		{
			name:    "Non-numeric ID",
			value:   "abc",
			wantErr: true,
		},
		{
			name:    "Empty value",
			value:   "",
			wantErr: true,
		},
		{
			name:    "Decimal value",
			value:   "12.5",
			wantErr: true,
		},
		{
			name:    "Integer overflow",
			value:   "999999999999999999999999999999999",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotID, err := parsePositiveID(tt.value)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected an error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if gotID != tt.wantID {
				t.Errorf("expected ID %d, got %d", tt.wantID, gotID)
			}
		})
	}
}
