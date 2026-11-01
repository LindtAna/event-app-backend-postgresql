package database

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"strings"
	"time"
)

var ErrInvalidCategory = errors.New("invalid or missing category")

// erstellt ein Event und löst dessen Kategorie innerhalb derselben Transaktion auf
func (m *EventModel) CreateWithCategory(ctx context.Context, tenantID int, event *Event) error {
	if tenantID <= 0 || event == nil || event.OwnerId <= 0 {
		return ErrInvalidEventScope
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	tx, err := m.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	categoryID, err := resolveCategory(ctx, tx, tenantID, event)
	if err != nil {
		return err
	}

	var eventID int

	err = tx.QueryRowContext(ctx, `
		INSERT INTO events (
			tenant_id, owner_id, title, description, image_url,
			location, start_date_time, end_date_time, category_id, url
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	    RETURNING id
	`,
		tenantID,
		event.OwnerId,
		event.Title,
		event.Description,
		event.ImageUrl,
		event.Location,
		event.StartDateTime,
		event.EndDateTime,
		categoryID,
		event.Url,
	).Scan(&eventID)

	if err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	event.Id = eventID
	event.CategoryId = categoryID
	event.CategoryName = ""

	return nil

}

// Aktualisiert ein Event und löst dessen Kategorie innerhalb derselben Transaktion auf
// Eine ungenutzte benutzerdefinierte Kategorie wird entfernt
func (m *EventModel) UpdateWithCategory(
	ctx context.Context, tenantID int,
	ownerID int, event *Event) error {
	if tenantID <= 0 || ownerID <= 0 || event == nil || event.Id <= 0 {
		return ErrInvalidEventScope
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	tx, err := m.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var oldCategoryID string

	err = tx.QueryRowContext(
		ctx,
		`SELECT category_id FROM events
		WHERE tenant_id = $1 AND id = $2 AND owner_id = $3`,
		tenantID,
		event.Id,
		ownerID,
	).Scan(&oldCategoryID)
	if err != nil {
		return err
	}

	newCategoryID, err := resolveCategory(ctx, tx, tenantID, event)
	if err != nil {
		return err
	}

	result, err := tx.ExecContext(ctx, `
		UPDATE events
		SET title = $1,
	    description = $2,
	    image_url = $3,
	    location = $4,
	    start_date_time = $5,
	    end_date_time = $6,
	    category_id = $7,
	    url = $8
	    WHERE tenant_id = $9 AND id = $10 AND owner_id = $11
	`,
		event.Title,
		event.Description,
		event.ImageUrl,
		event.Location,
		event.StartDateTime,
		event.EndDateTime,
		newCategoryID,
		event.Url,
		tenantID,
		event.Id,
		ownerID,
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

	if oldCategoryID != newCategoryID {
		if err := deleteUnusedCustomCategory(ctx, tx, tenantID, oldCategoryID); err != nil {
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	event.OwnerId = ownerID
	event.CategoryId = newCategoryID
	event.CategoryName = ""

	return nil

}

// löscht ein Event und entfernt dessen benutzerdefinierte Kategorie
// falls kein anderes Event diese Kategorie verwendet
func (m *EventModel) DeleteWithCategory(
	ctx context.Context,
	tenantID int,
	ownerID int,
	eventID int,
) error {
	if tenantID <= 0 || ownerID <= 0 || eventID <= 0 {
		return ErrInvalidEventScope
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	tx, err := m.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var categoryID string

	err = tx.QueryRowContext(
		ctx,
		`SELECT category_id FROM events
		WHERE tenant_id = $1 AND id = $2 AND owner_id = $3`,
		tenantID,
		eventID,
		ownerID,
	).Scan(&categoryID)

	if err != nil {
		return err
	}

	result, err := tx.ExecContext(
		ctx,
		`DELETE FROM events
		WHERE tenant_id = $1 AND id = $2 AND owner_id = $3`,
		tenantID,
		eventID,
		ownerID,
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

	if err := deleteUnusedCustomCategory(ctx, tx, tenantID, categoryID); err != nil {
		return err
	}

	return tx.Commit()

}

// normalizeCategoryName vereinheitlicht Leerzeichen und Groß-/Kleinschreibung.
func normalizeCategoryName(name string) string {
	return strings.ToLower(strings.Join(strings.Fields(name), " "))
}

// Gibt eine vorhandene Kategorie des Tenants zurück
// oder erstellt eine benutzerdefinierte Kategorie für diesen Tenant
// CategoryId || CategoryName, nicht beides
func resolveCategory(
	ctx context.Context,
	tx *sql.Tx,
	tenantID int,
	event *Event,
) (string, error) {
	if tenantID <= 0 || event == nil {
		return "", ErrInvalidEventScope
	}
	categoryName := strings.Join(strings.Fields(event.CategoryName), " ")
	categoryID := strings.TrimSpace(event.CategoryId)

	if categoryName != "" && categoryID != "" {
		return "", ErrInvalidCategory
	}

	if categoryName != "" {
		nameKey := normalizeCategoryName(categoryName)

		var existingID string

		err := tx.QueryRowContext(
			ctx,
			`SELECT id FROM categories
			WHERE tenant_id = $1 AND name_key = $2`,
			tenantID,
			nameKey,
		).Scan(&existingID)

		if err == nil {
			return existingID, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return "", err
		}

		randomBytes := make([]byte, 16)
		if _, err := rand.Read(randomBytes); err != nil {
			return "", err
		}
		newID := hex.EncodeToString(randomBytes)

		_, err = tx.ExecContext(
			ctx,
			`INSERT INTO categories (tenant_id, id, name, name_key, is_default)
			VALUES ($1, $2, $3, $4, 0)`,
			tenantID,
			newID,
			categoryName,
			nameKey,
		)
		if err != nil {
			return "", err
		}

		return newID, nil
	}

	if categoryID == "" {
		return "", ErrInvalidCategory
	}

	var existingID string
	err := tx.QueryRowContext(
		ctx,
		`SELECT id FROM categories
		WHERE tenant_id = $1 AND id = $2`,
		tenantID,
		categoryID,
	).Scan(&existingID)

	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrInvalidCategory
	}
	if err != nil {
		return "", err
	}

	return existingID, nil

}

// Standardkategorie wird niemals gelöscht
func deleteUnusedCustomCategory(
	ctx context.Context,
	tx *sql.Tx,
	tenantID int,
	categoryID string,
) error {

	_, err := tx.ExecContext(
		ctx,
		`DELETE FROM categories
         WHERE tenant_id = $1 AND id = $2 AND is_default = 0
         AND NOT EXISTS ( SELECT 1 FROM events
         WHERE tenant_id = $3 AND category_id = $4 )`,
		tenantID,
		categoryID,
		tenantID,
		categoryID,
	)

	return err

}
