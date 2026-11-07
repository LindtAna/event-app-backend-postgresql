package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

// erstellt einen Test-Request für den Einladungs-Handler
// Die Werte userId und tenantId simulieren den Kontext, den die Auth-Middleware
// nach erfolgreicher Authentifizierung normalerweise bereitstellt
func newInvitationTestContext(
	body string,
	userID int,
	tenantID int,
) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	req := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/invitations",
		strings.NewReader(body),
	)
	req.Header.Set("Content-Type", "application/json")

	c.Request = req
	c.Set("userId", userID)
	c.Set("tenantId", tenantID)

	return c, recorder
}

// TestCreateInvitationValidation prüft die Eingabevalidierung des Handlers,
// bevor eine Einladung in der Datenbank angelegt werden könnte
func TestCreateInvitationValidation(t *testing.T) {
	// Die Ablaufdaten werden relativ zur aktuellen Zeit erzeugt, damit die Tests
	// unabhängig vom Ausführungsdatum bleiben und eindeutig in Zukunft bzw. Vergangenheit liegen.
	futureDate := time.Now().Add(24 * time.Hour).UTC().Format(time.RFC3339)
	pastDate := time.Now().Add(-24 * time.Hour).UTC().Format(time.RFC3339)

	tests := []struct {
		name           string
		body           string
		userID         int
		tenantID       int
		expectedStatus int
		expectedError  string
	}{
		{
			name:           "Missing authenticated context",
			body:           `{"expiresAt":"` + futureDate + `"}`,
			userID:         0,
			tenantID:       0,
			expectedStatus: http.StatusUnauthorized,
			expectedError:  "Unauthorized",
		},
		{
			name:           "Invalid JSON",
			body:           `{invalid json}`,
			userID:         10,
			tenantID:       20,
			expectedStatus: http.StatusBadRequest,
			expectedError:  "Invalid invitation data",
		},
		{
			name:           "Missing expiration date",
			body:           `{}`,
			userID:         10,
			tenantID:       20,
			expectedStatus: http.StatusBadRequest,
			expectedError:  "Invalid invitation data",
		},
		{
			name:           "Invalid expiration date format",
			body:           `{"expiresAt":"not-a-date"}`,
			userID:         10,
			tenantID:       20,
			expectedStatus: http.StatusBadRequest,
			expectedError:  "expiresAt must be a valid RFC3339 timestamp",
		},
		{
			name:           "Expiration date is in the past",
			body:           `{"expiresAt":"` + pastDate + `"}`,
			userID:         10,
			tenantID:       20,
			expectedStatus: http.StatusBadRequest,
			expectedError:  "expiresAt must be in the future",
		},
	}

	// Die App benötigt hier keine DB-vVerbindung:
	// Alle Testfälle müssen bereits bei der Prüfung von Kontext oder Request-Daten abgewiesen werden
	app := &application{}

	// Tabellengesteuerte Tests stellen sicher, dass alle Validierungsfälle dieselben
	// HTTP-Status- und Response-Prüfungen durchlaufen
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, recorder := newInvitationTestContext(
				tt.body,
				tt.userID,
				tt.tenantID,
			)

			app.createInvitation(c)

			if recorder.Code != tt.expectedStatus {
				t.Fatalf(
					"expected status %d, got %d; body: %s",
					tt.expectedStatus,
					recorder.Code,
					recorder.Body.String(),
				)
			}

			if !strings.Contains(recorder.Body.String(), tt.expectedError) {
				t.Errorf(
					"expected response body to contain %q, got %q",
					tt.expectedError,
					recorder.Body.String(),
				)
			}
		})
	}
}
