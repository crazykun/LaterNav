package services

import (
	"errors"
	"strings"
	"unicode/utf8"

	"ai-later-nav/internal/database/repository"
	"ai-later-nav/internal/models"
)

var (
	ErrInvalidTagName = errors.New("标签名称须为1至100个字符")
	ErrTagExists      = errors.New("标签名称已存在")
	ErrTagNotFound    = errors.New("标签不存在")
	ErrInvalidMerge   = errors.New("请选择另一个有效的合并目标")
)

type TagService struct{ repo *repository.TagRepository }

func NewTagService() *TagService { return &TagService{repo: repository.NewTagRepository()} }

// NormalizeTagNames is shared by admin site forms and member submissions.
func NormalizeTagNames(raw string) ([]string, error) {
	seen := make(map[string]struct{})
	tags := make([]string, 0)
	for _, tag := range strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == '，' }) {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}
		if utf8.RuneCountInString(tag) > 100 {
			return nil, ErrInvalidTagName
		}
		if _, exists := seen[tag]; exists {
			continue
		}
		seen[tag] = struct{}{}
		tags = append(tags, tag)
	}
	return tags, nil
}

func validTagName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > 100 || strings.ContainsAny(name, ",，") {
		return "", ErrInvalidTagName
	}
	return name, nil
}

func (s *TagService) ListAll() ([]models.Tag, error) { return s.repo.ListAll() }

func (s *TagService) Page(query string, page, pageSize int) (*models.TagPage, error) {
	query = strings.TrimSpace(query)
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	total, err := s.repo.Count(query)
	if err != nil {
		return nil, err
	}
	totalPages := int((total + int64(pageSize) - 1) / int64(pageSize))
	if totalPages < 1 {
		totalPages = 1
	}
	if page > totalPages {
		page = totalPages
	}
	tags, err := s.repo.ListPage(query, pageSize, (page-1)*pageSize)
	if err != nil {
		return nil, err
	}
	return &models.TagPage{Tags: tags, Total: total, Page: page, PageSize: pageSize, TotalPages: totalPages}, nil
}

func (s *TagService) Create(name string) error {
	name, err := validTagName(name)
	if err != nil {
		return err
	}
	existing, err := s.repo.GetByName(name)
	if err != nil {
		return err
	}
	if existing != nil {
		return ErrTagExists
	}
	_, err = s.repo.Create(name)
	return err
}

func (s *TagService) Rename(id int64, name string) error {
	name, err := validTagName(name)
	if err != nil {
		return err
	}
	tag, err := s.repo.GetByID(id)
	if err != nil {
		return err
	}
	if tag == nil {
		return ErrTagNotFound
	}
	if tag.Name == name {
		return nil
	}
	existing, err := s.repo.GetByName(name)
	if err != nil {
		return err
	}
	if existing != nil {
		return ErrTagExists
	}
	return s.repo.Rename(id, tag.Name, name)
}

func (s *TagService) Delete(id int64) error {
	tag, err := s.repo.GetByID(id)
	if err != nil {
		return err
	}
	if tag == nil {
		return ErrTagNotFound
	}
	return s.repo.Delete(id, tag.Name)
}

func (s *TagService) Merge(sourceID, targetID int64) error {
	if sourceID == targetID {
		return ErrInvalidMerge
	}
	source, err := s.repo.GetByID(sourceID)
	if err != nil {
		return err
	}
	target, err := s.repo.GetByID(targetID)
	if err != nil {
		return err
	}
	if source == nil || target == nil {
		return ErrTagNotFound
	}
	return s.repo.Merge(sourceID, targetID, source.Name, target.Name)
}
