package repository

import (
	"ai-later-nav/internal/database"
	"ai-later-nav/internal/models"
	"database/sql"
	"fmt"
)

type FriendLinkRepository struct{ db *sql.DB }

func NewFriendLinkRepository() *FriendLinkRepository {
	return &FriendLinkRepository{db: database.DB}
}

const friendLinkColumns = `id, name, url, description, logo, sort_order, enabled, created_at, updated_at`

func scanFriendLink(scan func(dest ...interface{}) error, link *models.FriendLink) error {
	return scan(&link.ID, &link.Name, &link.URL, &link.Description, &link.Logo,
		&link.SortOrder, &link.Enabled, &link.CreatedAt, &link.UpdatedAt)
}

func (r *FriendLinkRepository) ListEnabled() ([]models.FriendLink, error) {
	return r.queryLinks(`SELECT ` + friendLinkColumns + ` FROM friend_links WHERE enabled = 1 ORDER BY sort_order ASC, id ASC`)
}

func (r *FriendLinkRepository) GetAll() ([]models.FriendLink, error) {
	return r.queryLinks(`SELECT ` + friendLinkColumns + ` FROM friend_links ORDER BY sort_order ASC, id ASC`)
}

func (r *FriendLinkRepository) queryLinks(query string) ([]models.FriendLink, error) {
	rows, err := r.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("query friend links: %w", err)
	}
	defer rows.Close()

	var links []models.FriendLink
	for rows.Next() {
		var link models.FriendLink
		if err := scanFriendLink(rows.Scan, &link); err != nil {
			return nil, fmt.Errorf("scan friend link: %w", err)
		}
		links = append(links, link)
	}
	return links, rows.Err()
}

func (r *FriendLinkRepository) GetByID(id int64) (*models.FriendLink, error) {
	var link models.FriendLink
	err := r.db.QueryRow(`SELECT `+friendLinkColumns+` FROM friend_links WHERE id = ?`, id).
		Scan(&link.ID, &link.Name, &link.URL, &link.Description, &link.Logo,
			&link.SortOrder, &link.Enabled, &link.CreatedAt, &link.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("query friend link: %w", err)
	}
	return &link, nil
}

func (r *FriendLinkRepository) Create(link *models.FriendLink) (int64, error) {
	result, err := r.db.Exec(`
		INSERT INTO friend_links (name, url, description, logo, sort_order, enabled)
		VALUES (?, ?, ?, ?, (SELECT COALESCE(MAX(sort_order), 0) + 10 FROM (SELECT sort_order FROM friend_links) newest), ?)
	`, link.Name, link.URL, link.Description, link.Logo, link.Enabled)
	if err != nil {
		return 0, fmt.Errorf("insert friend link: %w", err)
	}
	return result.LastInsertId()
}

func (r *FriendLinkRepository) Update(link *models.FriendLink) error {
	result, err := r.db.Exec(`
		UPDATE friend_links
		SET name = ?, url = ?, description = ?, logo = ?, sort_order = ?, enabled = ?
		WHERE id = ?
	`, link.Name, link.URL, link.Description, link.Logo, link.SortOrder, link.Enabled, link.ID)
	if err != nil {
		return fmt.Errorf("update friend link: %w", err)
	}
	return requireFriendLinkRows(result)
}

func (r *FriendLinkRepository) ToggleEnabled(id int64) error {
	result, err := r.db.Exec(`
		UPDATE friend_links SET enabled = 1 - enabled, updated_at = NOW() WHERE id = ?
	`, id)
	if err != nil {
		return fmt.Errorf("toggle friend link: %w", err)
	}
	return requireFriendLinkRows(result)
}

func (r *FriendLinkRepository) Delete(id int64) error {
	result, err := r.db.Exec(`DELETE FROM friend_links WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete friend link: %w", err)
	}
	return requireFriendLinkRows(result)
}

func requireFriendLinkRows(result sql.Result) error {
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("friend link rows affected: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("friend link not found")
	}
	return nil
}

// Move swaps the link with its nearest neighbor in (sort_order, id) order.
// Tuple comparison keeps the swap correct even when sort orders are equal.
func (r *FriendLinkRepository) Move(id int64, direction string) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var current models.FriendLink
	if err := tx.QueryRow(`SELECT id, sort_order FROM friend_links WHERE id = ? FOR UPDATE`, id).
		Scan(&current.ID, &current.SortOrder); err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("friend link not found")
		}
		return fmt.Errorf("lock friend link: %w", err)
	}

	comparison, order := ">", "ASC"
	if direction == "up" {
		comparison, order = "<", "DESC"
	}
	var neighbor struct {
		id        int64
		sortOrder int
	}
	err = tx.QueryRow(`
		SELECT id, sort_order FROM friend_links
		WHERE (sort_order `+comparison+` ? OR (sort_order = ? AND id `+comparison+` ?))
		ORDER BY sort_order `+order+`, id `+order+`
		LIMIT 1 FOR UPDATE`,
		current.SortOrder, current.SortOrder, current.ID).
		Scan(&neighbor.id, &neighbor.sortOrder)
	if err == sql.ErrNoRows {
		// Already at the edge; nothing to do.
		return tx.Commit()
	}
	if err != nil {
		return fmt.Errorf("query friend link neighbor: %w", err)
	}

	if _, err := tx.Exec(`UPDATE friend_links SET sort_order = ?, updated_at = NOW() WHERE id = ?`, neighbor.sortOrder, current.ID); err != nil {
		return fmt.Errorf("swap friend link order: %w", err)
	}
	if _, err := tx.Exec(`UPDATE friend_links SET sort_order = ?, updated_at = NOW() WHERE id = ?`, current.SortOrder, neighbor.id); err != nil {
		return fmt.Errorf("swap friend link order: %w", err)
	}
	return tx.Commit()
}
