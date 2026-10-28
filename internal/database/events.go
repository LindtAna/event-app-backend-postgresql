package database

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

var ErrInvalidEventScope = errors.New("invalid tenant or owner context")

type EventModel struct {
	DB *sql.DB
}

type Owner struct {
	Id   int    `json:"id"`
	Name string `json:"name"`
}

type Event struct {
	Id            int    `json:"id"`
	OwnerId       int    `json:"ownerId"`
	Owner         *Owner `json:"owner,omitempty"`
	Title         string `json:"title" binding:"required,min=3,max=100"`
	Description   string `json:"description" binding:"required,min=3,max=400"`
	ImageUrl      string `json:"imageUrl"`
	Location      string `json:"location" binding:"required,min=3,max=200"`
	StartDateTime string `json:"startDateTime" binding:"required"`
	EndDateTime   string `json:"endDateTime" binding:"required"`
	CategoryId    string `json:"categoryId"`
	CategoryName  string `json:"categoryName,omitempty"`
	Url           string `json:"url"`
}

type eventScanner interface {
	Scan(dest ...any) error
}

const eventSelect = `
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
	FROM events e
	LEFT JOIN users u
	ON u.tenant_id = e.tenant_id
	AND u.id = e.owner_id
	LEFT JOIN categories c
	ON c.tenant_id = e.tenant_id
	AND c.id = e.category_id
`

func scanEvent(row eventScanner) (*Event, error) {
	var event Event
	var ownerName string
	var categoryName string

	err := row.Scan(
		&event.Id,
		&event.OwnerId,
		&event.Title,
		&event.Description,
		&event.ImageUrl,
		&event.Location,
		&event.StartDateTime,
		&event.EndDateTime,
		&event.CategoryId,
		&event.Url,
		&ownerName,
		&categoryName,
	)
	if err != nil {
		return nil, err
	}

	if event.OwnerId > 0 {
		event.Owner = &Owner{
			Id:   event.OwnerId,
			Name: ownerName,
		}
	}

	if categoryName != "" {
		event.CategoryName = categoryName
	}

	return &event, nil
}

func (m *EventModel) GetAll(
	ctx context.Context,
	tenantID int,
) ([]*Event, error) {
	return m.queryEvents(ctx, tenantID, nil)
}

func (m *EventModel) queryEvents(
	ctx context.Context,
	tenantID int,
	ownerID *int,
) ([]*Event, error) {
	if tenantID <= 0 {
		return nil, ErrInvalidEventScope
	}

	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	query := eventSelect + `
		WHERE e.tenant_id = ?
	`
	args := []any{tenantID}

	if ownerID != nil {
		query += ` AND e.owner_id = ?`
		args = append(args, *ownerID)
	}

	query += ` ORDER BY e.start_date_time ASC`

	rows, err := m.DB.QueryContext(ctx, query, args...)
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

func (m *EventModel) Get(
	ctx context.Context,
	tenantID int,
	eventID int,
) (*Event, error) {
	if tenantID <= 0 || eventID <= 0 {
		return nil, ErrInvalidEventScope
	}

	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	query := eventSelect + `
		WHERE e.tenant_id = ? AND e.id = ?
	`

	event, err := scanEvent(
		m.DB.QueryRowContext(ctx, query, tenantID, eventID),
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return event, nil
}

func (m *EventModel) GetByOwnerID(
	ctx context.Context,
	tenantID int,
	ownerID int,
) ([]*Event, error) {
	if ownerID <= 0 {
		return nil, ErrInvalidEventScope
	}

	return m.queryEvents(ctx, tenantID, &ownerID)
}
