package services

import (
	"errors"

	"ai-later-nav/internal/database/repository"
	"ai-later-nav/internal/models"
)

var (
	ErrLastLoginMethod = errors.New("这是你唯一的登录方式，请先设置本站密码后再解绑")
	ErrBindingNotFound = errors.New("未找到该绑定记录")
)

type OAuthIdentityService struct {
	identityRepo *repository.OAuthIdentityRepository
	userRepo     *repository.UserRepository
}

func NewOAuthIdentityService() *OAuthIdentityService {
	return &OAuthIdentityService{
		identityRepo: repository.NewOAuthIdentityRepository(),
		userRepo:     repository.NewUserRepository(),
	}
}

func (s *OAuthIdentityService) List(userID int64) ([]models.OAuthIdentity, error) {
	return s.identityRepo.ListByUserID(userID)
}

func (s *OAuthIdentityService) Bind(userID int64, providerKey string, profile repository.OAuthIdentityProfile) error {
	return s.identityRepo.Bind(userID, providerKey, profile)
}

func (s *OAuthIdentityService) IsProviderBound(userID int64, providerKey string) (bool, error) {
	identities, err := s.identityRepo.ListByUserID(userID)
	if err != nil {
		return false, err
	}
	for _, identity := range identities {
		if identity.Provider == providerKey {
			return true, nil
		}
	}
	return false, nil
}

// canUnbindIdentity 拒绝纯三方用户解绑唯一登录方式：password_hash 为哨兵 "!" 时没有本站密码可兜底。
// count>1 时即使 passwordHash 为空也允许（还有其他三方登录方式）。
func canUnbindIdentity(count int, passwordHash string) error {
	if count > 1 || passwordHash != "!" {
		return nil
	}
	return ErrLastLoginMethod
}

// Unbind 拒绝纯三方用户解绑最后一个登录方式。
func (s *OAuthIdentityService) Unbind(userID int64, providerKey string) error {
	count, err := s.identityRepo.CountByUserID(userID)
	if err != nil {
		return err
	}
	passwordHash := ""
	if count <= 1 {
		user, err := s.userRepo.GetByID(userID)
		if err != nil {
			return err
		}
		if user == nil {
			return ErrBindingNotFound
		}
		passwordHash = user.PasswordHash
	}
	if err := canUnbindIdentity(count, passwordHash); err != nil {
		return err
	}
	return s.identityRepo.Unbind(userID, providerKey)
}
