package database

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	sqlite "modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

// Standardisierte Fehler für die Mandantenverwaltung.
var (
	ErrTenantNotFound  = errors.New("tenant not found")
	ErrTenantNameTaken = errors.New("company name or slug already exists")
	ErrInvalidTenant   = errors.New("invalid company name")
)

type TenantModel struct {
	DB *sql.DB
}

// Tenant repräsentiert eine Organisation bzw. ein Unternehmen.
type Tenant struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	NameKey   string `json:"-"`
	Slug      string `json:"slug"`
	Status    string `json:"status"`
	CreatedAt string `json:"createdAt"`
}

// normalizeTenantName normalisiert Leerzeichen und Groß-/Kleinschreibung
// um einen einheitlichen Schlüssel (name_key) für Duplikatsprüfungen zu erzeugen
func normalizeTenantName(name string) string {
	return strings.Join(strings.Fields(strings.ToLower(name)), " ")
}

// isValidTenantName prüft den Mandantennamen anhand einer strikten Whitelist,
// um ungültige Zeichen oder Injection-Angriffe im Namen zu verhindern
func isValidTenantName(name string) bool {
	hasAlphaNumeric := false

	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z',
			r >= 'A' && r <= 'Z',
			r >= '0' && r <= '9':
			hasAlphaNumeric = true

		case r == ' ',
			r == '-',
			r == '.',
			r == ',',
			r == '\'',
			r == '&':
			// Erlaubte Sonderzeichen.

		default:
			return false
		}
	}

	return hasAlphaNumeric
}

// slugify erzeugt einen URL-konformen Slug aus ASCII-Zeichen,
// indem Sonderzeichen entfernt und Leerzeichen durch Bindestriche ersetzt werden
func slugify(name string) string {
	var b strings.Builder
	lastHyphen := false

	for _, r := range strings.ToLower(strings.TrimSpace(name)) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastHyphen = false
			continue
		}

		if b.Len() > 0 && !lastHyphen {
			b.WriteByte('-')
			lastHyphen = true
		}
	}

	return strings.Trim(b.String(), "-")
}

// erkennt SQLite-Verletzungen von
// UNIQUE- und PRIMARY KEY-Constraints, um diese in domänenspezifische Fehler zu übersetzen.
func isTenantUniqueConstraintError(err error) bool {
	var sqliteErr *sqlite.Error
	if !errors.As(err, &sqliteErr) {
		return false
	}

	switch sqliteErr.Code() {
	case sqlite3.SQLITE_CONSTRAINT_UNIQUE,
		sqlite3.SQLITE_CONSTRAINT_PRIMARYKEY:
		return true
	default:
		return false
	}
}

// erstellt atomar einen Mandanten und dessen Owner
// Der übergebene Owner wird erst nach erfolgreichem Commit aktualisiert
func (m *TenantModel) CreateWithOwner(
	ctx context.Context,
	name string,
	owner *User,
) (*Tenant, error) {
	if owner == nil {
		return nil, ErrInvalidUser
	}

	// Mehrfache Leerzeichen bereinigen, während die Original-Groß-/Kleinschreibung erhalten bleibt
	cleanName := strings.Join(strings.Fields(name), " ")

	if cleanName == "" || !isValidTenantName(cleanName) {
		return nil, ErrInvalidTenant
	}

	// nameKey für eindeutige Indexprüfung und slug für URLs generieren
	nameKey := normalizeTenantName(cleanName)
	slug := slugify(cleanName)

	if nameKey == "" || slug == "" {
		return nil, ErrInvalidTenant
	}

	// Ein Timeout ergänzen, falls der Aufrufer keines gesetzt hat
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
	}

	// Transaktion starten, um Atomarität zu gewährleisten
	// Tenant und Owner müssen gemeinsam persistiert werden
	tx, err := m.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	} // Automatischer Rollback, falls die Transaktion nicht erfolgreich committed wird
	defer tx.Rollback()

	// Mandanten in die Datenbank einfügen
	// UNIQUE-Constraints greifen zuverlässig bei parallelen Aufrufen
	_, err = tx.ExecContext(ctx, `
		INSERT INTO tenants (name, name_key, slug, status)
		VALUES (?, ?, ?, 'active')
	`, cleanName, nameKey, slug)
	if err != nil {
		if isTenantUniqueConstraintError(err) {
			return nil, ErrTenantNameTaken
		}
		return nil, err
	}

	var tenantID int
	err = tx.QueryRowContext(ctx, `
		SELECT id
		FROM tenants
		WHERE name_key = ?
	`, nameKey).Scan(&tenantID)
	if err != nil {
		return nil, err
	}

	// Mit einer lokalen Kopie arbeiten, damit ein fehlgeschlagener
	// Commit den übergebenen Owner-Pointer nicht mit unvollständigen Daten manipuliert
	ownerToInsert := *owner
	ownerToInsert.TenantID = tenantID
	ownerToInsert.Role = "owner"
	ownerToInsert.Status = "active"

	// Owner im Rahmen derselben Transaktion anlegen
	if err := insertUserTx(ctx, tx, &ownerToInsert); err != nil {
		return nil, err
	}

	// Vollständigen Tenant-Datensatz inklusive generierter Felder auslesen
	var tenant Tenant
	err = tx.QueryRowContext(ctx, `
		SELECT id, name, name_key, slug, status, created_at
		FROM tenants
		WHERE id = ?
	`, tenantID).Scan(
		&tenant.ID,
		&tenant.Name,
		&tenant.NameKey,
		&tenant.Slug,
		&tenant.Status,
		&tenant.CreatedAt,
	)
	if err != nil {
		return nil, err
	}

	// Transaktion erfolgreich abschließen.
	if err := tx.Commit(); err != nil {
		return nil, err
	}

	// Den ursprünglichen Owner erst nach erfolgreichem Commit aktualisieren
	*owner = ownerToInsert

	return &tenant, nil
}

// Get ruft einen Mandanten anhand seiner ID aus der Datenbank ab
// Gibt ErrTenantNotFound zurück, falls kein entsprechender Datensatz existiert
func (m *TenantModel) Get(ctx context.Context, id int) (*Tenant, error) {
	var tenant Tenant

	err := m.DB.QueryRowContext(ctx, `
		SELECT id, name, name_key, slug, status, created_at
		FROM tenants
		WHERE id = ?
	`, id).Scan(
		&tenant.ID,
		&tenant.Name,
		&tenant.NameKey,
		&tenant.Slug,
		&tenant.Status,
		&tenant.CreatedAt,
	)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrTenantNotFound
	}
	if err != nil {
		return nil, err
	}

	return &tenant, nil
}
