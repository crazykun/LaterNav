package services

import (
	"errors"
	"fmt"
	"net/url"
	"strings"

	"ai-later-nav/internal/database/repository"
	"ai-later-nav/internal/models"
)

var (
	ErrFriendLinkNotFound = errors.New("友情链接不存在")
	ErrFriendLinkName     = errors.New("链接名称须为1至100个字符")
	ErrFriendLinkURL      = errors.New("链接地址无效（须为 http/https 链接）")
	ErrFriendLinkDesc     = errors.New("链接描述过长（最多255个字符）")
	ErrFriendLinkLogo     = errors.New("Logo 地址无效（可为空、/static/img/ 路径或 http/https 图片链接）")
)

type FriendLinkService struct {
	repo *repository.FriendLinkRepository
}

func NewFriendLinkService() *FriendLinkService {
	return &FriendLinkService{repo: repository.NewFriendLinkRepository()}
}

func (s *FriendLinkService) ListEnabled() ([]models.FriendLink, error) {
	return s.repo.ListEnabled()
}

func (s *FriendLinkService) GetAll() ([]models.FriendLink, error) {
	return s.repo.GetAll()
}

func (s *FriendLinkService) GetByID(id int64) (*models.FriendLink, error) {
	link, err := s.repo.GetByID(id)
	if err != nil {
		return nil, err
	}
	if link == nil {
		return nil, ErrFriendLinkNotFound
	}
	return link, nil
}

// normalizeFriendLinkURL 补全协议并校验，仅接受 http/https。
func normalizeFriendLinkURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", ErrFriendLinkURL
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return "", ErrFriendLinkURL
	}
	if len(raw) > 512 {
		return "", ErrFriendLinkURL
	}
	return raw, nil
}

// normalizeFriendLinkLogo 允许留空、本站上传路径或远程图片链接。
func normalizeFriendLinkLogo(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	if len(raw) > 512 {
		return "", ErrFriendLinkLogo
	}
	if strings.HasPrefix(raw, "/static/img/") {
		return raw, nil
	}
	if strings.Contains(raw, "://") {
		u, err := url.Parse(raw)
		if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
			return "", ErrFriendLinkLogo
		}
		return raw, nil
	}
	return "", ErrFriendLinkLogo
}

// Validate 校验并规范化友链字段，返回新的副本供仓储写入。
func (s *FriendLinkService) Validate(link *models.FriendLink) (*models.FriendLink, error) {
	clean := *link
	clean.Name = strings.TrimSpace(clean.Name)
	if clean.Name == "" || len([]rune(clean.Name)) > 100 {
		return nil, ErrFriendLinkName
	}
	finalURL, err := normalizeFriendLinkURL(clean.URL)
	if err != nil {
		return nil, err
	}
	clean.URL = finalURL
	clean.Description = strings.TrimSpace(clean.Description)
	if len([]rune(clean.Description)) > 255 {
		return nil, ErrFriendLinkDesc
	}
	logo, err := normalizeFriendLinkLogo(clean.Logo)
	if err != nil {
		return nil, err
	}
	clean.Logo = logo
	return &clean, nil
}

func (s *FriendLinkService) Create(link *models.FriendLink) (int64, error) {
	clean, err := s.Validate(link)
	if err != nil {
		return 0, err
	}
	return s.repo.Create(clean)
}

func (s *FriendLinkService) Update(link *models.FriendLink) error {
	clean, err := s.Validate(link)
	if err != nil {
		return err
	}
	return s.repo.Update(clean)
}

func (s *FriendLinkService) ToggleEnabled(id int64) error {
	if _, err := s.GetByID(id); err != nil {
		return err
	}
	return s.repo.ToggleEnabled(id)
}

func (s *FriendLinkService) Delete(id int64) error {
	if _, err := s.GetByID(id); err != nil {
		return err
	}
	return s.repo.Delete(id)
}

func (s *FriendLinkService) Move(id int64, direction string) error {
	if direction != "up" && direction != "down" {
		return ErrFriendLinkNotFound
	}
	if err := s.repo.Move(id, direction); err != nil {
		if strings.Contains(err.Error(), "not found") {
			return ErrFriendLinkNotFound
		}
		return fmt.Errorf("调整排序失败: %w", err)
	}
	return nil
}
