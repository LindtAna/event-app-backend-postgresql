//go:build integration

package main

import (
	"net/http"
	"strings"
	"testing"
)

const integrationProfilePath = "/api/v1/users/profile"

// sendet einen HTTP-Request mit einem Access Token an den geschützten API-Endpunkt
func integrationDoAuthenticatedRequest(
	t *testing.T,
	env *integrationTestEnvironment,
	method string,
	path string,
	accessToken string,
	body string,
) *http.Response {
	t.Helper()

	req, err := http.NewRequest(
		method,
		env.server.URL+path,
		strings.NewReader(body),
	)
	if err != nil {
		t.Fatalf("Failed to create authenticated HTTP request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")

	if accessToken != "" {
		req.Header.Set("Authorization", "Bearer "+accessToken)
	}

	response, err := env.server.Client().Do(req)
	if err != nil {
		t.Fatalf("Failed to send authenticated HTTP request: %v", err)
	}

	return response
}

// Prüft die erfolgreiche Aktualisierung des eigenen Profils,
// die Speicherung in PostgreSQL und den Schutz vertraulicher Benutzerdaten
func TestIntegrationUpdateProfile(t *testing.T) {
	env := newIntegrationTestEnvironment(t)

	owner := integrationRegisterOwner(t, env)
	auth, _ := integrationLogin(t, env, owner)

	updatedName := "Updated Integration Owner"
	updatedBio := "Updated integration test profile."
	updatedAvatarURL := "https://example.test/avatar.png"

	// Neben den Profildaten absichtlich zusätzliche Benutzerfelder senden
	// Diese dürfen durch den Client nicht verändert werden können
	payload := `{
		"name": "  ` + updatedName + `  ",
		"bio": "` + updatedBio + `",
		"avatarUrl": "  ` + updatedAvatarURL + `  ",
		"id": 999999,
		"tenantId": 999999,
		"role": "member",
		"status": "disabled",
		"email": "attacker@example.test"
	}`

	response := integrationDoAuthenticatedRequest(
		t,
		env,
		http.MethodPut,
		integrationProfilePath,
		auth.AccessToken,
		payload,
	)
	defer response.Body.Close()

	integrationRequireStatus(t, response, http.StatusOK)

	// Antwort des Profile-Endpunkts prüfen
	var result struct {
		Message string `json:"message"`
		User    struct {
			ID        int    `json:"id"`
			TenantID  int    `json:"tenantId"`
			Name      string `json:"name"`
			Email     string `json:"email"`
			Bio       string `json:"bio"`
			AvatarURL string `json:"avatarUrl"`
			Role      string `json:"role"`
			Status    string `json:"status"`
		} `json:"user"`
	}

	if err := decodeIntegrationResponse(response, &result); err != nil {
		t.Fatalf("Failed to decode profile update response: %v", err)
	}

	if result.Message != "Profile updated successfully" {
		t.Errorf(
			"Expected success message, got %q",
			result.Message,
		)
	}

	// Die aktualisierten Profildaten prüfen
	if result.User.Name != updatedName {
		t.Errorf(
			"Expected name %q, got %q",
			updatedName,
			result.User.Name,
		)
	}

	if result.User.Bio != updatedBio {
		t.Errorf(
			"Expected bio %q, got %q",
			updatedBio,
			result.User.Bio,
		)
	}

	if result.User.AvatarURL != updatedAvatarURL {
		t.Errorf(
			"Expected avatar URL %q, got %q",
			updatedAvatarURL,
			result.User.AvatarURL,
		)
	}

	// Benutzeridentität und Berechtigungen müssen unverändert bleiben
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

	if result.User.Email != owner.Email {
		t.Errorf(
			"Expected email %q, got %q",
			owner.Email,
			result.User.Email,
		)
	}

	if result.User.Role != "owner" {
		t.Errorf(
			"Expected role %q, got %q",
			"owner",
			result.User.Role,
		)
	}

	if result.User.Status != "active" {
		t.Errorf(
			"Expected status %q, got %q",
			"active",
			result.User.Status,
		)
	}

	// Zusätzlich prüfen, ob die Änderungen tatsächlich in PostgreSQL gespeichert wurden
	var (
		storedName      string
		storedBio       string
		storedAvatarURL string
		storedEmail     string
		storedTenantID  int
		storedRole      string
		storedStatus    string
	)

	err := env.db.QueryRow(`
		SELECT name, bio, avatar_url, email,
		       tenant_id, role, status
		FROM users
		WHERE id = $1
	`,
		owner.User.Id,
	).Scan(
		&storedName,
		&storedBio,
		&storedAvatarURL,
		&storedEmail,
		&storedTenantID,
		&storedRole,
		&storedStatus,
	)
	if err != nil {
		t.Fatalf("Failed to query updated profile from PostgreSQL: %v", err)
	}

	if storedName != updatedName {
		t.Errorf("Expected stored name %q, got %q", updatedName, storedName)
	}

	if storedBio != updatedBio {
		t.Errorf("Expected stored bio %q, got %q", updatedBio, storedBio)
	}

	if storedAvatarURL != updatedAvatarURL {
		t.Errorf(
			"Expected stored avatar URL %q, got %q",
			updatedAvatarURL,
			storedAvatarURL,
		)
	}

	if storedEmail != owner.Email {
		t.Errorf("Expected stored email %q, got %q", owner.Email, storedEmail)
	}

	if storedTenantID != owner.User.TenantID {
		t.Errorf(
			"Expected stored tenant ID %d, got %d",
			owner.User.TenantID,
			storedTenantID,
		)
	}

	if storedRole != "owner" {
		t.Errorf("Expected stored role %q, got %q", "owner", storedRole)
	}

	if storedStatus != "active" {
		t.Errorf("Expected stored status %q, got %q", "active", storedStatus)
	}
}

// Prüft, ob ungültige Profildaten und fehlerhaftes JSON mit HTTP 400 zurückgewiesen werden
func TestIntegrationUpdateProfileRejectsInvalidData(t *testing.T) {
	env := newIntegrationTestEnvironment(t)

	owner := integrationRegisterOwner(t, env)
	auth, _ := integrationLogin(t, env, owner)

	tests := []struct {
		name string
		body string
	}{
		{
			name: "Missing name",
			body: `{"bio":"Updated bio"}`,
		},
		{
			name: "Name shorter than two characters",
			body: `{"name":"A"}`,
		},
		{
			name: "Name containing only whitespace",
			body: `{"name":"   "}`,
		},
		{
			name: "Invalid JSON",
			body: `{"name":`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response := integrationDoAuthenticatedRequest(
				t,
				env,
				http.MethodPut,
				integrationProfilePath,
				auth.AccessToken,
				tt.body,
			)
			defer response.Body.Close()

			integrationRequireStatus(
				t,
				response,
				http.StatusBadRequest,
			)
		})
	}
}

// Prüft, ob das Benutzerprofil ohne gültigen Access Token nicht über den geschützten API-Endpunkt aktualisiert werden kann
func TestIntegrationUpdateProfileRequiresAuthentication(t *testing.T) {
	env := newIntegrationTestEnvironment(t)

	response := integrationDoAuthenticatedRequest(
		t,
		env,
		http.MethodPut,
		integrationProfilePath,
		"",
		`{"name":"Unauthorized Update"}`,
	)
	defer response.Body.Close()

	integrationRequireStatus(
		t,
		response,
		http.StatusUnauthorized,
	)
}
