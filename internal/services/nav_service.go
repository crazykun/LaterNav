package services

import (
	"ai-later-nav/internal/database/repository"
	"ai-later-nav/internal/models"
)

// IconKeys are the icon identifiers available to nav items; the homepage
// template maps each key to a fixed SVG.
var IconKeys = []string{"grid", "chat", "create", "code", "tool", "star", "link"}

func IsValidNavIcon(icon string) bool {
	for _, k := range IconKeys {
		if k == icon {
			return true
		}
	}
	return false
}

type NavService struct {
	navRepo *repository.NavRepository
}

func NewNavService() *NavService {
	return &NavService{navRepo: repository.NewNavRepository()}
}

func (s *NavService) ListEnabled() ([]models.NavItem, error) {
	return s.navRepo.ListEnabled()
}

func (s *NavService) GetAll() ([]models.NavItem, error) {
	return s.navRepo.GetAll()
}

func (s *NavService) GetByID(id int64) (*models.NavItem, error) {
	return s.navRepo.GetByID(id)
}

func (s *NavService) Create(n *models.NavItem) (int64, error) {
	if !IsValidNavIcon(n.Icon) {
		n.Icon = "grid"
	}
	return s.navRepo.Create(n)
}

func (s *NavService) Update(n *models.NavItem) error {
	if !IsValidNavIcon(n.Icon) {
		n.Icon = "grid"
	}
	return s.navRepo.Update(n)
}

func (s *NavService) Delete(id int64) error {
	return s.navRepo.Delete(id)
}
