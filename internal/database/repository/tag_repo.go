package repository

import (
	"database/sql"
	"fmt"
	"strings"

	"ai-later-nav/internal/database"
	"ai-later-nav/internal/models"
)

type TagRepository struct{ db *sql.DB }

func NewTagRepository() *TagRepository { return &TagRepository{db: database.DB} }

func (r *TagRepository) GetByID(id int64) (*models.Tag, error) {
	var tag models.Tag
	err := r.db.QueryRow(`SELECT id, name FROM tags WHERE id = ?`, id).Scan(&tag.ID, &tag.Name)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get tag: %w", err)
	}
	return &tag, nil
}

func (r *TagRepository) GetByName(name string) (*models.Tag, error) {
	var tag models.Tag
	err := r.db.QueryRow(`SELECT id, name FROM tags WHERE name = ?`, name).Scan(&tag.ID, &tag.Name)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get tag by name: %w", err)
	}
	return &tag, nil
}

func (r *TagRepository) ListAll() ([]models.Tag, error) {
	rows, err := r.db.Query(`SELECT id, name FROM tags ORDER BY name, id`)
	if err != nil {
		return nil, fmt.Errorf("list tags: %w", err)
	}
	defer rows.Close()
	var tags []models.Tag
	for rows.Next() {
		var tag models.Tag
		if err := rows.Scan(&tag.ID, &tag.Name); err != nil {
			return nil, err
		}
		tags = append(tags, tag)
	}
	return tags, rows.Err()
}

func (r *TagRepository) Count(query string) (int64, error) {
	var total int64
	err := r.db.QueryRow(`SELECT COUNT(*) FROM tags WHERE ? = '' OR name LIKE ?`, query, "%"+query+"%").Scan(&total)
	return total, err
}

func (r *TagRepository) ListPage(query string, limit, offset int) ([]models.TagUsage, error) {
	rows, err := r.db.Query(`
		SELECT t.id, t.name,
			(SELECT COUNT(*) FROM site_tags st JOIN sites s ON s.id = st.site_id WHERE st.tag_id = t.id AND s.deleted = 0) AS site_count
		FROM tags t WHERE ? = '' OR t.name LIKE ?
		ORDER BY site_count DESC, t.name, t.id LIMIT ? OFFSET ?`, query, "%"+query+"%", limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list tag page: %w", err)
	}
	defer rows.Close()
	tags := make([]models.TagUsage, 0, limit)
	for rows.Next() {
		var tag models.TagUsage
		if err := rows.Scan(&tag.ID, &tag.Name, &tag.SiteCount); err != nil {
			return nil, fmt.Errorf("scan tag usage: %w", err)
		}
		tags = append(tags, tag)
	}
	return tags, rows.Err()
}

func (r *TagRepository) Create(name string) (int64, error) {
	result, err := r.db.Exec(`INSERT INTO tags (name) VALUES (?)`, name)
	if err != nil {
		return 0, fmt.Errorf("create tag: %w", err)
	}
	return result.LastInsertId()
}

func (r *TagRepository) Rename(id int64, oldName, name string) error {
	return r.mutatePendingTags(map[string]string{oldName: name}, func(tx *sql.Tx) error {
		_, err := tx.Exec(`UPDATE tags SET name = ? WHERE id = ?`, name, id)
		return err
	})
}

func (r *TagRepository) Delete(id int64, name string) error {
	return r.mutatePendingTags(map[string]string{name: ""}, func(tx *sql.Tx) error {
		_, err := tx.Exec(`DELETE FROM tags WHERE id = ?`, id)
		return err
	})
}

// Merge copies relations without duplicate site/tag keys, then removes the source tag.
func (r *TagRepository) Merge(sourceID, targetID int64, sourceName, targetName string) error {
	return r.mutatePendingTags(map[string]string{sourceName: targetName}, func(tx *sql.Tx) error {
		if _, err := tx.Exec(`INSERT IGNORE INTO site_tags (site_id, tag_id) SELECT site_id, ? FROM site_tags WHERE tag_id = ?`, targetID, sourceID); err != nil {
			return fmt.Errorf("merge tag relations: %w", err)
		}
		if _, err := tx.Exec(`DELETE FROM site_tags WHERE tag_id = ?`, sourceID); err != nil {
			return fmt.Errorf("delete source tag relations: %w", err)
		}
		if _, err := tx.Exec(`DELETE FROM tags WHERE id = ?`, sourceID); err != nil {
			return fmt.Errorf("delete source tag: %w", err)
		}
		return nil
	})
}

// mutatePendingTags runs the tag mutation and pending snapshot updates atomically.
func (r *TagRepository) mutatePendingTags(tagUpdates map[string]string, mutate func(*sql.Tx) error) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := mutate(tx); err != nil {
		return err
	}
	rows, err := tx.Query(`SELECT id, tags FROM site_submissions WHERE status = 'pending' AND tags <> ''`)
	if err != nil {
		return err
	}
	var submissions []models.SiteSubmission
	for rows.Next() {
		var submission models.SiteSubmission
		if err := rows.Scan(&submission.ID, &submission.Tags); err != nil {
			rows.Close()
			return err
		}
		submissions = append(submissions, submission)
	}
	rows.Close()
	for _, submission := range submissions {
		tags, err := normalizeTagItems(submission.Tags)
		if err != nil {
			return err
		}
		updated := make([]string, 0, len(tags))
		for _, tag := range tags {
			if mapped, ok := tagUpdates[tag]; ok {
				if mapped == "" {
					continue
				}
				tag = mapped
			}
			duplicate := false
			for _, existing := range updated {
				if existing == tag {
					duplicate = true
					break
				}
			}
			if !duplicate {
				updated = append(updated, tag)
			}
		}
		if _, err := tx.Exec(`UPDATE site_submissions SET tags = ?, updated_at = NOW() WHERE id = ? AND status = 'pending'`, strings.Join(updated, ","), submission.ID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// normalizeTagItems mirrors the service-level comma normalization without a package cycle.
func normalizeTagItems(raw string) ([]string, error) {
	seen := make(map[string]struct{})
	var tags []string
	for _, tag := range strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == '，' }) {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}
		if _, ok := seen[tag]; ok {
			continue
		}
		seen[tag] = struct{}{}
		tags = append(tags, tag)
	}
	return tags, nil
}
