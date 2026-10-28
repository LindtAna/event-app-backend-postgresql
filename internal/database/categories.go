package database

import (
	"context"
	"database/sql"
	"errors"
)

var ErrInvalidCategoryScope = errors.New("invalid category scope")

type Category struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	IsDefault bool   `json:"isDefault"`
}

type CategoryModel struct {
	DB *sql.DB
}

func (m CategoryModel) GetAll(
	ctx context.Context,
	tenantID int) ([]Category, error) {

	if tenantID <= 0 {
		return nil, ErrInvalidCategoryScope
	}

	query := `
		SELECT id, name, is_default
		FROM categories
		WHERE tenant_id = ?
		ORDER BY is_default DESC, name COLLATE NOCASE ASC
	`
	//Die Standard-Systemkategorien -> immer oben in der Liste,
	// benutzerdefinierte Kategorien werden sortiert alphabetisch

	rows, err := m.DB.QueryContext(ctx, query, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	categories := make([]Category, 0)

	for rows.Next() {
		var category Category
		var isDefault int

		if err := rows.Scan(
			&category.ID,
			&category.Name,
			&isDefault,
		); err != nil {
			return nil, err
		}

		category.IsDefault = isDefault == 1
		categories = append(categories, category)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return categories, nil

}
