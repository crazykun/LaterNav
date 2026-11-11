package services

import (
	"errors"
	"strings"
	"unicode/utf8"

	"ai-later-nav/internal/database/repository"
	"ai-later-nav/internal/models"
)

var (
	ErrInvalidCategoryName = errors.New("分类名称须为1至100个字符")
	ErrCategoryExists      = errors.New("分类名称已存在")
	ErrCategoryNotFound    = errors.New("分类不存在")
	ErrCategoryInUse       = errors.New("该分类仍在使用，请先迁移此分类下的站点")
)

type CategoryService struct {
	repo *repository.CategoryRepository
}

func NewCategoryService() *CategoryService {
	return &CategoryService{repo: repository.NewCategoryRepository()}
}

func (s *CategoryService) ListAll() ([]models.Category, error) { return s.repo.ListAll() }

func (s *CategoryService) Names() ([]string, error) {
	categories, err := s.repo.ListAll()
	if err != nil {
		return nil, err
	}
	names := make([]string, len(categories))
	for i, category := range categories {
		names[i] = category.Name
	}
	return names, nil
}

func (s *CategoryService) Exists(name string) (bool, error) {
	if name == "" {
		return true, nil
	}
	category, err := s.repo.GetByName(name)
	return category != nil, err
}

func (s *CategoryService) Page(query string, page, pageSize int) (*models.CategoryPage, error) {
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
	categories, err := s.repo.ListPage(query, pageSize, (page-1)*pageSize)
	if err != nil {
		return nil, err
	}
	return &models.CategoryPage{Categories: categories, Total: total, Page: page, PageSize: pageSize, TotalPages: totalPages}, nil
}

func validCategoryName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > 100 || strings.ContainsAny(name, ",，") {
		return "", ErrInvalidCategoryName
	}
	return name, nil
}

// validCategoryIcon 空值表示按名称自动匹配，否则必须是导航图标白名单里的键。
func validCategoryIcon(icon string) (string, error) {
	icon = strings.TrimSpace(icon)
	if icon == "" || IsValidNavIcon(icon) {
		return icon, nil
	}
	return "", ErrInvalidCategoryName
}

func (s *CategoryService) Create(name, icon string) error {
	name, err := validCategoryName(name)
	if err != nil {
		return err
	}
	icon, err = validCategoryIcon(icon)
	if err != nil {
		return err
	}
	existing, err := s.repo.GetByName(name)
	if err != nil {
		return err
	}
	if existing != nil {
		return ErrCategoryExists
	}
	_, err = s.repo.Create(name, icon)
	return err
}

func (s *CategoryService) Rename(id int64, name, icon string) error {
	name, err := validCategoryName(name)
	if err != nil {
		return err
	}
	icon, err = validCategoryIcon(icon)
	if err != nil {
		return err
	}
	category, err := s.repo.GetByID(id)
	if err != nil {
		return err
	}
	if category == nil {
		return ErrCategoryNotFound
	}
	if category.Name == name && category.Icon == icon {
		return nil
	}
	if category.Name != name {
		existing, err := s.repo.GetByName(name)
		if err != nil {
			return err
		}
		if existing != nil {
			return ErrCategoryExists
		}
	}
	return s.repo.Rename(id, category.Name, name, icon)
}

func (s *CategoryService) Delete(id int64) error {
	category, err := s.repo.GetByID(id)
	if err != nil {
		return err
	}
	if category == nil {
		return ErrCategoryNotFound
	}
	return s.repo.Delete(id)
}

func (s *CategoryService) Move(id int64, direction string) error {
	if direction != "up" && direction != "down" {
		return ErrCategoryNotFound
	}
	return s.repo.Move(id, direction)
}
