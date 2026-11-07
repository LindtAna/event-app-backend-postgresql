//go:build integration

package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"testing"
	"time"

	"event-app/internal/database"

	"golang.org/x/crypto/bcrypt"
)

const integrationTestPassword = "IntegrationPassword123!"

type integrationAuthCredentials struct {
	User     database.User
	Email    string
	Password string
}

// sendet einen HTTP-Request mit JSON-Daten an den Testserver
func integrationDoJSON(
	t *testing.T,
	env *integrationTestEnvironment,
	method string,
	path string,
	payload interface{},
	cookie *http.Cookie,
) *http.Response {
	t.Helper()

	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Failed to encode JSON request: %v", err)
	}

	req, err := http.NewRequest(
		method,
		env.server.URL+path,
		bytes.NewReader(body),
	)
	if err != nil {
		t.Fatalf("Failed to create HTTP request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")

	if cookie != nil {
		req.AddCookie(cookie)
	}

	response, err := env.server.Client().Do(req)
	if err != nil {
		t.Fatalf("Failed to send HTTP request: %v", err)
	}

	return response
}

// prüft den erwarteten HTTP-Status und gibt den Response-Body bei einem Fehler aus
func integrationRequireStatus(
	t *testing.T,
	response *http.Response,
	expectedStatus int,
) {
	t.Helper()

	if response.StatusCode != expectedStatus {
		t.Fatalf(
			"Expected HTTP status %d, got %d",
			expectedStatus,
			response.StatusCode,
		)
	}
}

// erstellt einen Owner über den tatsächlichen HTTP-Endpunkt
// Der zugehörige Tenant wird nach dem Test automatisch entfernt
func integrationRegisterOwner(
	t *testing.T,
	env *integrationTestEnvironment,
) integrationAuthCredentials {
	t.Helper()

	uniqueID := strconv.FormatInt(time.Now().UnixNano(), 10)

	credentials := integrationAuthCredentials{
		Email:    fmt.Sprintf("owner-%s@example.test", uniqueID),
		Password: integrationTestPassword,
	}

	companyName := fmt.Sprintf("Integration Test %s", uniqueID)

	response := integrationDoJSON(
		t,
		env,
		http.MethodPost,
		"/api/v1/auth/register",
		map[string]string{
			"registrationType": "owner",
			"email":            credentials.Email,
			"password":         credentials.Password,
			"name":             "Integration Owner",
			"companyName":      companyName,
		},
		nil,
	)
	defer response.Body.Close()

	integrationRequireStatus(t, response, http.StatusCreated)

	if err := json.NewDecoder(response.Body).Decode(&credentials.User); err != nil {
		t.Fatalf("Failed to decode owner registration response: %v", err)
	}

	if credentials.User.Id <= 0 {
		t.Fatal("Expected a valid user ID after owner registration")
	}

	if credentials.User.TenantID <= 0 {
		t.Fatal("Expected a valid tenant ID after owner registration")
	}

	if credentials.User.Role != "owner" {
		t.Errorf(
			"Expected role %q, got %q",
			"owner",
			credentials.User.Role,
		)
	}

	// Den erstellten Tenant einschließlich seiner abhängigen Datensätze entfernen
	tenantID := credentials.User.TenantID

	t.Cleanup(func() {
		_, err := env.db.Exec(
			"DELETE FROM tenants WHERE id = $1",
			tenantID,
		)
		if err != nil {
			t.Errorf("Failed to clean up integration test tenant: %v", err)
		}
	})

	return credentials
}

// führt einen Login durch und prüft die zurückgegebenen Authentifizierungsdaten
func integrationLogin(
	t *testing.T,
	env *integrationTestEnvironment,
	credentials integrationAuthCredentials,
) (*authResponse, *http.Cookie) {
	t.Helper()

	response := integrationDoJSON(
		t,
		env,
		http.MethodPost,
		"/api/v1/auth/login",
		map[string]string{
			"email":    credentials.Email,
			"password": credentials.Password,
		},
		nil,
	)
	defer response.Body.Close()

	integrationRequireStatus(t, response, http.StatusOK)

	var result authResponse

	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to decode login response: %v", err)
	}

	if result.AccessToken == "" {
		t.Fatal("Expected a non-empty access token")
	}

	if result.User.Id != credentials.User.Id {
		t.Errorf(
			"Expected user ID %d, got %d",
			credentials.User.Id,
			result.User.Id,
		)
	}

	if result.User.Password != "" {
		t.Error("The password hash must not be included in the response")
	}

	// Der zurückgegebene Access Token muss als solcher gültig sein
	claims, err := env.app.parseAuthToken(
		result.AccessToken,
		accessTokenType,
	)
	if err != nil {
		t.Fatalf("Returned access token is invalid: %v", err)
	}

	if claims.UserID != credentials.User.Id {
		t.Errorf(
			"Expected token user ID %d, got %d",
			credentials.User.Id,
			claims.UserID,
		)
	}

	cookies := response.Cookies()

	var refreshCookie *http.Cookie

	for _, cookie := range cookies {
		if cookie.Name == "refreshToken" {
			refreshCookie = cookie
			break
		}
	}

	if refreshCookie == nil {
		t.Fatal("Expected a refreshToken cookie")
	}

	if !refreshCookie.HttpOnly {
		t.Error("Expected the refreshToken cookie to be HttpOnly")
	}

	if refreshCookie.Path != "/api/v1/auth" {
		t.Errorf(
			"Expected refresh cookie path %q, got %q",
			"/api/v1/auth",
			refreshCookie.Path,
		)
	}

	// Der Refresh Token muss den erwarteten Token-Typ und die richtige User-ID enthalten
	refreshClaims, err := env.app.parseAuthToken(
		refreshCookie.Value,
		refreshTokenType,
	)
	if err != nil {
		t.Fatalf("Returned refresh token is invalid: %v", err)
	}

	if refreshClaims.UserID != credentials.User.Id {
		t.Errorf(
			"Expected refresh token user ID %d, got %d",
			credentials.User.Id,
			refreshClaims.UserID,
		)
	}

	return &result, refreshCookie
}

// prüft die Registrierung eines Owners einschließlich Tenant-Erstellung und sicherer Passwortspeicherung.
func TestIntegrationRegisterOwner(t *testing.T) {
	env := newIntegrationTestEnvironment(t)

	credentials := integrationRegisterOwner(t, env)

	if credentials.User.Status != "active" {
		t.Errorf(
			"Expected user status %q, got %q",
			"active",
			credentials.User.Status,
		)
	}

	// Prüfen, ob der Tenant tatsächlich in PostgreSQL gespeichert wurde
	var tenantCount int

	err := env.db.QueryRow(
		"SELECT COUNT(*) FROM tenants WHERE id = $1",
		credentials.User.TenantID,
	).Scan(&tenantCount)
	if err != nil {
		t.Fatalf("Failed to query registered tenant: %v", err)
	}

	if tenantCount != 1 {
		t.Fatalf("Expected one registered tenant, got %d", tenantCount)
	}

	// Prüfen, ob ausschließlich ein Passwort-Hash und kein Klartext gespeichert wurde
	var passwordHash string

	err = env.db.QueryRow(
		"SELECT password_hash FROM users WHERE id = $1",
		credentials.User.Id,
	).Scan(&passwordHash)
	if err != nil {
		t.Fatalf("Failed to query stored password hash: %v", err)
	}

	if passwordHash == credentials.Password {
		t.Fatal("Password was stored in plain text")
	}

	if err := bcrypt.CompareHashAndPassword(
		[]byte(passwordHash),
		[]byte(credentials.Password),
	); err != nil {
		t.Fatal("Stored password hash does not match the original password")
	}
}

// prüft die Registrierung eines Members anhand einer gültigen Einladung und den atomaren Verbrauch der Einladung
func TestIntegrationRegisterMember(t *testing.T) {
	env := newIntegrationTestEnvironment(t)

	owner := integrationRegisterOwner(t, env)

	// Eine gültige Einladung für den zuvor registrierten Owner erstellen
	invitation, invitationCode, err := env.app.models.Invitations.Create(
		context.Background(),
		owner.User.TenantID,
		owner.User.Id,
		time.Now().UTC().Add(time.Hour),
	)
	if err != nil {
		t.Fatalf("Failed to create integration test invitation: %v", err)
	}

	memberEmail := fmt.Sprintf(
		"member-%d@example.test",
		time.Now().UnixNano(),
	)

	response := integrationDoJSON(
		t,
		env,
		http.MethodPost,
		"/api/v1/auth/register",
		map[string]string{
			"registrationType": "member",
			"email":            memberEmail,
			"password":         integrationTestPassword,
			"name":             "Integration Member",
			"invitationCode":   invitationCode,
		},
		nil,
	)
	defer response.Body.Close()

	integrationRequireStatus(t, response, http.StatusCreated)

	var member database.User

	if err := json.NewDecoder(response.Body).Decode(&member); err != nil {
		t.Fatalf("Failed to decode member registration response: %v", err)
	}

	if member.Id <= 0 {
		t.Fatal("Expected a valid member ID")
	}

	if member.TenantID != owner.User.TenantID {
		t.Errorf(
			"Expected tenant ID %d, got %d",
			owner.User.TenantID,
			member.TenantID,
		)
	}

	if member.Role != "member" {
		t.Errorf(
			"Expected role %q, got %q",
			"member",
			member.Role,
		)
	}

	if member.Status != "active" {
		t.Errorf(
			"Expected status %q, got %q",
			"active",
			member.Status,
		)
	}

	if member.Password != "" {
		t.Error("The password hash must not be included in the response")
	}

	// Prüfen, ob die Einladung innerhalb derselben Transaktion verbraucht wurde
	var usedAt sql.NullString

	err = env.db.QueryRow(
		"SELECT used_at FROM invitations WHERE id = $1",
		invitation.ID,
	).Scan(&usedAt)
	if err != nil {
		t.Fatalf("Failed to query invitation usage: %v", err)
	}

	if !usedAt.Valid || usedAt.String == "" {
		t.Fatal("Expected the invitation to be marked as used")
	}

	// Prüfen, ob der Member tatsächlich in der DB gespeichert wurde
	var storedRole string

	err = env.db.QueryRow(
		"SELECT role FROM users WHERE id = $1 AND tenant_id = $2",
		member.Id,
		member.TenantID,
	).Scan(&storedRole)
	if err != nil {
		t.Fatalf("Failed to query registered member: %v", err)
	}

	if storedRole != "member" {
		t.Errorf("Expected stored role %q, got %q", "member", storedRole)
	}
}

// prüft die Konfliktbehandlung bei einer bereits registrierten Email-Adresse
func TestIntegrationRegisterDuplicateEmail(t *testing.T) {
	env := newIntegrationTestEnvironment(t)

	owner := integrationRegisterOwner(t, env)

	duplicateCompanyName := fmt.Sprintf(
		"Duplicate Company %d",
		time.Now().UnixNano(),
	)

	response := integrationDoJSON(
		t,
		env,
		http.MethodPost,
		"/api/v1/auth/register",
		map[string]string{
			"registrationType": "owner",
			"email":            owner.Email,
			"password":         integrationTestPassword,
			"name":             "Another Owner",
			"companyName":      duplicateCompanyName,
		},
		nil,
	)
	defer response.Body.Close()

	integrationRequireStatus(t, response, http.StatusConflict)

	// Die Tenant-Erstellung und User-Registrierung müssen atomar sein
	// Bei einem Email-Konflikt darf kein zusätzlicher Tenant zurückbleiben
	var tenantCount int

	err := env.db.QueryRow(
		"SELECT COUNT(*) FROM tenants WHERE name = $1",
		duplicateCompanyName,
	).Scan(&tenantCount)
	if err != nil {
		t.Fatalf("Failed to check tenant rollback: %v", err)
	}

	if tenantCount != 0 {
		t.Errorf(
			"Expected no tenant after failed registration, found %d",
			tenantCount,
		)
	}
}

// prüft die erfolgreiche Anmeldung, die Ausgabe des Access Tokens und das Refresh-Cookie
func TestIntegrationLogin(t *testing.T) {
	env := newIntegrationTestEnvironment(t)

	credentials := integrationRegisterOwner(t, env)

	// Großschreibung der Email prüfen, da die DB-abfrage normalisiert wird
	credentials.Email = fmt.Sprintf(
		"OWNER-%s@EXAMPLE.TEST",
		credentials.Email[len("owner-"):len(credentials.Email)-len("@example.test")],
	)

	_, refreshCookie := integrationLogin(t, env, credentials)

	if refreshCookie.Value == "" {
		t.Fatal("Expected a non-empty refresh token")
	}

	if refreshCookie.MaxAge != int(refreshTokenTTL.Seconds()) {
		t.Errorf(
			"Expected refresh cookie MaxAge %d, got %d",
			int(refreshTokenTTL.Seconds()),
			refreshCookie.MaxAge,
		)
	}
}

// prüft die Ablehnung eines unbekannten Benutzers und eines falschen Passworts
func TestIntegrationLoginInvalidCredentials(t *testing.T) {
	env := newIntegrationTestEnvironment(t)

	credentials := integrationRegisterOwner(t, env)

	tests := []struct {
		name     string
		email    string
		password string
	}{
		{
			name:     "Unknown email",
			email:    fmt.Sprintf("unknown-%d@example.test", time.Now().UnixNano()),
			password: integrationTestPassword,
		},
		{
			name:     "Incorrect password",
			email:    credentials.Email,
			password: "IncorrectPassword123!",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response := integrationDoJSON(
				t,
				env,
				http.MethodPost,
				"/api/v1/auth/login",
				map[string]string{
					"email":    tt.email,
					"password": tt.password,
				},
				nil,
			)
			defer response.Body.Close()

			integrationRequireStatus(
				t,
				response,
				http.StatusUnauthorized,
			)
		})
	}
}

// prüft die Erneuerung des Access Tokens anhand eines gültigen Refresh-Cookies
func TestIntegrationRefreshToken(t *testing.T) {
	env := newIntegrationTestEnvironment(t)

	credentials := integrationRegisterOwner(t, env)

	_, refreshCookie := integrationLogin(t, env, credentials)

	response := integrationDoJSON(
		t,
		env,
		http.MethodPost,
		"/api/v1/auth/refresh",
		map[string]string{},
		refreshCookie,
	)
	defer response.Body.Close()

	integrationRequireStatus(t, response, http.StatusOK)

	var result authResponse

	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to decode refresh response: %v", err)
	}

	if result.AccessToken == "" {
		t.Fatal("Expected a new access token")
	}

	if result.User.Id != credentials.User.Id {
		t.Errorf(
			"Expected user ID %d, got %d",
			credentials.User.Id,
			result.User.Id,
		)
	}

	claims, err := env.app.parseAuthToken(
		result.AccessToken,
		accessTokenType,
	)
	if err != nil {
		t.Fatalf("Returned access token is invalid: %v", err)
	}

	if claims.UserID != credentials.User.Id {
		t.Errorf(
			"Expected token user ID %d, got %d",
			credentials.User.Id,
			claims.UserID,
		)
	}
}

// prüft, dass ein Refresh ohne Cookie abgelehnt wird
func TestIntegrationRefreshWithoutCookie(t *testing.T) {
	env := newIntegrationTestEnvironment(t)

	response := integrationDoJSON(
		t,
		env,
		http.MethodPost,
		"/api/v1/auth/refresh",
		map[string]string{},
		nil,
	)
	defer response.Body.Close()

	integrationRequireStatus(t, response, http.StatusUnauthorized)
}

// prüft die erfolgreiche Abmeldung und das Löschen des Refresh-Cookies
func TestIntegrationLogout(t *testing.T) {
	env := newIntegrationTestEnvironment(t)

	response := integrationDoJSON(
		t,
		env,
		http.MethodPost,
		"/api/v1/auth/logout",
		map[string]string{},
		nil,
	)
	defer response.Body.Close()

	integrationRequireStatus(t, response, http.StatusOK)

	cookies := response.Cookies()

	if len(cookies) != 1 {
		t.Fatalf("Expected one cookie deletion header, got %d cookies", len(cookies))
	}

	cookie := cookies[0]

	if cookie.Name != "refreshToken" {
		t.Errorf(
			"Expected cookie name %q, got %q",
			"refreshToken",
			cookie.Name,
		)
	}

	if cookie.Value != "" {
		t.Errorf("Expected an empty cookie value, got %q", cookie.Value)
	}

	if cookie.MaxAge != -1 {
		t.Errorf("Expected MaxAge=-1, got %d", cookie.MaxAge)
	}

	if cookie.Path != "/api/v1/auth" {
		t.Errorf(
			"Expected cookie path %q, got %q",
			"/api/v1/auth",
			cookie.Path,
		)
	}

	if !cookie.HttpOnly {
		t.Error("Expected the deleted cookie to retain the HttpOnly attribute")
	}
}
