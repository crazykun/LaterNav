package repository

import (
	"database/sql"
	"fmt"

	"ai-later-nav/internal/database"
	"ai-later-nav/internal/models"
)

type CategoryRepository struct{ db *sql.DB }

func NewCategoryRepository() *CategoryRepository { return &CategoryRepository{db: database.DB} }

func (r *CategoryRepository) ListAll() ([]models.Category, error) {
	rows, err := r.db.Query(`SELECT id, name, sort_order, created_at, updated_at FROM categories ORDER BY sort_order, id`)
	if err != nil {
		return nil, fmt.Errorf("list categories: %w", err)
	}
	defer rows.Close()
	var categories []models.Category
	for rows.Next() {
		var c models.Category
		if err := rows.Scan(&c.ID, &c.Name, &c.SortOrder, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan category: %w", err)
		}
		categories = append(categories, c)
	}
	return categories, rows.Err()
}

func (r *CategoryRepository) GetByID(id int64) (*models.Category, error) {
	var c models.Category
	err := r.db.QueryRow(`SELECT id, name, sort_order, created_at, updated_at FROM categories WHERE id = ?`, id).
		Scan(&c.ID, &c.Name, &c.SortOrder, &c.CreatedAt, &c.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get category: %w", err)
	}
	return &c, nil
}

func (r *CategoryRepository) GetByName(name string) (*models.Category, error) {
	var c models.Category
	err := r.db.QueryRow(`SELECT id, name, sort_order, created_at, updated_at FROM categories WHERE name = ?`, name).
		Scan(&c.ID, &c.Name, &c.SortOrder, &c.CreatedAt, &c.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get category by name: %w", err)
	}
	return &c, nil
}

func (r *CategoryRepository) Count(query string) (int64, error) {
	var total int64
	err := r.db.QueryRow(`SELECT COUNT(*) FROM categories WHERE ? = '' OR name LIKE ?`, query, "%"+query+"%").Scan(&total)
	return total, err
}

func (r *CategoryRepository) ListPage(query string, limit, offset int) ([]models.CategoryUsage, error) {
	rows, err := r.db.Query(`
		SELECT c.id, c.name, c.sort_order, c.created_at, c.updated_at,
			(SELECT COUNT(*) FROM sites s WHERE s.category = c.name) AS site_count
		FROM categories c
		WHERE ? = '' OR c.name LIKE ?
		ORDER BY c.sort_order, c.id LIMIT ? OFFSET ?`, query, "%"+query+"%", limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list category page: %w", err)
	}
	defer rows.Close()
	categories := make([]models.CategoryUsage, 0, limit)
	for rows.Next() {
		var c models.CategoryUsage
		if err := rows.Scan(&c.ID, &c.Name, &c.SortOrder, &c.CreatedAt, &c.UpdatedAt, &c.SiteCount); err != nil {
			return nil, fmt.Errorf("scan category usage: %w", err)
		}
		categories = append(categories, c)
	}
	return categories, rows.Err()
}

func (r *CategoryRepository) Create(name string) (int64, error) {
	result, err := r.db.Exec(`INSERT INTO categories (name, sort_order) SELECT ?, COALESCE(MAX(sort_order), 0) + 10 FROM categories`, name)
	if err != nil {
		return 0, fmt.Errorf("create category: %w", err)
	}
	return result.LastInsertId()
}

// Rename updates the master record and every live reference in one transaction.
func (r *CategoryRepository) Rename(id int64, oldName, newName string) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE categories SET name = ? WHERE id = ?`, newName, id); err != nil {
		return fmt.Errorf("rename category: %w", err)
	}
	if _, err := tx.Exec(`UPDATE sites SET category = ?, updated_at = NOW() WHERE category = ?`, newName, oldName); err != nil {
		return fmt.Errorf("update site categories: %w", err)
	}
	if _, err := tx.Exec(`UPDATE site_submissions SET category = ?, updated_at = NOW() WHERE category = ? AND status = 'pending'`, newName, oldName); err != nil {
		return fmt.Errorf("update pending submission categories: %w", err)
	}
	if _, err := tx.Exec(`UPDATE nav_items SET url = ?, updated_at = NOW() WHERE url = ?`, "/search?category="+newName, "/search?category="+oldName); err != nil {
		return fmt.Errorf("update category navigation: %w", err)
	}
	return tx.Commit()
}

// Delete is guarded in SQL as well as in the service to prevent accidental unlinking.
func (r *CategoryRepository) Delete(id int64) error {
	result, err := r.db.Exec(`DELETE FROM categories WHERE id = ? AND NOT EXISTS (
		SELECT 1 FROM sites s WHERE s.category = categories.name
	)`, id)
	if err != nil {
		return fmt.Errorf("delete category: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return fmt.Errorf("category not found or still in use")
	}
	return nil
}

func (r *CategoryRepository) Move(id int64, direction string) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var current, neighbor models.Category
	if err := tx.QueryRow(`SELECT id, sort_order FROM categories WHERE id = ? FOR UPDATE`, id).Scan(&current.ID, &current.SortOrder); err != nil {
		return err
	}
	operator, order := ">", "ASC"
	if direction == "up" {
		operator, order = "<", "DESC"
	}
	err = tx.QueryRow(`SELECT id, sort_order FROM categories WHERE sort_order `+operator+` ? ORDER BY sort_order `+order+`, id `+order+` LIMIT 1 FOR UPDATE`, current.SortOrder).
		Scan(&neighbor.ID, &neighbor.SortOrder)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE categories SET sort_order = ? WHERE id = ?`, neighbor.SortOrder, current.ID); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE categories SET sort_order = ? WHERE id = ?`, current.SortOrder, neighbor.ID); err != nil {
		return err
	}
	return tx.Commit()
}
