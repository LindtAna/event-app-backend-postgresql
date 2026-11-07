//go:build integration

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"event-app/internal/database"
)

const integrationEventTestCategoryID = "integration-events-category"

// sendet einen authentifizierten HTTP-Request an einen Event-Endpunkt und übermittelt optional einen JSON-Body
func integrationDoEventRequest(
	t *testing.T,
	env *integrationTestEnvironment,
	method string,
	path string,
	accessToken string,
	payload any,
) *http.Response {
	t.Helper()

	var body io.Reader

	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("Failed to encode event request: %v", err)
		}

		body = bytes.NewReader(encoded)
	}

	req, err := http.NewRequest(
		method,
		env.server.URL+path,
		body,
	)
	if err != nil {
		t.Fatalf("Failed to create event request: %v", err)
	}

	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")

	response, err := env.server.Client().Do(req)
	if err != nil {
		t.Fatalf("Failed to send event request: %v", err)
	}

	return response
}

// legt eine minimale technische Kategorie für die Event-Integrationstests an
func integrationEnsureEventTestCategory(
	t *testing.T,
	env *integrationTestEnvironment,
	tenantID int,
) string {
	t.Helper()

	_, err := env.db.Exec(
		`INSERT INTO categories (
			tenant_id, id, name, name_key, is_default
		)
		VALUES ($1, $2, $3, $4, 0)
		ON CONFLICT (tenant_id, id) DO NOTHING`,
		tenantID,
		integrationEventTestCategoryID,
		"Integration Events",
		"integration events",
	)
	if err != nil {
		t.Fatalf("Failed to create event test category: %v", err)
	}

	return integrationEventTestCategoryID
}

// erstellt ein Event direkt in PostgreSQL.
// Die HTTP-API ist nicht Gegenstand der jeweiligen Testdatenvorbereitung
func integrationCreateEventFixture(
	t *testing.T,
	env *integrationTestEnvironment,
	tenantID int,
	ownerID int,
	title string,
	start time.Time,
) int {
	t.Helper()

	categoryID := integrationEnsureEventTestCategory(
		t,
		env,
		tenantID,
	)

	var eventID int

	err := env.db.QueryRow(
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
		t.Fatalf("Failed to create event test fixture: %v", err)
	}

	return eventID
}

// erstellt gültige Event-Testdaten
func integrationEventPayload(
	title string,
	categoryID string,
	ownerID int,
) database.Event {
	return database.Event{
		OwnerId:       ownerID,
		Title:         title,
		Description:   "Integration test event description",
		Location:      "Integration test location",
		StartDateTime: "2027-06-10T10:00:00Z",
		EndDateTime:   "2027-06-10T11:00:00Z",
		CategoryId:    categoryID,
		Url:           "https://example.test/events/integration",
	}
}

// prüft, ob ein Event in PostgreSQL existiert
func integrationEventExists(
	t *testing.T,
	env *integrationTestEnvironment,
	tenantID int,
	eventID int,
) bool {
	t.Helper()

	var exists bool

	err := env.db.QueryRow(
		`SELECT EXISTS (
			SELECT 1 FROM events
			WHERE tenant_id = $1 AND id = $2
		)`,
		tenantID,
		eventID,
	).Scan(&exists)

	if err != nil {
		t.Fatalf("Failed to query event existence: %v", err)
	}

	return exists
}

// liest den tatsächlichen Owner und Titel eines Events aus PostgreSQL
func integrationReadEventState(
	t *testing.T,
	env *integrationTestEnvironment,
	tenantID int,
	eventID int,
) (int, string) {
	t.Helper()

	var ownerID int
	var title string

	err := env.db.QueryRow(
		`SELECT owner_id, title
		FROM events
		WHERE tenant_id = $1 AND id = $2`,
		tenantID,
		eventID,
	).Scan(&ownerID, &title)

	if err != nil {
		t.Fatalf("Failed to query event state: %v", err)
	}

	return ownerID, title
}

// prüft die Erstellung eines Events und verhindert die Manipulation
// der Owner-Zuordnung durch vom Client übermittelte Daten
func TestIntegrationCreateEvent(t *testing.T) {
	env := newIntegrationTestEnvironment(t)

	owner, ownerToken := integrationCreateAttendeeOwner(t, env)
	member, memberToken := integrationCreateAttendeeMember(t, env, ownerToken)

	categoryID := integrationEnsureEventTestCategory(
		t,
		env,
		owner.User.TenantID,
	)

	// Der Member versucht, das Event dem Owner zuzuordnen
	payload := integrationEventPayload(
		"Created by Member",
		categoryID,
		owner.User.Id,
	)

	response := integrationDoEventRequest(
		t,
		env,
		http.MethodPost,
		"/api/v1/events",
		memberToken,
		payload,
	)
	defer response.Body.Close()

	integrationRequireStatus(t, response, http.StatusCreated)

	var createdEvent database.Event

	if err := decodeIntegrationResponse(response, &createdEvent); err != nil {
		t.Fatalf("Failed to decode event creation response: %v", err)
	}

	if createdEvent.Id <= 0 {
		t.Fatal("Expected a valid event ID")
	}

	if createdEvent.OwnerId != member.User.Id {
		t.Errorf(
			"Expected the authenticated member ID %d as owner, got %d",
			member.User.Id,
			createdEvent.OwnerId,
		)
	}

	if createdEvent.Title != payload.Title {
		t.Errorf(
			"Expected title %q, got %q",
			payload.Title,
			createdEvent.Title,
		)
	}

	if createdEvent.CategoryId != categoryID {
		t.Errorf(
			"Expected category ID %q, got %q",
			categoryID,
			createdEvent.CategoryId,
		)
	}

	// Die tatsächliche Tenant- und Owner-Zuordnung in PostgreSQL prüfen
	var storedTenantID int

	err := env.db.QueryRow(
		`SELECT tenant_id
		FROM events
		WHERE id = $1`,
		createdEvent.Id,
	).Scan(&storedTenantID)

	if err != nil {
		t.Fatalf("Failed to query created event tenant: %v", err)
	}

	if storedTenantID != member.User.TenantID {
		t.Errorf(
			"Expected tenant ID %d, got %d",
			member.User.TenantID,
			storedTenantID,
		)
	}

	storedOwnerID, storedTitle := integrationReadEventState(
		t,
		env,
		member.User.TenantID,
		createdEvent.Id,
	)

	if storedOwnerID != member.User.Id {
		t.Errorf(
			"Expected owner ID %d in PostgreSQL, got %d",
			member.User.Id,
			storedOwnerID,
		)
	}

	if storedTitle != payload.Title {
		t.Errorf(
			"Expected stored title %q, got %q",
			payload.Title,
			storedTitle,
		)
	}
}

// prüft die Event-Liste, die Tenant-Isolierung und den ownerId-Filter
func TestIntegrationGetAllEvents(t *testing.T) {
	env := newIntegrationTestEnvironment(t)

	owner, ownerToken := integrationCreateAttendeeOwner(t, env)
	member, _ := integrationCreateAttendeeMember(t, env, ownerToken)
	foreignOwner := integrationRegisterOwner(t, env)

	ownerEventID := integrationCreateEventFixture(
		t,
		env,
		owner.User.TenantID,
		owner.User.Id,
		"Owner Event",
		time.Date(2027, 6, 10, 10, 0, 0, 0, time.UTC),
	)

	memberEventID := integrationCreateEventFixture(
		t,
		env,
		owner.User.TenantID,
		member.User.Id,
		"Member Event",
		time.Date(2027, 6, 11, 10, 0, 0, 0, time.UTC),
	)

	foreignEventID := integrationCreateEventFixture(
		t,
		env,
		foreignOwner.User.TenantID,
		foreignOwner.User.Id,
		"Foreign Tenant Event",
		time.Date(2027, 6, 12, 10, 0, 0, 0, time.UTC),
	)

	t.Run("Returns only events belonging to the authenticated tenant", func(t *testing.T) {
		response := integrationDoEventRequest(
			t,
			env,
			http.MethodGet,
			"/api/v1/events",
			ownerToken,
			nil,
		)
		defer response.Body.Close()

		integrationRequireStatus(t, response, http.StatusOK)

		var events []database.Event

		if err := decodeIntegrationResponse(response, &events); err != nil {
			t.Fatalf("Failed to decode event list: %v", err)
		}

		if len(events) != 2 {
			t.Fatalf("Expected 2 tenant events, got %d", len(events))
		}

		foundEvents := make(map[int]bool)

		for _, event := range events {
			foundEvents[event.Id] = true
		}

		if !foundEvents[ownerEventID] {
			t.Error("Expected the owner's event in the result")
		}

		if !foundEvents[memberEventID] {
			t.Error("Expected the member's event in the result")
		}

		if foundEvents[foreignEventID] {
			t.Error("An event from another tenant must not be returned")
		}
	})

	t.Run("Filters events by ownerId within the tenant", func(t *testing.T) {
		path := fmt.Sprintf(
			"/api/v1/events?ownerId=%d",
			member.User.Id,
		)

		response := integrationDoEventRequest(
			t,
			env,
			http.MethodGet,
			path,
			ownerToken,
			nil,
		)
		defer response.Body.Close()

		integrationRequireStatus(t, response, http.StatusOK)

		var events []database.Event

		if err := decodeIntegrationResponse(response, &events); err != nil {
			t.Fatalf("Failed to decode filtered event list: %v", err)
		}

		if len(events) != 1 {
			t.Fatalf("Expected 1 member event, got %d", len(events))
		}

		if events[0].Id != memberEventID {
			t.Errorf(
				"Expected event ID %d, got %d",
				memberEventID,
				events[0].Id,
			)
		}

		if events[0].OwnerId != member.User.Id {
			t.Errorf(
				"Expected owner ID %d, got %d",
				member.User.Id,
				events[0].OwnerId,
			)
		}
	})

	t.Run("Rejects a non-positive ownerId", func(t *testing.T) {
		response := integrationDoEventRequest(
			t,
			env,
			http.MethodGet,
			"/api/v1/events?ownerId=0",
			ownerToken,
			nil,
		)
		defer response.Body.Close()

		integrationRequireStatus(t, response, http.StatusBadRequest)
	})
}

// prüft den Zugriff auf ein einzelnes Event und die Tenant-Isolierung
func TestIntegrationGetEvent(t *testing.T) {
	env := newIntegrationTestEnvironment(t)

	owner, ownerToken := integrationCreateAttendeeOwner(t, env)
	member, _ := integrationCreateAttendeeMember(t, env, ownerToken)
	foreignOwner := integrationRegisterOwner(t, env)

	memberEventID := integrationCreateEventFixture(
		t,
		env,
		owner.User.TenantID,
		member.User.Id,
		"Member Event",
		time.Date(2027, 7, 10, 10, 0, 0, 0, time.UTC),
	)

	foreignEventID := integrationCreateEventFixture(
		t,
		env,
		foreignOwner.User.TenantID,
		foreignOwner.User.Id,
		"Foreign Tenant Event",
		time.Date(2027, 7, 11, 10, 0, 0, 0, time.UTC),
	)

	t.Run("A tenant user can retrieve another user's event", func(t *testing.T) {
		response := integrationDoEventRequest(
			t,
			env,
			http.MethodGet,
			fmt.Sprintf("/api/v1/events/%d", memberEventID),
			ownerToken,
			nil,
		)
		defer response.Body.Close()

		integrationRequireStatus(t, response, http.StatusOK)

		var event database.Event

		if err := decodeIntegrationResponse(response, &event); err != nil {
			t.Fatalf("Failed to decode event response: %v", err)
		}

		if event.Id != memberEventID {
			t.Errorf(
				"Expected event ID %d, got %d",
				memberEventID,
				event.Id,
			)
		}

		if event.OwnerId != member.User.Id {
			t.Errorf(
				"Expected owner ID %d, got %d",
				member.User.Id,
				event.OwnerId,
			)
		}

		if event.Owner == nil {
			t.Fatal("Expected the event owner information in the response")
		}

		if event.Owner.Id != member.User.Id {
			t.Errorf(
				"Expected owner information for user %d, got %d",
				member.User.Id,
				event.Owner.Id,
			)
		}
	})

	t.Run("An event from another tenant is not accessible", func(t *testing.T) {
		response := integrationDoEventRequest(
			t,
			env,
			http.MethodGet,
			fmt.Sprintf("/api/v1/events/%d", foreignEventID),
			ownerToken,
			nil,
		)
		defer response.Body.Close()

		integrationRequireStatus(t, response, http.StatusNotFound)
	})
}

// prüft die Aktualisierung eines eigenen Events sowie Berechtigungsfehler und die Tenant-Isolierung
func TestIntegrationUpdateEvent(t *testing.T) {
	env := newIntegrationTestEnvironment(t)

	owner, ownerToken := integrationCreateAttendeeOwner(t, env)
	member, memberToken := integrationCreateAttendeeMember(t, env, ownerToken)
	foreignOwner := integrationRegisterOwner(t, env)

	memberEventID := integrationCreateEventFixture(
		t,
		env,
		owner.User.TenantID,
		member.User.Id,
		"Original Event Title",
		time.Date(2027, 9, 10, 10, 0, 0, 0, time.UTC),
	)

	foreignEventID := integrationCreateEventFixture(
		t,
		env,
		foreignOwner.User.TenantID,
		foreignOwner.User.Id,
		"Foreign Event Title",
		time.Date(2027, 9, 11, 10, 0, 0, 0, time.UTC),
	)

	categoryID := integrationEnsureEventTestCategory(
		t,
		env,
		member.User.TenantID,
	)

	t.Run("Event owner can update their own event", func(t *testing.T) {
		// Der Body enthält absichtlich eine falsche Event-ID und Owner-ID.
		// Beide Werte müssen durch Path und Authentifizierung überschrieben werden.
		payload := integrationEventPayload(
			"Updated Event Title",
			categoryID,
			owner.User.Id,
		)
		payload.Id = foreignEventID

		response := integrationDoEventRequest(
			t,
			env,
			http.MethodPut,
			fmt.Sprintf("/api/v1/events/%d", memberEventID),
			memberToken,
			payload,
		)
		defer response.Body.Close()

		integrationRequireStatus(t, response, http.StatusOK)

		var updatedEvent database.Event

		if err := decodeIntegrationResponse(response, &updatedEvent); err != nil {
			t.Fatalf("Failed to decode updated event: %v", err)
		}

		if updatedEvent.Id != memberEventID {
			t.Errorf(
				"Expected event ID %d, got %d",
				memberEventID,
				updatedEvent.Id,
			)
		}

		if updatedEvent.OwnerId != member.User.Id {
			t.Errorf(
				"Expected owner ID %d, got %d",
				member.User.Id,
				updatedEvent.OwnerId,
			)
		}

		if updatedEvent.Title != payload.Title {
			t.Errorf(
				"Expected updated title %q, got %q",
				payload.Title,
				updatedEvent.Title,
			)
		}

		storedOwnerID, storedTitle := integrationReadEventState(
			t,
			env,
			member.User.TenantID,
			memberEventID,
		)

		if storedOwnerID != member.User.Id {
			t.Errorf(
				"Expected owner ID %d in PostgreSQL, got %d",
				member.User.Id,
				storedOwnerID,
			)
		}

		if storedTitle != payload.Title {
			t.Errorf(
				"Expected stored title %q, got %q",
				payload.Title,
				storedTitle,
			)
		}
	})

	t.Run("Another user from the same tenant cannot update the event", func(t *testing.T) {
		payload := integrationEventPayload(
			"Unauthorized Update",
			categoryID,
			owner.User.Id,
		)

		response := integrationDoEventRequest(
			t,
			env,
			http.MethodPut,
			fmt.Sprintf("/api/v1/events/%d", memberEventID),
			ownerToken,
			payload,
		)
		defer response.Body.Close()

		integrationRequireStatus(t, response, http.StatusForbidden)

		storedOwnerID, storedTitle := integrationReadEventState(
			t,
			env,
			member.User.TenantID,
			memberEventID,
		)

		if storedOwnerID != member.User.Id {
			t.Error("A forbidden request must not change the event owner")
		}

		if storedTitle != "Updated Event Title" {
			t.Errorf(
				"A forbidden request must not change the event title, got %q",
				storedTitle,
			)
		}
	})

	t.Run("An event from another tenant cannot be updated", func(t *testing.T) {
		payload := integrationEventPayload(
			"Foreign Event Update",
			categoryID,
			member.User.Id,
		)

		response := integrationDoEventRequest(
			t,
			env,
			http.MethodPut,
			fmt.Sprintf("/api/v1/events/%d", foreignEventID),
			memberToken,
			payload,
		)
		defer response.Body.Close()

		integrationRequireStatus(t, response, http.StatusNotFound)

		_, storedTitle := integrationReadEventState(
			t,
			env,
			foreignOwner.User.TenantID,
			foreignEventID,
		)

		if storedTitle != "Foreign Event Title" {
			t.Error("A request from another tenant must not modify the foreign event")
		}
	})
}

// prüft das Löschen eines eigenen Events sowie Berechtigungsfehler und die Tenant-Isolierung
func TestIntegrationDeleteEvent(t *testing.T) {
	env := newIntegrationTestEnvironment(t)

	owner, ownerToken := integrationCreateAttendeeOwner(t, env)
	member, memberToken := integrationCreateAttendeeMember(t, env, ownerToken)
	foreignOwner := integrationRegisterOwner(t, env)

	memberEventID := integrationCreateEventFixture(
		t,
		env,
		owner.User.TenantID,
		member.User.Id,
		"Member Delete Event",
		time.Date(2027, 10, 10, 10, 0, 0, 0, time.UTC),
	)

	foreignEventID := integrationCreateEventFixture(
		t,
		env,
		foreignOwner.User.TenantID,
		foreignOwner.User.Id,
		"Foreign Delete Event",
		time.Date(2027, 10, 11, 10, 0, 0, 0, time.UTC),
	)

	t.Run("Another user from the same tenant cannot delete the event", func(t *testing.T) {
		response := integrationDoEventRequest(
			t,
			env,
			http.MethodDelete,
			fmt.Sprintf("/api/v1/events/%d", memberEventID),
			ownerToken,
			nil,
		)
		defer response.Body.Close()

		integrationRequireStatus(t, response, http.StatusForbidden)

		if !integrationEventExists(
			t,
			env,
			member.User.TenantID,
			memberEventID,
		) {
			t.Error("A forbidden request must not delete the event")
		}
	})

	t.Run("An event from another tenant cannot be deleted", func(t *testing.T) {
		response := integrationDoEventRequest(
			t,
			env,
			http.MethodDelete,
			fmt.Sprintf("/api/v1/events/%d", foreignEventID),
			memberToken,
			nil,
		)
		defer response.Body.Close()

		integrationRequireStatus(t, response, http.StatusNotFound)

		if !integrationEventExists(
			t,
			env,
			foreignOwner.User.TenantID,
			foreignEventID,
		) {
			t.Error("A request from another tenant must not delete the foreign event")
		}
	})

	t.Run("Event owner can delete their own event", func(t *testing.T) {
		response := integrationDoEventRequest(
			t,
			env,
			http.MethodDelete,
			fmt.Sprintf("/api/v1/events/%d", memberEventID),
			memberToken,
			nil,
		)
		defer response.Body.Close()

		integrationRequireStatus(t, response, http.StatusNoContent)

		if integrationEventExists(
			t,
			env,
			member.User.TenantID,
			memberEventID,
		) {
			t.Error("Expected the event to be deleted from PostgreSQL")
		}
	})
}
