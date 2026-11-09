package services

import (
	"errors"
	"fmt"
	"net/url"
	"strings"

	"ai-later-nav/internal/database/repository"
	"ai-later-nav/internal/models"
)

// 投稿业务校验错误。
var (
	ErrInvalidName        = errors.New("站点名称无效（须为1至150个字符）")
	ErrInvalidURL         = errors.New("站点地址无效（须为 http/https 链接）")
	ErrInvalidDescription = errors.New("站点描述过长（最多500个字符）")
	ErrInvalidTags        = errors.New("标签过多或过长（最多10个，总共500个字符）")
	ErrSubmissionExists   = errors.New("该站点已被收录，无需重复投稿")
	ErrAlreadySubmitted   = errors.New("您已提交过该站点，请等待审核结果")
	ErrTooManyPending     = errors.New("待审核投稿过多，请稍后再试")
	ErrSubmissionNotFound = errors.New("投稿不存在")
	ErrNotPending         = errors.New("该投稿已被处理，请刷新后重试")
)

// MaxPendingSubmissionsPerUser 单用户同时待审核的投稿上限，防止刷投稿。
const MaxPendingSubmissionsPerUser = 10

const maxSubmissionTags = 10

type SiteSubmissionService struct {
	submissionRepo *repository.SiteSubmissionRepository
	siteRepo       *repository.SiteRepository
	siteService    *SiteService
}

func NewSiteSubmissionService() *SiteSubmissionService {
	return &SiteSubmissionService{
		submissionRepo: repository.NewSiteSubmissionRepository(),
		siteRepo:       repository.NewSiteRepository(),
		siteService:    NewSiteService(),
	}
}

// normalizeSubmissionURL 补全缺失的协议并校验，仅接受 http/https，返回规范化后的 URL。
func normalizeSubmissionURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", ErrInvalidURL
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return "", ErrInvalidURL
	}
	if len(raw) > 512 {
		return "", ErrInvalidURL
	}
	return raw, nil
}

// normalizeSubmissionTags 按中英文逗号拆分标签，去空去重后以英文逗号重新拼接。
func normalizeSubmissionTags(raw string) (string, error) {
	seen := make(map[string]struct{})
	var tags []string
	for _, t := range strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == '，' }) {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		if _, ok := seen[t]; ok {
			continue
		}
		seen[t] = struct{}{}
		tags = append(tags, t)
	}
	if len(tags) > maxSubmissionTags || len(strings.Join(tags, ",")) > 500 {
		return "", ErrInvalidTags
	}
	return strings.Join(tags, ","), nil
}

// Submit 会员投稿一个站点。
func (s *SiteSubmissionService) Submit(userID int64, name, rawURL, description, logo, category, tags string) (int64, error) {
	if userID <= 0 {
		return 0, ErrSubmissionNotFound
	}
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 150 {
		return 0, ErrInvalidName
	}
	finalURL, err := normalizeSubmissionURL(rawURL)
	if err != nil {
		return 0, err
	}
	description = strings.TrimSpace(description)
	if len(description) > 500 {
		return 0, ErrInvalidDescription
	}
	logo = strings.TrimSpace(logo)
	if len(logo) > 512 {
		logo = logo[:512]
	}
	category = strings.TrimSpace(category)
	if len(category) > 100 {
		category = category[:100]
	}
	finalTags, err := normalizeSubmissionTags(tags)
	if err != nil {
		return 0, err
	}

	// 已收录的直接拒绝，避免产生无效投稿。
	existing, err := s.siteRepo.GetByURL(finalURL)
	if err != nil {
		return 0, err
	}
	if existing != nil {
		return 0, ErrSubmissionExists
	}

	// 同一用户同一条 URL 待审核中时不允许重复投稿。
	duplicate, err := s.submissionRepo.CountPendingByUserURL(userID, finalURL)
	if err != nil {
		return 0, err
	}
	if duplicate > 0 {
		return 0, ErrAlreadySubmitted
	}

	pending, err := s.submissionRepo.CountPendingByUser(userID)
	if err != nil {
		return 0, err
	}
	if pending >= MaxPendingSubmissionsPerUser {
		return 0, ErrTooManyPending
	}

	submission := &models.SiteSubmission{
		UserID:      userID,
		Name:        name,
		URL:         finalURL,
		Description: description,
		Logo:        logo,
		Category:    category,
		Tags:        finalTags,
		Status:      models.SubmissionStatusPending,
	}
	id, err := s.submissionRepo.Create(submission)
	if err != nil {
		return 0, err
	}
	return id, nil
}

// ListMine 会员查看自己的投稿（倒序，最多 limit 条）。
func (s *SiteSubmissionService) ListMine(userID int64, limit int) ([]models.SiteSubmission, error) {
	return s.submissionRepo.ListByUser(userID, limit)
}

// ListByStatus 管理端按状态列出投稿。
func (s *SiteSubmissionService) ListByStatus(status string) ([]models.SiteSubmission, error) {
	return s.submissionRepo.ListByStatus(status)
}

// CountByStatus 各状态数量，供审核页 Tab 展示。
func (s *SiteSubmissionService) CountByStatus() (map[string]int64, error) {
	return s.submissionRepo.CountByStatus()
}

// CountPending 待审核总数，供后台侧边栏徽标展示。
func (s *SiteSubmissionService) CountPending() (int64, error) {
	return s.submissionRepo.CountPending()
}

// getPendingSubmission 取出投稿并校验处于待审核状态。
func (s *SiteSubmissionService) getPendingSubmission(id int64) (*models.SiteSubmission, error) {
	submission, err := s.submissionRepo.GetByID(id)
	if err != nil {
		return nil, err
	}
	if submission == nil {
		return nil, ErrSubmissionNotFound
	}
	if submission.Status != models.SubmissionStatusPending {
		return nil, ErrNotPending
	}
	return submission, nil
}

// Approve 审核通过：按投稿内容写入 sites 表（含标签），并回链 site_id。
func (s *SiteSubmissionService) Approve(id, reviewerID int64) error {
	submission, err := s.getPendingSubmission(id)
	if err != nil {
		return err
	}

	// 通过前兜底查重：投稿期间站点可能已被其他渠道收录。
	existing, err := s.siteRepo.GetByURL(submission.URL)
	if err != nil {
		return err
	}
	if existing != nil {
		return fmt.Errorf("%w（已存在同名收录：%s）", ErrSubmissionExists, existing.Name)
	}

	var tags []string
	if submission.Tags != "" {
		tags = strings.Split(submission.Tags, ",")
	}
	site := &models.Site{
		Name:        submission.Name,
		URL:         submission.URL,
		Description: submission.Description,
		Logo:        submission.Logo,
		Category:    submission.Category,
	}
	siteID, err := s.siteService.Create(site, tags)
	if err != nil {
		return fmt.Errorf("写入站点失败: %w", err)
	}
	return s.submissionRepo.MarkApproved(id, reviewerID, siteID)
}

// Reject 驳回投稿，原因可为空。
func (s *SiteSubmissionService) Reject(id, reviewerID int64, reason string) error {
	if _, err := s.getPendingSubmission(id); err != nil {
		return err
	}
	reason = strings.TrimSpace(reason)
	if len(reason) > 500 {
		reason = reason[:500]
	}
	return s.submissionRepo.MarkRejected(id, reviewerID, reason)
}
