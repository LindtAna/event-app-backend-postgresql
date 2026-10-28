package database

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
	"time"
)

// ErrInvalidInvitation wird zurückgegeben, wenn eine Einladung ungültig,
// abgelaufen, bereits verwendet oder widerrufen ist
var ErrInvalidInvitation = errors.New(
	"invalid, expired, used or revoked invitation",
)

type InvitationModel struct {
	DB *sql.DB
}

type Invitation struct {
	ID        int     `json:"id"`
	TenantID  int     `json:"tenantId"`
	ExpiresAt string  `json:"expiresAt"`
	UsedAt    *string `json:"usedAt,omitempty"`
	RevokedAt *string `json:"revokedAt,omitempty"`
	CreatedBy int     `json:"createdBy"`
	CreatedAt string  `json:"createdAt"`
}

// hashInvitationToken generiert einen SHA-256-Hash des Rohtokens
// Dies stellt sicher, dass Tokens niemals im Klartext in der Datenbank gespeichert werden
func hashInvitationToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// erstellt eine neue Einladung und gibt das rohe Token nur einmal zurück
// In der Datenbank wird ausschließlich dessen Hash gespeichert
func (m *InvitationModel) Create(
	ctx context.Context,
	tenantID, createdBy int,
	expiresAt time.Time,
) (*Invitation, string, error) {
	// Validierung der Eingangsparameter, um ungültige DB-Schreibvorgänge zu verhindern
	if tenantID <= 0 ||
		createdBy <= 0 ||
		!expiresAt.After(time.Now()) {
		return nil, "", ErrInvalidInvitation
	}

	// Kryptografisch sichere Zufallsbytes für das Einladungs-Token generieren
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return nil, "", err
	}

	// Das Token URL-sicher kodieren und den Hash für die DB-Speicherung berechnen
	rawToken := base64.RawURLEncoding.EncodeToString(tokenBytes)
	tokenHash := hashInvitationToken(rawToken)

	// Timeout für den Datenbankvorgang setzen, um hängende Goroutinen zu vermeiden
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	expiration := expiresAt.UTC().Format("2006-01-02 15:04:05")

	tx, err := m.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, "", err
	}
	defer tx.Rollback()

	result, err := tx.ExecContext(ctx, `
		INSERT INTO invitations (
			tenant_id,
			token_hash,
			expires_at,
			created_by
		)
		VALUES (?, ?, ?, ?)
	`,
		tenantID,
		tokenHash,
		expiration,
		createdBy,
	)
	if err != nil {
		return nil, "", err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, "", err
	}

	var createdAt string
	err = tx.QueryRowContext(ctx, `
		SELECT created_at
		FROM invitations
		WHERE id = ? AND tenant_id = ?
	`, id, tenantID).Scan(&createdAt)
	if err != nil {
		return nil, "", err
	}

	invitation := &Invitation{
		ID:        int(id),
		TenantID:  tenantID,
		ExpiresAt: expiration,
		CreatedBy: createdBy,
		CreatedAt: createdAt,
	}

	if err := tx.Commit(); err != nil {
		return nil, "", err
	}
	return invitation, rawToken, nil
}

// ListByTenant ruft alle Einladungen für einen bestimmten Mandanten (Tenant) ab,
// sortiert nach dem Erstellungsdatum in absteigender Reihenfolge
func (m *InvitationModel) ListByTenant(
	ctx context.Context,
	tenantID int,
) ([]*Invitation, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	rows, err := m.DB.QueryContext(ctx, `
		SELECT
			id,
			tenant_id,
			expires_at,
			used_at,
			revoked_at,
			created_by,
			created_at
		FROM invitations
		WHERE tenant_id = ?
		ORDER BY created_at DESC
	`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var invitations []*Invitation

	for rows.Next() {
		var invitation Invitation
		var usedAt, revokedAt sql.NullString

		if err := rows.Scan(
			&invitation.ID,
			&invitation.TenantID,
			&invitation.ExpiresAt,
			&usedAt,
			&revokedAt,
			&invitation.CreatedBy,
			&invitation.CreatedAt,
		); err != nil {
			return nil, err
		}

		// Nullable Datenbankfelder sicher in Pointer konvertieren
		if usedAt.Valid {
			invitation.UsedAt = &usedAt.String
		}
		if revokedAt.Valid {
			invitation.RevokedAt = &revokedAt.String
		}

		invitations = append(invitations, &invitation)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return invitations, nil
}

// widerruft eine aktive Einladung manuell, sofern sie noch nicht verwendet,
// abgelaufen oder bereits widerrufen wurde
func (m *InvitationModel) Revoke(
	ctx context.Context,
	tenantID, invitationID int,
) error {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	result, err := m.DB.ExecContext(ctx, `
		UPDATE invitations
		SET revoked_at = CURRENT_TIMESTAMP
		WHERE tenant_id = ?
		  AND id = ?
		  AND used_at IS NULL
		  AND revoked_at IS NULL
		  AND datetime(expires_at) > datetime('now')
	`, tenantID, invitationID)
	if err != nil {
		return err
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	// Wenn keine Zeile aktualisiert wurde, existiert die Einladung entweder nicht
	// oder sie ist bereits ungültig/verbraucht
	if affected == 0 {
		return ErrInvalidInvitation
	}

	return nil
}

// erstellt ein neues Mitglied
// verbraucht die Einladung atomar in einer Transaktion
// TenantID und Rolle des Nutzers werden dabei überschrieben, um Manipulationen zu verhindern
func (m *InvitationModel) RegisterMember(
	ctx context.Context,
	rawToken string,
	user *User,
) error {
	if strings.TrimSpace(rawToken) == "" || user == nil {
		return ErrInvalidInvitation
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// Transaktion starten, um Konsistenz zwischen
	// Einladungsverbrauch und Benutzererstellung zu garantieren
	tx, err := m.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var invitationID, tenantID int

	// Gültigkeit des Tokens prüfen
	// IDs innerhalb der Transaktion sperren/abrufen
	err = tx.QueryRowContext(ctx, `
		SELECT id, tenant_id
		FROM invitations
		WHERE token_hash = ?
		  AND used_at IS NULL
		  AND revoked_at IS NULL
		  AND datetime(expires_at) > datetime('now')
	`, hashInvitationToken(rawToken)).Scan(
		&invitationID,
		&tenantID,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrInvalidInvitation
	}
	if err != nil {
		return err
	}

	// Prüfen, ob der zugehörige Mandant (Tenant) aktiv ist
	var tenantStatus string
	err = tx.QueryRowContext(ctx, `
		SELECT status
		FROM tenants
		WHERE id = ?
	`, tenantID).Scan(&tenantStatus)

	if errors.Is(err, sql.ErrNoRows) {
		return ErrInvalidInvitation
	}
	if err != nil {
		return err
	}
	if tenantStatus != "active" {
		return ErrInvalidInvitation
	}

	// Mit einer lokalen Kopie arbeiten, damit ein fehlgeschlagener
	// Commit den übergebenen User-Pointer nicht mit unvollständigen Daten manipuliert
	userToRegister := *user
	userToRegister.TenantID = tenantID
	userToRegister.Role = "member"
	userToRegister.Status = "active"

	// Benutzer innerhalb derselben Transaktion einfügen
	if err := insertUserTx(ctx, tx, &userToRegister); err != nil {
		return err
	}

	// Die Einladung als verwendet markieren
	result, err := tx.ExecContext(ctx, `
		UPDATE invitations
		SET used_at = CURRENT_TIMESTAMP
		WHERE id = ?
		  AND used_at IS NULL
		  AND revoked_at IS NULL
		  AND datetime(expires_at) > datetime('now')
	`, invitationID)
	if err != nil {
		return err
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected != 1 {
		return ErrInvalidInvitation
	}

	// Transaktion erfolgreich abschließen
	if err := tx.Commit(); err != nil {
		return err
	}
	// Den User erst nach erfolgreichem Commit aktualisieren
	*user = userToRegister

	return nil
}

// entfernt dauerhaft eine Einladung, die zu dem angegebenen Mandanten gehört
func (m *InvitationModel) Delete(
	ctx context.Context,
	tenantID int,
	invitationID int,
) error {
	result, err := m.DB.ExecContext(
		ctx,
		`DELETE FROM invitations WHERE tenant_id = ? AND id = ?`,
		tenantID,
		invitationID,
	)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rowsAffected == 0 {
		return ErrInvalidInvitation
	}
	return nil
}
