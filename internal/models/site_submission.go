package models

import "time"

// 站点投稿审核状态。
const (
	SubmissionStatusPending  = "pending"
	SubmissionStatusApproved = "approved"
	SubmissionStatusRejected = "rejected"
)

type SiteSubmission struct {
	ID           int64     `json:"id" db:"id"`
	UserID       int64     `json:"user_id" db:"user_id"`
	Name         string    `json:"name" db:"name"`
	URL          string    `json:"url" db:"url"`
	Description  string    `json:"description" db:"description"`
	Logo         string    `json:"logo" db:"logo"`
	Category     string    `json:"category" db:"category"`
	Tags         string    `json:"tags" db:"tags"`
	Status       string    `json:"status" db:"status"`
	RejectReason string    `json:"reject_reason" db:"reject_reason"`
	ReviewedBy   int64     `json:"reviewed_by" db:"reviewed_by"`
	ReviewedAt   time.Time `json:"reviewed_at" db:"reviewed_at"`
	SiteID       int64     `json:"site_id" db:"site_id"`
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time `json:"updated_at" db:"updated_at"`

	// SubmitterName 由列表查询 JOIN users 带出（优先昵称），仅管理端展示使用。
	SubmitterName string `json:"submitter_name" db:"submitter_name"`
}

// StatusText 返回状态的中文展示文案。
func (s *SiteSubmission) StatusText() string {
	switch s.Status {
	case SubmissionStatusApproved:
		return "已通过"
	case SubmissionStatusRejected:
		return "已驳回"
	default:
		return "待审核"
	}
}
