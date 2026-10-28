package database

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

var ErrAttendeeAlreadyExists = errors.New("attendee already exists")

type AttendeeModel struct {
	DB *sql.DB
}

type Attendee struct {
	Id      int `json:"id"`
	UserId  int `json:"userId"`
	EventId int `json:"eventId"`
}

func (m *AttendeeModel) Insert(
	ctx context.Context,
	tenantID int,
	attendee *Attendee,
) error {
	if tenantID <= 0 || attendee == nil || attendee.EventId <= 0 || attendee.UserId <= 0 {
		return ErrInvalidEventScope
	}

	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	tx, err := m.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var valid int

	err = tx.QueryRowContext(ctx, `
		SELECT
			EXISTS (
				SELECT 1 FROM events
				WHERE tenant_id = ? AND id = ?
			)
			AND EXISTS (
				SELECT 1 FROM users
				WHERE tenant_id = ?
				  AND id = ?
				  AND status = 'active'
			)
	`,
		tenantID, attendee.EventId,
		tenantID, attendee.UserId,
	).Scan(&valid)
	if err != nil {
		return err
	}

	if valid != 1 {
		return sql.ErrNoRows
	}

	result, err := tx.ExecContext(ctx, `
		INSERT INTO attendees (tenant_id, event_id, user_id)
		VALUES (?, ?, ?)
		ON CONFLICT (tenant_id, event_id, user_id) DO NOTHING
	`,
		tenantID,
		attendee.EventId,
		attendee.UserId,
	)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rowsAffected == 0 {
		return ErrAttendeeAlreadyExists
	}

	id, err := result.LastInsertId()
	if err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	attendee.Id = int(id)
	return nil
}

func (m *AttendeeModel) GetByEventAndAttendee(
	ctx context.Context,
	tenantID int,
	eventID int,
	userID int,
) (*Attendee, error) {

	if tenantID <= 0 || eventID <= 0 || userID <= 0 {
		return nil, ErrInvalidEventScope
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	query := `
		SELECT id, event_id, user_id
		FROM attendees
		WHERE tenant_id = ?
		  AND event_id = ?
		  AND user_id = ?
	`

	var attendee Attendee

	err := m.DB.QueryRowContext(ctx, query, tenantID, eventID, userID).Scan(
		&attendee.Id,
		&attendee.EventId,
		&attendee.UserId,
	)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return &attendee, nil
}

func (m *AttendeeModel) GetAttendeesByEvent(
	ctx context.Context,
	tenantID int,
	eventID int,
) ([]*User, error) {
	if tenantID <= 0 || eventID <= 0 {
		return nil, ErrInvalidEventScope
	}

	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	query := `
		SELECT u.id, u.name, COALESCE(u.avatar_url, '')
		FROM attendees a
		JOIN users u
			ON u.tenant_id = a.tenant_id
			AND u.id = a.user_id
		WHERE a.tenant_id = ?
		  AND a.event_id = ?
		ORDER BY u.name COLLATE NOCASE
	`

	rows, err := m.DB.QueryContext(ctx, query, tenantID, eventID)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	users := make([]*User, 0)

	for rows.Next() {
		var user User

		if err := rows.Scan(&user.Id, &user.Name, &user.AvatarUrl); err != nil {
			return nil, err
		}
		users = append(users, &user)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return users, nil
}

func (m *AttendeeModel) Delete(
	ctx context.Context,
	tenantID int,
	userID int,
	eventID int,
) error {
	if tenantID <= 0 || userID <= 0 || eventID <= 0 {
		return ErrInvalidEventScope
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	result, err := m.DB.ExecContext(ctx, `
		DELETE FROM attendees
		WHERE tenant_id = ?
		  AND user_id = ?
		  AND event_id = ?
	`,
		tenantID,
		userID,
		eventID,
	)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rowsAffected == 0 {
		return sql.ErrNoRows
	}

	return nil
}

func (m *AttendeeModel) GetEventsByAttendee(
	ctx context.Context,
	tenantID int,
	attendeeID int,
) ([]*Event, error) {
	if tenantID <= 0 || attendeeID <= 0 {
		return nil, ErrInvalidEventScope
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	query := `
		SELECT
			e.id,
			e.owner_id,
			e.title,
			e.description,
			e.image_url,
			e.location,
			e.start_date_time,
			e.end_date_time,
			e.category_id,
			e.url,
			COALESCE(u.name, ''),
			COALESCE(c.name, '')
		FROM attendees a
		JOIN events e
			ON e.tenant_id = a.tenant_id
			AND e.id = a.event_id
		JOIN users u
			ON u.tenant_id = e.tenant_id
			AND u.id = e.owner_id
		LEFT JOIN categories c
		ON c.tenant_id = e.tenant_id
		AND c.id = e.category_id	
		WHERE a.tenant_id = ?
		  AND a.user_id = ?
		ORDER BY e.start_date_time ASC
	`

	rows, err := m.DB.QueryContext(ctx, query, tenantID, attendeeID)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	events := make([]*Event, 0)

	for rows.Next() {
		event, err := scanEvent(rows)
		if err != nil {
			return nil, err
		}

		events = append(events, event)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return events, nil
}
