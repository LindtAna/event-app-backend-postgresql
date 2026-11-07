//go:build integration

package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// erstellt einen HTTP-Testserver, der die echte Auth-Middleware mit einem geschützten Test-Endpunkt verwendet
func integrationAuthMiddlewareServer(
	t *testing.T,
	env *integrationTestEnvironment,
	handler gin.HandlerFunc,
) *httptest.Server {
	t.Helper()

	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(env.app.AuthMiddleware())
	router.GET("/protected", handler)

	server := httptest.NewServer(router)
	t.Cleanup(server.Close)

	return server
}

// sendet einen geschützten HTTP-Request mit dem angegebenen Access Token an den Testserver
func integrationDoAuthMiddlewareRequest(
	t *testing.T,
	server *httptest.Server,
	accessToken string,
) *http.Response {
	t.Helper()

	req, err := http.NewRequest(
		http.MethodGet,
		server.URL+"/protected",
		nil,
	)
	if err != nil {
		t.Fatalf("Failed to create HTTP request: %v", err)
	}

	req.Header.Set("Authorization", "Bearer "+accessToken)

	response, err := server.Client().Do(req)
	if err != nil {
		t.Fatalf("Failed to send HTTP request: %v", err)
	}

	return response
}

// registriert einen Owner und meldet ihn an
// Das zurückgegebene Access Token wird für die Middleware-Tests verwendet
func integrationAuthenticatedOwner(
	t *testing.T,
	env *integrationTestEnvironment,
) (integrationAuthCredentials, string) {
	t.Helper()

	owner := integrationRegisterOwner(t, env)

	auth, _ := integrationLogin(t, env, owner)

	return owner, auth.AccessToken
}

// prüft, dass die Middleware den Request mit HTTP 401 zurückweist und den nachgelagerten Handler nicht ausführt
func integrationRequireMiddlewareUnauthorized(
	t *testing.T,
	env *integrationTestEnvironment,
	accessToken string,
) {
	t.Helper()

	handlerCalled := false

	server := integrationAuthMiddlewareServer(
		t,
		env,
		func(c *gin.Context) {
			handlerCalled = true
			c.Status(http.StatusOK)
		},
	)

	response := integrationDoAuthMiddlewareRequest(
		t,
		server,
		accessToken,
	)
	defer response.Body.Close()

	integrationRequireStatus(
		t,
		response,
		http.StatusUnauthorized,
	)

	if handlerCalled {
		t.Error("Protected handler must not be called for an inactive or deleted user")
	}
}

// Prüft, ob die Middleware die aktuellen Benutzerdaten aus PostgreSQL lädt
// und die entsprechenden Werte im Gin-Kontext bereitstellt
func TestIntegrationAuthMiddlewareUsesCurrentUserFromDatabase(t *testing.T) {
	env := newIntegrationTestEnvironment(t)

	owner, accessToken := integrationAuthenticatedOwner(t, env)

	updatedName := "Updated Integration Owner"

	// Benutzerinformationen nach der Token-Ausstellung direkt in PostgreSQL ändern
	// Der Access Token enthält weiterhin nur die ursprüngliche User-ID
	_, err := env.db.Exec(`
		UPDATE users
		SET name = $1, role = 'member'
		WHERE id = $2 AND tenant_id = $3
	`,
		updatedName,
		owner.User.Id,
		owner.User.TenantID,
	)
	if err != nil {
		t.Fatalf("Failed to update integration test user: %v", err)
	}

	server := integrationAuthMiddlewareServer(
		t,
		env,
		func(c *gin.Context) {
			// Den von der Auth-Middleware bereitgestellten Benutzer auslesen.
			user := env.app.GetUserFromContext(c)

			// Zusätzlich die einzelnen Kontextwerte abrufen.
			contextUserID, _ := c.Get("userId")
			contextTenantID, _ := c.Get("tenantId")
			contextRole, _ := c.Get("role")

			c.JSON(http.StatusOK, gin.H{
				"user": user,
				"context": gin.H{
					"userId":   contextUserID,
					"tenantId": contextTenantID,
					"role":     contextRole,
				},
			})
		},
	)

	response := integrationDoAuthMiddlewareRequest(
		t,
		server,
		accessToken,
	)
	defer response.Body.Close()

	integrationRequireStatus(t, response, http.StatusOK)

	var result struct {
		User struct {
			ID       int    `json:"id"`
			TenantID int    `json:"tenantId"`
			Name     string `json:"name"`
			Role     string `json:"role"`
			Status   string `json:"status"`
		} `json:"user"`

		Context struct {
			UserID   int    `json:"userId"`
			TenantID int    `json:"tenantId"`
			Role     string `json:"role"`
		} `json:"context"`
	}

	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to decode middleware response: %v", err)
	}

	// Prüfen, ob die aktuellen Benutzerdaten aus PostgreSQL stammen
	if result.User.ID != owner.User.Id {
		t.Errorf(
			"Expected user ID %d, got %d",
			owner.User.Id,
			result.User.ID,
		)
	}

	if result.User.TenantID != owner.User.TenantID {
		t.Errorf(
			"Expected tenant ID %d, got %d",
			owner.User.TenantID,
			result.User.TenantID,
		)
	}

	if result.User.Name != updatedName {
		t.Errorf(
			"Expected updated name %q, got %q",
			updatedName,
			result.User.Name,
		)
	}

	if result.User.Role != "member" {
		t.Errorf(
			"Expected updated role %q, got %q",
			"member",
			result.User.Role,
		)
	}

	if result.User.Status != "active" {
		t.Errorf(
			"Expected user status %q, got %q",
			"active",
			result.User.Status,
		)
	}

	// Prüfen, ob die aktuellen Daten korrekt im Gin-Kontext hinterlegt sind
	if result.Context.UserID != owner.User.Id {
		t.Errorf(
			"Expected context user ID %d, got %d",
			owner.User.Id,
			result.Context.UserID,
		)
	}

	if result.Context.TenantID != owner.User.TenantID {
		t.Errorf(
			"Expected context tenant ID %d, got %d",
			owner.User.TenantID,
			result.Context.TenantID,
		)
	}

	if result.Context.Role != "member" {
		t.Errorf(
			"Expected context role %q, got %q",
			"member",
			result.Context.Role,
		)
	}
}

// Prüft, ob ein Benutzer nach seiner Deaktivierung
// trotz eines zuvor ausgestellten gültigen Access Tokens abgewiesen wird
func TestIntegrationAuthMiddlewareRejectsDisabledUser(t *testing.T) {
	env := newIntegrationTestEnvironment(t)

	owner, accessToken := integrationAuthenticatedOwner(t, env)

	// Benutzer nach der Token-Ausstellung deaktivieren
	_, err := env.db.Exec(`
		UPDATE users
		SET status = 'disabled'
		WHERE id = $1 AND tenant_id = $2
	`,
		owner.User.Id,
		owner.User.TenantID,
	)
	if err != nil {
		t.Fatalf("Failed to disable integration test user: %v", err)
	}

	// Der zuvor ausgestellte Access Token darf nicht mehr gültig sein
	integrationRequireMiddlewareUnauthorized(
		t,
		env,
		accessToken,
	)
}

// Prüft, ob Benutzer eines gesperrten Tenants
// trotz eines zuvor ausgestellten gültigen Access Tokens abgewiesen werden
func TestIntegrationAuthMiddlewareRejectsSuspendedTenant(t *testing.T) {
	env := newIntegrationTestEnvironment(t)

	owner, accessToken := integrationAuthenticatedOwner(t, env)

	// Tenant nach der Token-Ausstellung sperren
	_, err := env.db.Exec(`
		UPDATE tenants
		SET status = 'suspended'
		WHERE id = $1
	`,
		owner.User.TenantID,
	)
	if err != nil {
		t.Fatalf("Failed to suspend integration test tenant: %v", err)
	}

	// Der Benutzer darf nicht mehr auf geschützte Endpunkte zugreifen
	integrationRequireMiddlewareUnauthorized(
		t,
		env,
		accessToken,
	)
}

// Prüft, ob ein gelöschter Benutzer
// trotz eines zuvor ausgestellten gültigen Access Tokens abgewiesen wird
func TestIntegrationAuthMiddlewareRejectsDeletedUser(t *testing.T) {
	env := newIntegrationTestEnvironment(t)

	owner, accessToken := integrationAuthenticatedOwner(t, env)

	// Benutzer nach der Token-Ausstellung dauerhaft aus PostgreSQL entfernen.
	// Der Tenant bleibt bestehen, da ausschließlich der Benutzer gelöscht wird
	_, err := env.db.Exec(`
		DELETE FROM users
		WHERE id = $1 AND tenant_id = $2
	`,
		owner.User.Id,
		owner.User.TenantID,
	)
	if err != nil {
		t.Fatalf("Failed to delete integration test user: %v", err)
	}

	// Die Middleware muss den fehlenden Benutzer erkennen
	integrationRequireMiddlewareUnauthorized(
		t,
		env,
		accessToken,
	)
}
