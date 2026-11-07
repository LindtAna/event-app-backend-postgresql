//go:build integration

package main

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"event-app/internal/database"
)

// registriert einen Owner und meldet ihn an
func integrationCreateAttendeeOwner(
	t *testing.T,
	env *integrationTestEnvironment,
) (integrationAuthCredentials, string) {
	t.Helper()

	owner := integrationRegisterOwner(t, env)
	loginResult, _ := integrationLogin(t, env, owner)

	return owner, loginResult.AccessToken
}

// integrationCreateAttendeeMember registriert ein Mitglied über eine Einladung
// und gibt dessen Access Token für geschützte API-Aufrufe zurück
func integrationCreateAttendeeMember(
	t *testing.T,
	env *integrationTestEnvironment,
	ownerAccessToken string,
) (integrationAuthCredentials, string) {
	t.Helper()

	invitation := integrationCreateInvitation(t, env, ownerAccessToken)
	member := integrationRegisterMember(t, env, invitation.InvitationCode)
	loginResult, _ := integrationLogin(t, env, member)

	return member, loginResult.AccessToken
}

// sendet einen authentifizierten Request an einen Attendee-Endpunkt
func integrationDoAttendeeRequest(
	t *testing.T,
	env *integrationTestEnvironment,
	method string,
	path string,
	accessToken string,
) *http.Response {
	t.Helper()

	req, err := http.NewRequest(method, env.server.URL+path, nil)
	if err != nil {
		t.Fatalf("Failed to create attendee request: %v", err)
	}

	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")

	response, err := env.server.Client().Do(req)
	if err != nil {
		t.Fatalf("Failed to send attendee request: %v", err)
	}

	return response
}

// legt eine minimale Kategorie und ein Event direkt in PostgreSQL an
// Dadurch bleiben die Attendee-Tests unabhängig von den Event- und Category-HTTP-Handlern
func integrationCreateAttendeeEvent(
	t *testing.T,
	env *integrationTestEnvironment,
	tenantID int,
	ownerID int,
	title string,
	start time.Time,
) int {
	t.Helper()

	const categoryID = "integration-attendees-category"

	_, err := env.db.Exec(
		`INSERT INTO categories (tenant_id, id, name, name_key, is_default)
		 VALUES ($1, $2, $3, $4, 0)
		 ON CONFLICT (tenant_id, id) DO NOTHING`,
		tenantID,
		categoryID,
		"Integration Test",
		"integration test",
	)
	if err != nil {
		t.Fatalf("Failed to create the attendee test category: %v", err)
	}

	var eventID int
	err = env.db.QueryRow(
		`INSERT INTO events (
			tenant_id, owner_id, title, description, location,
			start_date_time, end_date_time, category_id
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id`,
		tenantID,
		ownerID,
		title,
		"Integration test event description",
		"Integration test location",
		start.UTC().Format(time.RFC3339Nano),
		start.Add(time.Hour).UTC().Format(time.RFC3339Nano),
		categoryID,
	).Scan(&eventID)
	if err != nil {
		t.Fatalf("Failed to create attendee test event: %v", err)
	}

	return eventID
}

// bereitet eine Teilnahme direkt in PostgreSQL vor
// Die zu testenden Änderungen selbst werden weiterhin über die HTTP-API ausgeführt
func integrationInsertAttendee(
	t *testing.T,
	env *integrationTestEnvironment,
	tenantID int,
	eventID int,
	userID int,
) {
	t.Helper()

	_, err := env.db.Exec(
		`INSERT INTO attendees (tenant_id, event_id, user_id)
		 VALUES ($1, $2, $3)`,
		tenantID,
		eventID,
		userID,
	)
	if err != nil {
		t.Fatalf("Failed to prepare attendee test data: %v", err)
	}
}

// prüft den tatsächlichen Teilnahmestatus in PostgreSQL
func integrationAttendeeExists(
	t *testing.T,
	env *integrationTestEnvironment,
	tenantID int,
	eventID int,
	userID int,
) bool {
	t.Helper()

	var exists bool
	err := env.db.QueryRow(
		`SELECT EXISTS (
			SELECT 1 FROM attendees
			WHERE tenant_id = $1 AND event_id = $2 AND user_id = $3
		)`,
		tenantID,
		eventID,
		userID,
	).Scan(&exists)
	if err != nil {
		t.Fatalf("Failed to query attendee existence: %v", err)
	}

	return exists
}

// setzt unterscheidbare Namen und Avatar-URLs, damit Sortierung und zurückgegebene Benutzerinformationen geprüft werden können
func integrationSetAttendeeUserData(
	t *testing.T,
	env *integrationTestEnvironment,
	tenantID int,
	userID int,
	name string,
	avatarURL string,
) {
	t.Helper()

	result, err := env.db.Exec(
		`UPDATE users
		 SET name = $1, avatar_url = $2
		 WHERE tenant_id = $3 AND id = $4`,
		name,
		avatarURL,
		tenantID,
		userID,
	)
	if err != nil {
		t.Fatalf("Failed to update attendee test user: %v", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		t.Fatalf("Failed to check attendee test user update: %v", err)
	}
	if rowsAffected != 1 {
		t.Fatalf("Expected to update one test user, updated %d", rowsAffected)
	}
}

// prüft das Hinzufügen von Teilnehmern einschließlich Berechtigungen,
// Tenant-Zuordnung, Aktivitätsstatus und doppelter Teilnahmen
func TestIntegrationAddAttendeeToEvent(t *testing.T) {
	env := newIntegrationTestEnvironment(t)

	owner, ownerToken := integrationCreateAttendeeOwner(t, env)
	memberA, memberAToken := integrationCreateAttendeeMember(t, env, ownerToken)
	memberB, _ := integrationCreateAttendeeMember(t, env, ownerToken)
	disabledMember, _ := integrationCreateAttendeeMember(t, env, ownerToken)

	foreignOwner := integrationRegisterOwner(t, env)

	eventID := integrationCreateAttendeeEvent(
		t, env, owner.User.TenantID, owner.User.Id,
		"Owner Event", time.Date(2027, 3, 10, 10, 0, 0, 0, time.UTC),
	)
	selfJoinEventID := integrationCreateAttendeeEvent(
		t, env, owner.User.TenantID, owner.User.Id,
		"Self Join Event", time.Date(2027, 3, 11, 10, 0, 0, 0, time.UTC),
	)
	foreignEventID := integrationCreateAttendeeEvent(
		t, env, foreignOwner.User.TenantID, foreignOwner.User.Id,
		"Foreign Tenant Event", time.Date(2027, 3, 12, 10, 0, 0, 0, time.UTC),
	)

	_, err := env.db.Exec(
		`UPDATE users SET status = 'disabled' WHERE tenant_id = $1 AND id = $2`,
		disabledMember.User.TenantID,
		disabledMember.User.Id,
	)
	if err != nil {
		t.Fatalf("Failed to disable attendee test user: %v", err)
	}

	t.Run("Owner can add an active user from the same tenant", func(t *testing.T) {
		response := integrationDoAttendeeRequest(
			t, env, http.MethodPost,
			fmt.Sprintf("/api/v1/events/%d/attendees/%d", eventID, memberA.User.Id),
			ownerToken,
		)
		defer response.Body.Close()

		integrationRequireStatus(t, response, http.StatusCreated)

		var attendee database.Attendee
		if err := decodeIntegrationResponse(response, &attendee); err != nil {
			t.Fatalf("Failed to decode attendee creation response: %v", err)
		}

		if attendee.Id <= 0 {
			t.Error("Expected a valid attendee ID")
		}
		if attendee.EventId != eventID {
			t.Errorf("Expected event ID %d, got %d", eventID, attendee.EventId)
		}
		if attendee.UserId != memberA.User.Id {
			t.Errorf("Expected user ID %d, got %d", memberA.User.Id, attendee.UserId)
		}
		if !integrationAttendeeExists(t, env, owner.User.TenantID, eventID, memberA.User.Id) {
			t.Error("Expected the attendee to be stored in PostgreSQL")
		}
	})

	t.Run("Duplicate attendance returns conflict", func(t *testing.T) {
		response := integrationDoAttendeeRequest(
			t, env, http.MethodPost,
			fmt.Sprintf("/api/v1/events/%d/attendees/%d", eventID, memberA.User.Id),
			ownerToken,
		)
		defer response.Body.Close()

		integrationRequireStatus(t, response, http.StatusConflict)
	})

	t.Run("A user can add themselves to an event", func(t *testing.T) {
		response := integrationDoAttendeeRequest(
			t, env, http.MethodPost,
			fmt.Sprintf("/api/v1/events/%d/attendees/%d", selfJoinEventID, memberA.User.Id),
			memberAToken,
		)
		defer response.Body.Close()

		integrationRequireStatus(t, response, http.StatusCreated)
		if !integrationAttendeeExists(t, env, owner.User.TenantID, selfJoinEventID, memberA.User.Id) {
			t.Error("Expected the user's self-registration to be stored")
		}
	})

	t.Run("A user cannot add another attendee to an event they do not own", func(t *testing.T) {
		response := integrationDoAttendeeRequest(
			t, env, http.MethodPost,
			fmt.Sprintf("/api/v1/events/%d/attendees/%d", eventID, memberB.User.Id),
			memberAToken,
		)
		defer response.Body.Close()

		integrationRequireStatus(t, response, http.StatusForbidden)
		if integrationAttendeeExists(t, env, owner.User.TenantID, eventID, memberB.User.Id) {
			t.Error("A forbidden request must not create an attendee record")
		}
	})

	t.Run("An inactive user cannot be added", func(t *testing.T) {
		response := integrationDoAttendeeRequest(
			t, env, http.MethodPost,
			fmt.Sprintf("/api/v1/events/%d/attendees/%d", eventID, disabledMember.User.Id),
			ownerToken,
		)
		defer response.Body.Close()

		integrationRequireStatus(t, response, http.StatusNotFound)
		if integrationAttendeeExists(t, env, owner.User.TenantID, eventID, disabledMember.User.Id) {
			t.Error("An inactive user must not be added as an attendee")
		}
	})

	t.Run("A user from another tenant cannot be added", func(t *testing.T) {
		response := integrationDoAttendeeRequest(
			t, env, http.MethodPost,
			fmt.Sprintf("/api/v1/events/%d/attendees/%d", eventID, foreignOwner.User.Id),
			ownerToken,
		)
		defer response.Body.Close()

		integrationRequireStatus(t, response, http.StatusNotFound)
	})

	t.Run("An event from another tenant is not accessible", func(t *testing.T) {
		response := integrationDoAttendeeRequest(
			t, env, http.MethodPost,
			fmt.Sprintf("/api/v1/events/%d/attendees/%d", foreignEventID, memberA.User.Id),
			ownerToken,
		)
		defer response.Body.Close()

		integrationRequireStatus(t, response, http.StatusNotFound)
	})
}

// prüft die Teilnehmerliste einschließlich leerer Ergebnisse,
// alphabetischer Sortierung und Tenant-Isolierung
func TestIntegrationGetAttendeesForEvent(t *testing.T) {
	env := newIntegrationTestEnvironment(t)

	owner, ownerToken := integrationCreateAttendeeOwner(t, env)
	memberA, _ := integrationCreateAttendeeMember(t, env, ownerToken)
	memberB, _ := integrationCreateAttendeeMember(t, env, ownerToken)
	foreignOwner := integrationRegisterOwner(t, env)

	eventID := integrationCreateAttendeeEvent(
		t, env, owner.User.TenantID, owner.User.Id,
		"Event With Attendees", time.Date(2027, 4, 10, 10, 0, 0, 0, time.UTC),
	)
	emptyEventID := integrationCreateAttendeeEvent(
		t, env, owner.User.TenantID, owner.User.Id,
		"Empty Event", time.Date(2027, 4, 11, 10, 0, 0, 0, time.UTC),
	)
	foreignEventID := integrationCreateAttendeeEvent(
		t, env, foreignOwner.User.TenantID, foreignOwner.User.Id,
		"Foreign Event", time.Date(2027, 4, 12, 10, 0, 0, 0, time.UTC),
	)

	integrationSetAttendeeUserData(
		t, env, owner.User.TenantID, memberA.User.Id,
		"Zoe", "https://example.test/zoe.png",
	)
	integrationSetAttendeeUserData(
		t, env, owner.User.TenantID, memberB.User.Id,
		"Alice", "",
	)

	integrationInsertAttendee(t, env, owner.User.TenantID, eventID, memberA.User.Id)
	integrationInsertAttendee(t, env, owner.User.TenantID, eventID, memberB.User.Id)

	t.Run("Returns attendees sorted by name", func(t *testing.T) {
		response := integrationDoAttendeeRequest(
			t, env, http.MethodGet,
			fmt.Sprintf("/api/v1/events/%d/attendees", eventID),
			ownerToken,
		)
		defer response.Body.Close()

		integrationRequireStatus(t, response, http.StatusOK)

		var users []database.User
		if err := decodeIntegrationResponse(response, &users); err != nil {
			t.Fatalf("Failed to decode attendee list: %v", err)
		}

		if len(users) != 2 {
			t.Fatalf("Expected 2 attendees, got %d", len(users))
		}
		if users[0].Id != memberB.User.Id || users[0].Name != "Alice" {
			t.Errorf("Expected Alice first, got user %d with name %q", users[0].Id, users[0].Name)
		}
		if users[1].Id != memberA.User.Id || users[1].Name != "Zoe" {
			t.Errorf("Expected Zoe second, got user %d with name %q", users[1].Id, users[1].Name)
		}
		if users[1].AvatarUrl != "https://example.test/zoe.png" {
			t.Errorf("Expected Zoe's avatar URL, got %q", users[1].AvatarUrl)
		}
	})

	t.Run("Returns an empty JSON array when there are no attendees", func(t *testing.T) {
		response := integrationDoAttendeeRequest(
			t, env, http.MethodGet,
			fmt.Sprintf("/api/v1/events/%d/attendees", emptyEventID),
			ownerToken,
		)
		defer response.Body.Close()

		integrationRequireStatus(t, response, http.StatusOK)

		var users []database.User
		if err := decodeIntegrationResponse(response, &users); err != nil {
			t.Fatalf("Failed to decode empty attendee list: %v", err)
		}
		if users == nil {
			t.Fatal("Expected an empty JSON array, got null")
		}
		if len(users) != 0 {
			t.Errorf("Expected no attendees, got %d", len(users))
		}
	})

	t.Run("An event from another tenant is not accessible", func(t *testing.T) {
		response := integrationDoAttendeeRequest(
			t, env, http.MethodGet,
			fmt.Sprintf("/api/v1/events/%d/attendees", foreignEventID),
			ownerToken,
		)
		defer response.Body.Close()

		integrationRequireStatus(t, response, http.StatusNotFound)
	})
}

// prüft die Events eines Teilnehmers einschließlich Datumsreihenfolge, leerer Ergebnisse und Tenant-Isolierung
func TestIntegrationGetEventsByAttendee(t *testing.T) {
	env := newIntegrationTestEnvironment(t)

	owner, ownerToken := integrationCreateAttendeeOwner(t, env)
	member, _ := integrationCreateAttendeeMember(t, env, ownerToken)
	memberWithoutEvents, _ := integrationCreateAttendeeMember(t, env, ownerToken)
	foreignOwner := integrationRegisterOwner(t, env)

	laterEventID := integrationCreateAttendeeEvent(
		t, env, owner.User.TenantID, owner.User.Id,
		"Later Event", time.Date(2027, 8, 15, 10, 0, 0, 0, time.UTC),
	)
	earliestEventID := integrationCreateAttendeeEvent(
		t, env, owner.User.TenantID, owner.User.Id,
		"Earlier Event", time.Date(2027, 2, 15, 10, 0, 0, 0, time.UTC),
	)
	foreignEventID := integrationCreateAttendeeEvent(
		t, env, foreignOwner.User.TenantID, foreignOwner.User.Id,
		"Foreign Tenant Event", time.Date(2027, 1, 15, 10, 0, 0, 0, time.UTC),
	)

	integrationInsertAttendee(t, env, owner.User.TenantID, laterEventID, member.User.Id)
	integrationInsertAttendee(t, env, owner.User.TenantID, earliestEventID, member.User.Id)
	integrationInsertAttendee(t, env, foreignOwner.User.TenantID, foreignEventID, foreignOwner.User.Id)

	t.Run("Returns a user's events sorted by start date", func(t *testing.T) {
		// Ein Tenant-Benutzer darf die Events eines anderen Benutzers desselben Tenants abrufen
		response := integrationDoAttendeeRequest(
			t, env, http.MethodGet,
			fmt.Sprintf("/api/v1/attendees/%d/events", member.User.Id),
			ownerToken,
		)
		defer response.Body.Close()

		integrationRequireStatus(t, response, http.StatusOK)

		var events []database.Event
		if err := decodeIntegrationResponse(response, &events); err != nil {
			t.Fatalf("Failed to decode attendee events: %v", err)
		}

		if len(events) != 2 {
			t.Fatalf("Expected 2 attendee events, got %d", len(events))
		}
		if events[0].Id != earliestEventID || events[0].Title != "Earlier Event" {
			t.Errorf("Expected the earlier event first, got event %d (%q)", events[0].Id, events[0].Title)
		}
		if events[1].Id != laterEventID || events[1].Title != "Later Event" {
			t.Errorf("Expected the later event second, got event %d (%q)", events[1].Id, events[1].Title)
		}
	})

	t.Run("Returns an empty JSON array when the user has no events", func(t *testing.T) {
		response := integrationDoAttendeeRequest(
			t, env, http.MethodGet,
			fmt.Sprintf("/api/v1/attendees/%d/events", memberWithoutEvents.User.Id),
			ownerToken,
		)
		defer response.Body.Close()

		integrationRequireStatus(t, response, http.StatusOK)

		var events []database.Event
		if err := decodeIntegrationResponse(response, &events); err != nil {
			t.Fatalf("Failed to decode empty attendee events: %v", err)
		}
		if events == nil {
			t.Fatal("Expected an empty JSON array, got null")
		}
		if len(events) != 0 {
			t.Errorf("Expected no events, got %d", len(events))
		}
	})

	t.Run("A user from another tenant is not accessible", func(t *testing.T) {
		response := integrationDoAttendeeRequest(
			t, env, http.MethodGet,
			fmt.Sprintf("/api/v1/attendees/%d/events", foreignOwner.User.Id),
			ownerToken,
		)
		defer response.Body.Close()

		integrationRequireStatus(t, response, http.StatusNotFound)
	})
}

// prüft das Entfernen von Teilnehmern durch den Event-Ersteller oder den jeweiligen Teilnehmer sowie Berechtigungsfehler und Tenant-Isolierung
func TestIntegrationDeleteAttendeeFromEvent(t *testing.T) {
	env := newIntegrationTestEnvironment(t)

	owner, ownerToken := integrationCreateAttendeeOwner(t, env)
	memberA, _ := integrationCreateAttendeeMember(t, env, ownerToken)
	memberB, memberBToken := integrationCreateAttendeeMember(t, env, ownerToken)
	foreignOwner := integrationRegisterOwner(t, env)

	ownerDeleteEventID := integrationCreateAttendeeEvent(
		t, env, owner.User.TenantID, owner.User.Id,
		"Owner Delete Event", time.Date(2027, 5, 10, 10, 0, 0, 0, time.UTC),
	)
	selfDeleteEventID := integrationCreateAttendeeEvent(
		t, env, owner.User.TenantID, owner.User.Id,
		"Self Delete Event", time.Date(2027, 5, 11, 10, 0, 0, 0, time.UTC),
	)
	forbiddenDeleteEventID := integrationCreateAttendeeEvent(
		t, env, owner.User.TenantID, owner.User.Id,
		"Forbidden Delete Event", time.Date(2027, 5, 12, 10, 0, 0, 0, time.UTC),
	)
	foreignEventID := integrationCreateAttendeeEvent(
		t, env, foreignOwner.User.TenantID, foreignOwner.User.Id,
		"Foreign Tenant Event", time.Date(2027, 5, 13, 10, 0, 0, 0, time.UTC),
	)

	integrationInsertAttendee(t, env, owner.User.TenantID, ownerDeleteEventID, memberA.User.Id)
	integrationInsertAttendee(t, env, owner.User.TenantID, selfDeleteEventID, memberB.User.Id)
	integrationInsertAttendee(t, env, owner.User.TenantID, forbiddenDeleteEventID, memberA.User.Id)
	integrationInsertAttendee(t, env, foreignOwner.User.TenantID, foreignEventID, foreignOwner.User.Id)

	t.Run("Event owner can remove an attendee", func(t *testing.T) {
		response := integrationDoAttendeeRequest(
			t, env, http.MethodDelete,
			fmt.Sprintf("/api/v1/events/%d/attendees/%d", ownerDeleteEventID, memberA.User.Id),
			ownerToken,
		)
		integrationRequireStatus(t, response, http.StatusNoContent)
		response.Body.Close()
		if integrationAttendeeExists(t, env, owner.User.TenantID, ownerDeleteEventID, memberA.User.Id) {
			t.Fatal("Expected the attendee record to be deleted")
		}

		// Eine bereits entfernte Teilnahme darf nicht erneut gelöscht werden
		repeatedResponse := integrationDoAttendeeRequest(
			t, env, http.MethodDelete,
			fmt.Sprintf("/api/v1/events/%d/attendees/%d", ownerDeleteEventID, memberA.User.Id),
			ownerToken,
		)
		defer repeatedResponse.Body.Close()

		integrationRequireStatus(t, repeatedResponse, http.StatusNotFound)
	})

	t.Run("An attendee can remove their own participation", func(t *testing.T) {
		response := integrationDoAttendeeRequest(
			t, env, http.MethodDelete,
			fmt.Sprintf("/api/v1/events/%d/attendees/%d", selfDeleteEventID, memberB.User.Id),
			memberBToken,
		)
		integrationRequireStatus(t, response, http.StatusNoContent)
		response.Body.Close()
		if integrationAttendeeExists(t, env, owner.User.TenantID, selfDeleteEventID, memberB.User.Id) {
			t.Error("Expected the attendee to be able to remove their own participation")
		}
	})

	t.Run("A user cannot remove another user's participation", func(t *testing.T) {
		response := integrationDoAttendeeRequest(
			t, env, http.MethodDelete,
			fmt.Sprintf("/api/v1/events/%d/attendees/%d", forbiddenDeleteEventID, memberA.User.Id),
			memberBToken,
		)
		defer response.Body.Close()

		integrationRequireStatus(t, response, http.StatusForbidden)
		if !integrationAttendeeExists(t, env, owner.User.TenantID, forbiddenDeleteEventID, memberA.User.Id) {
			t.Error("A forbidden request must not delete the attendee record")
		}
	})

	t.Run("An event from another tenant cannot be modified", func(t *testing.T) {
		response := integrationDoAttendeeRequest(
			t, env, http.MethodDelete,
			fmt.Sprintf("/api/v1/events/%d/attendees/%d", foreignEventID, foreignOwner.User.Id),
			ownerToken,
		)
		defer response.Body.Close()

		integrationRequireStatus(t, response, http.StatusNotFound)
		if !integrationAttendeeExists(t, env, foreignOwner.User.TenantID, foreignEventID, foreignOwner.User.Id) {
			t.Error("A request from another tenant must not delete the foreign attendee record")
		}
	})
}
