package repository

import (
	"ai-later-nav/internal/database"
	"ai-later-nav/internal/models"
	"database/sql"
	"fmt"
)

type NavRepository struct {
	db *sql.DB
}

func NewNavRepository() *NavRepository {
	return &NavRepository{db: database.DB}
}

const navColumns = "id, name, url, icon, sort_order, enabled, created_at, updated_at"

func navScanDest(n *models.NavItem) []any {
	return []any{&n.ID, &n.Name, &n.URL, &n.Icon, &n.SortOrder, &n.Enabled, &n.CreatedAt, &n.UpdatedAt}
}

func (r *NavRepository) queryNavItems(query string, args ...any) ([]models.NavItem, error) {
	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("query nav items: %w", err)
	}
	defer rows.Close()

	var items []models.NavItem
	for rows.Next() {
		var n models.NavItem
		if err := rows.Scan(navScanDest(&n)...); err != nil {
			return nil, fmt.Errorf("scan nav item: %w", err)
		}
		items = append(items, n)
	}
	return items, rows.Err()
}

// ListEnabled returns enabled items ordered for homepage rendering.
func (r *NavRepository) ListEnabled() ([]models.NavItem, error) {
	return r.queryNavItems("SELECT " + navColumns + " FROM nav_items WHERE enabled = 1 ORDER BY sort_order ASC, id ASC")
}

func (r *NavRepository) GetAll() ([]models.NavItem, error) {
	return r.queryNavItems("SELECT " + navColumns + " FROM nav_items ORDER BY sort_order ASC, id ASC")
}

func (r *NavRepository) GetByID(id int64) (*models.NavItem, error) {
	var n models.NavItem
	err := r.db.QueryRow("SELECT "+navColumns+" FROM nav_items WHERE id = ?", id).
		Scan(navScanDest(&n)...)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("query nav item: %w", err)
	}
	return &n, nil
}

func (r *NavRepository) Create(n *models.NavItem) (int64, error) {
	res, err := r.db.Exec(
		"INSERT INTO nav_items (name, url, icon, sort_order, enabled) VALUES (?, ?, ?, ?, ?)",
		n.Name, n.URL, n.Icon, n.SortOrder, n.Enabled,
	)
	if err != nil {
		return 0, fmt.Errorf("insert nav item: %w", err)
	}
	return res.LastInsertId()
}

func (r *NavRepository) Update(n *models.NavItem) error {
	_, err := r.db.Exec(
		"UPDATE nav_items SET name = ?, url = ?, icon = ?, sort_order = ?, enabled = ? WHERE id = ?",
		n.Name, n.URL, n.Icon, n.SortOrder, n.Enabled, n.ID,
	)
	if err != nil {
		return fmt.Errorf("update nav item: %w", err)
	}
	return nil
}

func (r *NavRepository) Delete(id int64) error {
	_, err := r.db.Exec("DELETE FROM nav_items WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete nav item: %w", err)
	}
	return nil
}
