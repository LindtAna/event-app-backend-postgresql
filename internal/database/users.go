package database

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

var (
	ErrUserNotFound = errors.New("user not found")
	ErrInvalidUser  = errors.New("invalid user")
)

type UserModel struct {
	DB *sql.DB
}

type User struct {
	Id        int    `json:"id"`
	TenantID  int    `json:"tenantId"`
	Name      string `json:"name"`
	Email     string `json:"email"`
	Password  string `json:"-"`
	Bio       string `json:"bio"`
	AvatarUrl string `json:"avatarUrl"`
	Role      string `json:"role"`
	Status    string `json:"status"`
	CreatedAt string `json:"createdAt"`
}

const userSelect = `
	SELECT id, tenant_id, name, email, password_hash,
	       bio, avatar_url, role, status, created_at
	FROM users
`

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// enthält die allgemeine Logik für das Hinzufügen eines Benutzers
func insertUserTx(ctx context.Context, tx *sql.Tx, user *User) error {
	if user == nil || user.TenantID <= 0 ||
		strings.TrimSpace(user.Name) == "" ||
		normalizeEmail(user.Email) == "" ||
		user.Password == "" {
		return ErrInvalidUser
	}

	if user.Role == "" {
		user.Role = "member"
	}
	if user.Status == "" {
		user.Status = "active"
	}

	err := tx.QueryRowContext(ctx, `
	INSERT INTO users (
		tenant_id, email, name, password_hash,
		bio, avatar_url, role, status
	)
	VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	RETURNING id, created_at
`,
		user.TenantID,
		normalizeEmail(user.Email),
		strings.TrimSpace(user.Name),
		user.Password,
		user.Bio,
		user.AvatarUrl,
		user.Role,
		user.Status,
	).Scan(&user.Id, &user.CreatedAt)

	if err != nil {
		return err
	}

	user.Email = normalizeEmail(user.Email)

	return nil
}

func (m *UserModel) Insert(user *User) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	tx, err := m.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := insertUserTx(ctx, tx, user); err != nil {
		return err
	}

	return tx.Commit()
}

func (m *UserModel) getUser(
	ctx context.Context, query string,
	args ...interface{}) (*User, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	var user User

	err := m.DB.QueryRowContext(ctx, query, args...).Scan(
		&user.Id,
		&user.TenantID,
		&user.Name,
		&user.Email,
		&user.Password,
		&user.Bio,
		&user.AvatarUrl,
		&user.Role,
		&user.Status,
		&user.CreatedAt,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	return &user, nil
}

func (m *UserModel) Get(
	ctx context.Context,
	id int,
) (*User, error) {
	if id <= 0 {
		return nil, ErrInvalidUser
	}

	return m.getUser(ctx, userSelect+` WHERE id = $1`, id)
}

func (m *UserModel) GetByEmail(ctx context.Context,
	email string) (*User, error) {
	email = normalizeEmail(email)
	if email == "" {
		return nil, ErrInvalidUser
	}

	return m.getUser(
		ctx,
		userSelect+` WHERE email = $1`,
		email,
	)
}

func (m *UserModel) GetByIDInTenant(
	ctx context.Context,
	tenantID, userID int,
) (*User, error) {
	if tenantID <= 0 || userID <= 0 {
		return nil, ErrInvalidUser
	}
	return m.getUser(
		ctx,
		userSelect+` WHERE tenant_id = $1 AND id = $2`,
		tenantID,
		userID,
	)
}

// Aktualisierung des Profils
func (m *UserModel) UpdateProfile(ctx context.Context,
	tenantID, userID int,
	name, bio, avatarUrl string,
) error {
	if tenantID <= 0 || userID <= 0 {
		return ErrInvalidUser
	}

	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	result, err := m.DB.ExecContext(ctx, `
		UPDATE users
	    SET name = $1, bio = $2, avatar_url = $3
	    WHERE tenant_id = $4
	    AND id = $5
	    AND status = 'active'
	`,
		strings.TrimSpace(name),
		bio,
		avatarUrl,
		tenantID,
		userID,
	)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return ErrUserNotFound
	}

	return nil
}
