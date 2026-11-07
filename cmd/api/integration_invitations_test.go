//go:build integration

package main

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"event-app/internal/database"
)

// sendet einen HTTP-Request an den Testserver
// Ein Access Token wird bei Bedarf als Bearer Token im Authorization-Header übergeben
func integrationDoInvitationRequest(
	t *testing.T,
	env *integrationTestEnvironment,
	method string,
	path string,
	body string,
	accessToken string,
) *http.Response {
	t.Helper()

	req, err := http.NewRequest(
		method,
		env.server.URL+path,
		strings.NewReader(body),
	)
	if err != nil {
		t.Fatalf("Failed to create invitation request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")

	if accessToken != "" {
		req.Header.Set("Authorization", "Bearer "+accessToken)
	}

	response, err := env.server.Client().Do(req)
	if err != nil {
		t.Fatalf("Failed to send invitation request: %v", err)
	}

	return response
}

// serialisiert die Nutzdaten und sendet den Request mit dem angegebenen Access Token an den Integrationstestserver
func integrationDoInvitationJSON(
	t *testing.T,
	env *integrationTestEnvironment,
	method string,
	path string,
	payload interface{},
	accessToken string,
) *http.Response {
	t.Helper()

	var body []byte
	if payload != nil {
		var err error
		body, err = json.Marshal(payload)
		if err != nil {
			t.Fatalf("Failed to encode invitation request: %v", err)
		}
	}

	return integrationDoInvitationRequest(
		t,
		env,
		method,
		path,
		string(body),
		accessToken,
	)
}

// registriert einen Owner und meldet ihn an
// Für die geschützten Invitation-Endpunkte wird der Access Token benötigt
func integrationCreateOwnerAndLogin(
	t *testing.T,
	env *integrationTestEnvironment,
) (integrationAuthCredentials, string) {
	t.Helper()

	owner := integrationRegisterOwner(t, env)
	loginResult, _ := integrationLogin(t, env, owner)

	return owner, loginResult.AccessToken
}

type integrationInvitationResponse struct {
	Invitation     database.Invitation `json:"invitation"`
	InvitationCode string              `json:"invitationCode"`
}

// legt eine Einladung über den echten HTTP-Endpunkt an
func integrationCreateInvitation(
	t *testing.T,
	env *integrationTestEnvironment,
	accessToken string,
) integrationInvitationResponse {
	t.Helper()

	expiresAt := time.Now().UTC().Add(24 * time.Hour).Format(time.RFC3339)

	response := integrationDoInvitationJSON(
		t,
		env,
		http.MethodPost,
		"/api/v1/invitations",
		map[string]string{"expiresAt": expiresAt},
		accessToken,
	)
	defer response.Body.Close()

	integrationRequireStatus(t, response, http.StatusCreated)

	var result integrationInvitationResponse
	if err := decodeIntegrationResponse(response, &result); err != nil {
		t.Fatalf("Failed to decode invitation creation response: %v", err)
	}

	if result.Invitation.ID <= 0 {
		t.Fatal("Expected a valid invitation ID")
	}
	if result.InvitationCode == "" {
		t.Fatal("Expected the raw invitation code to be returned once")
	}

	return result
}

// registriert ein Mitglied mit einem gültigen Einladungscode
func integrationRegisterMember(
	t *testing.T,
	env *integrationTestEnvironment,
	invitationCode string,
) integrationAuthCredentials {
	t.Helper()

	email := fmt.Sprintf("invitation-member-%d@example.test", time.Now().UnixNano())

	response := integrationDoJSON(
		t,
		env,
		http.MethodPost,
		"/api/v1/auth/register",
		map[string]string{
			"registrationType": "member",
			"email":            email,
			"password":         integrationTestPassword,
			"name":             "Invitation Test Member",
			"invitationCode":   invitationCode,
		},
		nil,
	)
	defer response.Body.Close()

	integrationRequireStatus(t, response, http.StatusCreated)

	var member database.User
	if err := decodeIntegrationResponse(response, &member); err != nil {
		t.Fatalf("Failed to decode member registration response: %v", err)
	}

	if member.Id <= 0 {
		t.Fatal("Expected a valid member ID")
	}

	return integrationAuthCredentials{
		User:     member,
		Email:    email,
		Password: integrationTestPassword,
	}
}

// prüft, ob eine Einladung unter der angegebenen ID in der DB vorhanden ist
func integrationInvitationExists(
	t *testing.T,
	env *integrationTestEnvironment,
	invitationID int,
) bool {
	t.Helper()

	var count int
	err := env.db.QueryRow(
		"SELECT COUNT(*) FROM invitations WHERE id = $1",
		invitationID,
	).Scan(&count)
	if err != nil {
		t.Fatalf("Failed to query invitation existence: %v", err)
	}

	return count == 1
}

// prüft, ob eine Einladung widerrufen wurde
func integrationInvitationRevokedAt(
	t *testing.T,
	env *integrationTestEnvironment,
	invitationID int,
) sql.NullString {
	t.Helper()

	var revokedAt sql.NullString
	err := env.db.QueryRow(
		"SELECT revoked_at FROM invitations WHERE id = $1",
		invitationID,
	).Scan(&revokedAt)
	if err != nil {
		t.Fatalf("Failed to query invitation revocation state: %v", err)
	}

	return revokedAt
}

// prüft die Erstellung einer Einladung, die Tenant-Zuordnung und die sichere Speicherung des Token-Hashes
func TestIntegrationCreateInvitation(t *testing.T) {
	env := newIntegrationTestEnvironment(t)
	owner, accessToken := integrationCreateOwnerAndLogin(t, env)

	result := integrationCreateInvitation(t, env, accessToken)

	if result.Invitation.TenantID != owner.User.TenantID {
		t.Errorf(
			"Expected tenant ID %d, got %d",
			owner.User.TenantID,
			result.Invitation.TenantID,
		)
	}

	if result.Invitation.CreatedBy != owner.User.Id {
		t.Errorf(
			"Expected creator ID %d, got %d",
			owner.User.Id,
			result.Invitation.CreatedBy,
		)
	}

	if result.Invitation.ExpiresAt == "" {
		t.Error("Expected an expiration timestamp")
	}
	if result.Invitation.CreatedAt == "" {
		t.Error("Expected a creation timestamp")
	}
	if result.Invitation.UsedAt != nil {
		t.Error("A newly created invitation must not be marked as used")
	}
	if result.Invitation.RevokedAt != nil {
		t.Error("A newly created invitation must not be marked as revoked")
	}

	// Der gespeicherte Wert muss dem SHA-256-Hash des Tokens entsprechen,
	// während das rohe Token nicht in der Datenbank abgelegt werden darf
	var storedTokenHash string
	err := env.db.QueryRow(
		"SELECT token_hash FROM invitations WHERE id = $1 AND tenant_id = $2",
		result.Invitation.ID,
		owner.User.TenantID,
	).Scan(&storedTokenHash)
	if err != nil {
		t.Fatalf("Failed to query stored invitation token hash: %v", err)
	}

	sum := sha256.Sum256([]byte(result.InvitationCode))
	expectedTokenHash := hex.EncodeToString(sum[:])

	if storedTokenHash == result.InvitationCode {
		t.Fatal("The raw invitation token must not be stored in the database")
	}
	if storedTokenHash != expectedTokenHash {
		t.Errorf(
			"Expected stored token hash %q, got %q",
			expectedTokenHash,
			storedTokenHash,
		)
	}
}

// prüft die Ablehnung ungültiger Ablaufzeitpunkte beim Erstellen einer Einladung
func TestIntegrationCreateInvitationRejectsInvalidData(t *testing.T) {
	env := newIntegrationTestEnvironment(t)
	owner, accessToken := integrationCreateOwnerAndLogin(t, env)

	pastDate := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339)

	tests := []struct {
		name          string
		body          string
		expectedError string
	}{
		{
			name:          "Missing expiration date",
			body:          `{}`,
			expectedError: "Invalid invitation data",
		},
		{
			name:          "Invalid JSON",
			body:          `{invalid json}`,
			expectedError: "Invalid invitation data",
		},
		{
			name:          "Invalid expiration date format",
			body:          `{"expiresAt":"not-a-date"}`,
			expectedError: "expiresAt must be a valid RFC3339 timestamp",
		},
		{
			name:          "Expiration date is in the past",
			body:          fmt.Sprintf(`{"expiresAt":%q}`, pastDate),
			expectedError: "expiresAt must be in the future",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response := integrationDoInvitationRequest(
				t,
				env,
				http.MethodPost,
				"/api/v1/invitations",
				tt.body,
				accessToken,
			)
			defer response.Body.Close()

			integrationRequireStatus(t, response, http.StatusBadRequest)

			var result map[string]string
			if err := decodeIntegrationResponse(response, &result); err != nil {
				t.Fatalf("Failed to decode validation error response: %v", err)
			}

			if result["error"] != tt.expectedError {
				t.Errorf(
					"Expected error %q, got %q",
					tt.expectedError,
					result["error"],
				)
			}
		})
	}

	// Kein ungültiger Request darf einen Datensatz in der Datenbank anlegen
	var invitationCount int
	err := env.db.QueryRow(
		"SELECT COUNT(*) FROM invitations WHERE tenant_id = $1",
		owner.User.TenantID,
	).Scan(&invitationCount)
	if err != nil {
		t.Fatalf("Failed to query invitation count: %v", err)
	}
	if invitationCount != 0 {
		t.Errorf("Expected no invitations after invalid requests, got %d", invitationCount)
	}
}

// prüft eine leere Einladungsliste sowie die Tenant-Isolation beim Abruf
func TestIntegrationGetAllInvitations(t *testing.T) {
	env := newIntegrationTestEnvironment(t)
	ownerA, accessTokenA := integrationCreateOwnerAndLogin(t, env)
	_, accessTokenB := integrationCreateOwnerAndLogin(t, env)

	// Ein Tenant ohne Einladungen muss eine leere JSON-Liste statt null erhalten
	emptyResponse := integrationDoInvitationJSON(
		t,
		env,
		http.MethodGet,
		"/api/v1/invitations",
		nil,
		accessTokenA,
	)
	integrationRequireStatus(t, emptyResponse, http.StatusOK)

	var emptyList []database.Invitation
	if err := decodeIntegrationResponse(emptyResponse, &emptyList); err != nil {
		emptyResponse.Body.Close()
		t.Fatalf("Failed to decode empty invitation list: %v", err)
	}
	emptyResponse.Body.Close()

	if emptyList == nil {
		t.Fatal("Expected an empty JSON array, got nil")
	}
	if len(emptyList) != 0 {
		t.Fatalf("Expected an empty invitation list, got %d entries", len(emptyList))
	}

	first := integrationCreateInvitation(t, env, accessTokenA)
	second := integrationCreateInvitation(t, env, accessTokenA)
	foreign := integrationCreateInvitation(t, env, accessTokenB)

	response := integrationDoInvitationJSON(
		t,
		env,
		http.MethodGet,
		"/api/v1/invitations",
		nil,
		accessTokenA,
	)
	defer response.Body.Close()

	integrationRequireStatus(t, response, http.StatusOK)

	var invitations []database.Invitation
	if err := decodeIntegrationResponse(response, &invitations); err != nil {
		t.Fatalf("Failed to decode invitation list: %v", err)
	}

	if len(invitations) != 2 {
		t.Fatalf("Expected 2 invitations for the first tenant, got %d", len(invitations))
	}

	found := make(map[int]bool, len(invitations))
	for _, invitation := range invitations {
		if invitation.TenantID != ownerA.User.TenantID {
			t.Errorf(
				"Expected only tenant %d invitations, got tenant %d",
				ownerA.User.TenantID,
				invitation.TenantID,
			)
		}
		found[invitation.ID] = true
	}

	if !found[first.Invitation.ID] || !found[second.Invitation.ID] {
		t.Error("The invitation list does not contain both invitations belonging to the authenticated tenant")
	}
	if found[foreign.Invitation.ID] {
		t.Error("The invitation list must not contain an invitation belonging to another tenant")
	}
}

// prüft, dass alle Invitation-Endpunkte ohne Access Token nicht erreichbar sind
func TestIntegrationInvitationEndpointsRequireAuthentication(t *testing.T) {
	env := newIntegrationTestEnvironment(t)
	_, accessToken := integrationCreateOwnerAndLogin(t, env)
	invitation := integrationCreateInvitation(t, env, accessToken)

	tests := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{
			name:   "Create invitation",
			method: http.MethodPost,
			path:   "/api/v1/invitations",
			body:   `{"expiresAt":"` + time.Now().UTC().Add(24*time.Hour).Format(time.RFC3339) + `"}`,
		},
		{
			name:   "List invitations",
			method: http.MethodGet,
			path:   "/api/v1/invitations",
		},
		{
			name:   "Revoke invitation",
			method: http.MethodDelete,
			path:   fmt.Sprintf("/api/v1/invitations/%d", invitation.Invitation.ID),
		},
		{
			name:   "Permanently delete invitation",
			method: http.MethodDelete,
			path:   fmt.Sprintf("/api/v1/invitations/%d/permanent", invitation.Invitation.ID),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response := integrationDoInvitationRequest(
				t,
				env,
				tt.method,
				tt.path,
				tt.body,
				"",
			)
			defer response.Body.Close()

			integrationRequireStatus(t, response, http.StatusUnauthorized)
		})
	}
}

// prüft, dass authentifizierte Mitglieder keine Einladungen verwalten dürfen
func TestIntegrationInvitationEndpointsRequireOwner(t *testing.T) {
	env := newIntegrationTestEnvironment(t)
	owner, ownerAccessToken := integrationCreateOwnerAndLogin(t, env)

	// Eine Einladung zur Registrierung des Members und eine weitere für die Zugriffstests anlegen
	memberRegistrationInvitation := integrationCreateInvitation(t, env, ownerAccessToken)
	protectedInvitation := integrationCreateInvitation(t, env, ownerAccessToken)

	member := integrationRegisterMember(
		t,
		env,
		memberRegistrationInvitation.InvitationCode,
	)
	memberLogin, _ := integrationLogin(t, env, member)

	if member.User.TenantID != owner.User.TenantID {
		t.Fatalf("Expected member tenant ID %d, got %d", owner.User.TenantID, member.User.TenantID)
	}
	if member.User.Role != "member" {
		t.Fatalf("Expected member role, got %q", member.User.Role)
	}

	tests := []struct {
		name   string
		method string
		path   string
		body   interface{}
	}{
		{
			name:   "Create invitation",
			method: http.MethodPost,
			path:   "/api/v1/invitations",
			body: map[string]string{
				"expiresAt": time.Now().UTC().Add(24 * time.Hour).Format(time.RFC3339),
			},
		},
		{
			name:   "List invitations",
			method: http.MethodGet,
			path:   "/api/v1/invitations",
			body:   nil,
		},
		{
			name:   "Revoke invitation",
			method: http.MethodDelete,
			path:   fmt.Sprintf("/api/v1/invitations/%d", protectedInvitation.Invitation.ID),
			body:   nil,
		},
		{
			name:   "Permanently delete invitation",
			method: http.MethodDelete,
			path:   fmt.Sprintf("/api/v1/invitations/%d/permanent", protectedInvitation.Invitation.ID),
			body:   nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response := integrationDoInvitationJSON(
				t,
				env,
				tt.method,
				tt.path,
				tt.body,
				memberLogin.AccessToken,
			)
			defer response.Body.Close()

			integrationRequireStatus(t, response, http.StatusForbidden)
		})
	}

	// Abgewiesene Member-Requests dürfen die Einladung nicht verändern oder löschen
	if !integrationInvitationExists(t, env, protectedInvitation.Invitation.ID) {
		t.Fatal("The member must not be able to delete the owner's invitation")
	}
	if integrationInvitationRevokedAt(t, env, protectedInvitation.Invitation.ID).Valid {
		t.Fatal("The member must not be able to revoke the owner's invitation")
	}
}

// prüft den erfolgreichen Widerruf und die Ablehnung eines erneuten Widerrufs
func TestIntegrationRevokeInvitation(t *testing.T) {
	env := newIntegrationTestEnvironment(t)
	_, accessToken := integrationCreateOwnerAndLogin(t, env)
	invitation := integrationCreateInvitation(t, env, accessToken)

	path := fmt.Sprintf("/api/v1/invitations/%d", invitation.Invitation.ID)

	response := integrationDoInvitationJSON(
		t,
		env,
		http.MethodDelete,
		path,
		nil,
		accessToken,
	)
	response.Body.Close()
	integrationRequireStatus(t, response, http.StatusNoContent)

	revokedAt := integrationInvitationRevokedAt(t, env, invitation.Invitation.ID)
	if !revokedAt.Valid || revokedAt.String == "" {
		t.Fatal("Expected the invitation to have a revocation timestamp")
	}

	// Eine bereits widerrufene Einladung gilt nicht mehr als aktiv
	repeatedResponse := integrationDoInvitationJSON(
		t,
		env,
		http.MethodDelete,
		path,
		nil,
		accessToken,
	)
	defer repeatedResponse.Body.Close()

	integrationRequireStatus(t, repeatedResponse, http.StatusNotFound)
}

// prüft, dass ein Tenant keine Einladung eines anderen Tenants widerrufen kann
func TestIntegrationRevokeInvitationTenantIsolation(t *testing.T) {
	env := newIntegrationTestEnvironment(t)
	_, accessTokenA := integrationCreateOwnerAndLogin(t, env)
	_, accessTokenB := integrationCreateOwnerAndLogin(t, env)
	foreignInvitation := integrationCreateInvitation(t, env, accessTokenB)

	response := integrationDoInvitationJSON(
		t,
		env,
		http.MethodDelete,
		fmt.Sprintf("/api/v1/invitations/%d", foreignInvitation.Invitation.ID),
		nil,
		accessTokenA,
	)
	defer response.Body.Close()

	integrationRequireStatus(t, response, http.StatusNotFound)

	revokedAt := integrationInvitationRevokedAt(t, env, foreignInvitation.Invitation.ID)
	if revokedAt.Valid {
		t.Fatal("A tenant must not be able to revoke another tenant's invitation")
	}
}

// prüft den dauerhaften Löschvorgang sowie die Ablehnung einer erneuten Löschung
func TestIntegrationDeleteInvitation(t *testing.T) {
	env := newIntegrationTestEnvironment(t)
	_, accessToken := integrationCreateOwnerAndLogin(t, env)
	invitation := integrationCreateInvitation(t, env, accessToken)

	path := fmt.Sprintf("/api/v1/invitations/%d/permanent", invitation.Invitation.ID)

	response := integrationDoInvitationJSON(
		t,
		env,
		http.MethodDelete,
		path,
		nil,
		accessToken,
	)
	response.Body.Close()
	integrationRequireStatus(t, response, http.StatusNoContent)

	if integrationInvitationExists(t, env, invitation.Invitation.ID) {
		t.Fatal("Expected the invitation to be permanently removed from the database")
	}

	repeatedResponse := integrationDoInvitationJSON(
		t,
		env,
		http.MethodDelete,
		path,
		nil,
		accessToken,
	)
	defer repeatedResponse.Body.Close()

	integrationRequireStatus(t, repeatedResponse, http.StatusNotFound)
}

// prüft, dass ein Tenant eine Einladung eines anderen Tenants nicht dauerhaft löschen kann
func TestIntegrationDeleteInvitationTenantIsolation(t *testing.T) {
	env := newIntegrationTestEnvironment(t)
	_, accessTokenA := integrationCreateOwnerAndLogin(t, env)
	_, accessTokenB := integrationCreateOwnerAndLogin(t, env)
	foreignInvitation := integrationCreateInvitation(t, env, accessTokenB)

	response := integrationDoInvitationJSON(
		t,
		env,
		http.MethodDelete,
		fmt.Sprintf("/api/v1/invitations/%d/permanent", foreignInvitation.Invitation.ID),
		nil,
		accessTokenA,
	)
	defer response.Body.Close()

	integrationRequireStatus(t, response, http.StatusNotFound)

	if !integrationInvitationExists(t, env, foreignInvitation.Invitation.ID) {
		t.Fatal("A tenant must not be able to delete another tenant's invitation")
	}
}
