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

	result, err := tx.ExecContext(ctx, `
		INSERT INTO events (
			tenant_id, owner_id, title, description, image_url,
			location, start_date_time, end_date_time, category_id, url
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
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
	)
	if err != nil {
		return err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	event.Id = int(id)
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
		WHERE tenant_id = ? AND id = ? AND owner_id = ?`,
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
		SET title = ?,
		    description = ?,
		    image_url = ?,
		    location = ?,
		    start_date_time = ?,
		    end_date_time = ?,
		    category_id = ?,
		    url = ?
		WHERE tenant_id = ? AND id = ? AND owner_id = ?
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
		WHERE tenant_id = ? AND id = ? AND owner_id = ?`,
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
		WHERE tenant_id = ? AND id = ? AND owner_id = ?`,
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
			WHERE tenant_id = ? AND name_key = ?`,
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
			`INSERT INTO categories (tenant_id, id, name, name_key, is_default) VALUES (?, ?, ?, ?, 0)`,
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
		WHERE tenant_id = ? AND id = ?`,
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
		WHERE tenant_id = ? AND id = ? AND is_default = 0
		AND NOT EXISTS ( SELECT 1 FROM events
		WHERE tenant_id = ? AND category_id = ? )`,
		tenantID,
		categoryID,
		tenantID,
		categoryID,
	)

	return err

}
