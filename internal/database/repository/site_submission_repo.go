package repository

import (
	"ai-later-nav/internal/database"
	"ai-later-nav/internal/models"
	"database/sql"
	"fmt"
)

type SiteSubmissionRepository struct {
	db *sql.DB
}

func NewSiteSubmissionRepository() *SiteSubmissionRepository {
	return &SiteSubmissionRepository{db: database.DB}
}

const submissionColumns = `
	ss.id, ss.user_id, ss.name, ss.url, ss.description, ss.logo, ss.category, ss.tags,
	ss.status, ss.reject_reason, COALESCE(ss.reviewed_by, 0), COALESCE(ss.reviewed_at, ss.created_at),
	COALESCE(ss.site_id, 0), ss.created_at, ss.updated_at
`

func scanSubmission(scan func(dest ...interface{}) error, s *models.SiteSubmission) error {
	return scan(&s.ID, &s.UserID, &s.Name, &s.URL, &s.Description, &s.Logo, &s.Category, &s.Tags,
		&s.Status, &s.RejectReason, &s.ReviewedBy, &s.ReviewedAt,
		&s.SiteID, &s.CreatedAt, &s.UpdatedAt)
}

func (r *SiteSubmissionRepository) Create(s *models.SiteSubmission) (int64, error) {
	result, err := r.db.Exec(`
		INSERT INTO site_submissions (user_id, name, url, description, logo, category, tags, status)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, s.UserID, s.Name, s.URL, s.Description, s.Logo, s.Category, s.Tags, s.Status)
	if err != nil {
		return 0, fmt.Errorf("insert site submission: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("get site submission id: %w", err)
	}
	return id, nil
}

func (r *SiteSubmissionRepository) GetByID(id int64) (*models.SiteSubmission, error) {
	var s models.SiteSubmission
	err := r.db.QueryRow(`
		SELECT `+submissionColumns+`
		FROM site_submissions ss WHERE ss.id = ?
	`, id).Scan(&s.ID, &s.UserID, &s.Name, &s.URL, &s.Description, &s.Logo, &s.Category, &s.Tags,
		&s.Status, &s.RejectReason, &s.ReviewedBy, &s.ReviewedAt,
		&s.SiteID, &s.CreatedAt, &s.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("query site submission by id: %w", err)
	}
	return &s, nil
}

// ListByStatus 管理端按状态列出投稿，JOIN users 带出提交人展示名。
func (r *SiteSubmissionRepository) ListByStatus(status string) ([]models.SiteSubmission, error) {
	rows, err := r.db.Query(`
		SELECT `+submissionColumns+`, COALESCE(NULLIF(u.nickname, ''), u.username) AS submitter_name
		FROM site_submissions ss
		JOIN users u ON u.id = ss.user_id
		WHERE ss.status = ?
		ORDER BY ss.created_at DESC
	`, status)
	if err != nil {
		return nil, fmt.Errorf("query site submissions by status: %w", err)
	}
	defer rows.Close()

	var list []models.SiteSubmission
	for rows.Next() {
		var s models.SiteSubmission
		if err := rows.Scan(&s.ID, &s.UserID, &s.Name, &s.URL, &s.Description, &s.Logo, &s.Category, &s.Tags,
			&s.Status, &s.RejectReason, &s.ReviewedBy, &s.ReviewedAt,
			&s.SiteID, &s.CreatedAt, &s.UpdatedAt, &s.SubmitterName); err != nil {
			return nil, fmt.Errorf("scan site submission: %w", err)
		}
		list = append(list, s)
	}
	return list, rows.Err()
}

// CountByStatus 一次查询返回各状态数量，供审核页 Tab 徽标使用。
func (r *SiteSubmissionRepository) CountByStatus() (map[string]int64, error) {
	rows, err := r.db.Query(`SELECT status, COUNT(*) FROM site_submissions GROUP BY status`)
	if err != nil {
		return nil, fmt.Errorf("count site submissions by status: %w", err)
	}
	defer rows.Close()

	counts := map[string]int64{
		models.SubmissionStatusPending:  0,
		models.SubmissionStatusApproved: 0,
		models.SubmissionStatusRejected: 0,
	}
	for rows.Next() {
		var status string
		var count int64
		if err := rows.Scan(&status, &count); err != nil {
			return nil, fmt.Errorf("scan submission status count: %w", err)
		}
		counts[status] = count
	}
	return counts, rows.Err()
}

func (r *SiteSubmissionRepository) CountPending() (int64, error) {
	var count int64
	err := r.db.QueryRow(`
		SELECT COUNT(*) FROM site_submissions WHERE status = ?
	`, models.SubmissionStatusPending).Scan(&count)
	return count, err
}

// ListByUser 会员端查看自己的投稿，按时间倒序。
func (r *SiteSubmissionRepository) ListByUser(userID int64, limit int) ([]models.SiteSubmission, error) {
	rows, err := r.db.Query(`
		SELECT `+submissionColumns+`
		FROM site_submissions ss WHERE ss.user_id = ?
		ORDER BY ss.created_at DESC
		LIMIT ?
	`, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("query site submissions by user: %w", err)
	}
	defer rows.Close()

	var list []models.SiteSubmission
	for rows.Next() {
		var s models.SiteSubmission
		if err := scanSubmission(rows.Scan, &s); err != nil {
			return nil, fmt.Errorf("scan site submission: %w", err)
		}
		list = append(list, s)
	}
	return list, rows.Err()
}

// CountPendingByUserURL 同一用户同一条 URL 处于待审核中的数量，用于投稿防重。
func (r *SiteSubmissionRepository) CountPendingByUserURL(userID int64, url string) (int64, error) {
	var count int64
	err := r.db.QueryRow(`
		SELECT COUNT(*) FROM site_submissions
		WHERE user_id = ? AND url = ? AND status = ?
	`, userID, url, models.SubmissionStatusPending).Scan(&count)
	return count, err
}

// CountPendingByUser 单用户待审核投稿数量，用于投稿频率控制。
func (r *SiteSubmissionRepository) CountPendingByUser(userID int64) (int64, error) {
	var count int64
	err := r.db.QueryRow(`
		SELECT COUNT(*) FROM site_submissions
		WHERE user_id = ? AND status = ?
	`, userID, models.SubmissionStatusPending).Scan(&count)
	return count, err
}

// MarkApproved 审核通过：记录审核人与回链站点。
func (r *SiteSubmissionRepository) MarkApproved(id, reviewerID, siteID int64) error {
	_, err := r.db.Exec(`
		UPDATE site_submissions
		SET status = ?, reviewed_by = ?, reviewed_at = NOW(), site_id = ?
		WHERE id = ?
	`, models.SubmissionStatusApproved, reviewerID, siteID, id)
	if err != nil {
		return fmt.Errorf("mark site submission approved: %w", err)
	}
	return nil
}

// MarkRejected 驳回：记录审核人与驳回原因。
func (r *SiteSubmissionRepository) MarkRejected(id, reviewerID int64, reason string) error {
	_, err := r.db.Exec(`
		UPDATE site_submissions
		SET status = ?, reject_reason = ?, reviewed_by = ?, reviewed_at = NOW()
		WHERE id = ?
	`, models.SubmissionStatusRejected, reason, reviewerID, id)
	if err != nil {
		return fmt.Errorf("mark site submission rejected: %w", err)
	}
	return nil
}
