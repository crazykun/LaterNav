package services

import (
	"ai-later-nav/internal/database/repository"
	"ai-later-nav/internal/models"
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/go-sql-driver/mysql"

	"golang.org/x/crypto/bcrypt"
)

var (
	ErrUserNotFound           = errors.New("user not found")
	ErrUsernameExists         = errors.New("username already exists")
	ErrInvalidPassword        = errors.New("invalid password")
	ErrUserBlocked            = errors.New("user is blocked")
	ErrCannotModifySelfRole   = errors.New("cannot modify your own role")
	ErrCannotBlockSelf        = errors.New("cannot block yourself")
	ErrInvalidRole            = errors.New("invalid role")
	ErrInvalidStatus          = errors.New("invalid status")
	ErrInvalidUsername        = errors.New("invalid username")
	ErrInvalidNickname        = errors.New("invalid nickname")
	ErrUsernameReauthRequired = errors.New("username change requires reauthentication")
)

type UserService struct {
	userRepo *repository.UserRepository
}

func NewUserService() *UserService {
	return &UserService{
		userRepo: repository.NewUserRepository(),
	}
}

func (s *UserService) Register(username, password string) (*models.User, error) {
	if existing, _ := s.userRepo.GetByUsername(username); existing != nil {
		return nil, ErrUsernameExists
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	user := &models.User{
		Username:     username,
		PasswordHash: string(hash),
		Role:         "user",
	}

	id, err := s.userRepo.Create(user)
	if err != nil {
		return nil, err
	}

	user.ID = id
	return user, nil
}

func (s *UserService) Login(username, password string) (*models.User, error) {
	user, err := s.userRepo.GetByUsername(username)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, ErrUserNotFound
	}

	if s.IsBlocked(user) {
		return nil, ErrUserBlocked
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return nil, ErrInvalidPassword
	}

	return user, nil
}

func (s *UserService) GetByID(id int64) (*models.User, error) {
	return s.userRepo.GetByID(id)
}

// ValidateProfile checks the database length limits and rejects control characters.
func ValidateProfile(username, nickname string) error {
	if !utf8.ValidString(username) || username == "" || strings.TrimSpace(username) != username || utf8.RuneCountInString(username) > 100 || strings.IndexFunc(username, unicode.IsControl) >= 0 {
		return ErrInvalidUsername
	}
	if !utf8.ValidString(nickname) || utf8.RuneCountInString(nickname) > 100 || strings.IndexFunc(nickname, unicode.IsControl) >= 0 {
		return ErrInvalidNickname
	}
	return nil
}

// Username changes are disabled until a trusted reauthentication flow exists.
func CheckProfileUsername(current, requested string) error {
	if current != requested {
		return ErrUsernameReauthRequired
	}
	return nil
}

func (s *UserService) UpdateProfile(userID int64, username, nickname string) error {
	username = strings.TrimSpace(username)
	nickname = strings.TrimSpace(nickname)
	if err := ValidateProfile(username, nickname); err != nil {
		return err
	}
	user, err := s.userRepo.GetByID(userID)
	if err != nil {
		return err
	}
	if user == nil {
		return ErrUserNotFound
	}
	if err := CheckProfileUsername(user.Username, username); err != nil {
		return err
	}
	if err = s.userRepo.UpdateProfile(userID, nickname); err != nil {
		var mysqlErr *mysql.MySQLError
		if errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 {
			return ErrUsernameExists
		}
		return err
	}
	return nil
}

func (s *UserService) SetInitialCredentials(userID int64, password string) error {
	if len(password) < 6 {
		return ErrInvalidPassword
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	if err := s.userRepo.UpdateInitialCredentials(userID, string(hash)); err != nil {
		return err
	}
	return nil
}
func (s *UserService) ChangePassword(userID int64, currentPassword, newPassword string) error {
	user, err := s.userRepo.GetByID(userID)
	if err != nil {
		return err
	}
	if user == nil {
		return ErrUserNotFound
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(currentPassword)); err != nil {
		return ErrInvalidPassword
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	return s.userRepo.UpdatePassword(userID, string(hash))
}

func (s *UserService) AddFavorite(userID, siteID int64) error {
	return s.userRepo.AddFavorite(userID, siteID)
}

func (s *UserService) RemoveFavorite(userID, siteID int64) error {
	return s.userRepo.RemoveFavorite(userID, siteID)
}

func (s *UserService) IsFavorite(userID, siteID int64) (bool, error) {
	return s.userRepo.IsFavorite(userID, siteID)
}

func (s *UserService) GetFavoriteIDs(userID int64) ([]int64, error) {
	return s.userRepo.GetFavorites(userID)
}

func (s *UserService) CountUsers() (int64, error) {
	return s.userRepo.CountUsers()
}

func (s *UserService) GetAllUsers() ([]*models.User, error) {
	return s.userRepo.GetAllUsers()
}

// GetAdminUsers returns one page and its linked identities without N+1 queries.
func (s *UserService) GetAdminUsers(query string, page, pageSize int) (*models.UserPage, error) {
	query = strings.TrimSpace(query)
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	total, err := s.userRepo.CountUsersMatching(query)
	if err != nil {
		return nil, err
	}
	totalPages := int((total + int64(pageSize) - 1) / int64(pageSize))
	if totalPages == 0 {
		totalPages = 1
	}
	if page > totalPages {
		page = totalPages
	}
	users, err := s.userRepo.ListUsersPage(query, pageSize, (page-1)*pageSize)
	if err != nil {
		return nil, err
	}
	userIDs := make([]int64, len(users))
	for i, user := range users {
		userIDs[i] = user.ID
	}
	identities, err := s.userRepo.ListOAuthIdentitiesByUserIDs(userIDs)
	if err != nil {
		return nil, err
	}
	for _, user := range users {
		user.OAuthIdentities = identities[user.ID]
	}
	return &models.UserPage{Users: users, Total: total, Page: page, PageSize: pageSize, TotalPages: totalPages}, nil
}

func (s *UserService) HasAnyUser() (bool, error) {
	count, err := s.userRepo.CountUsers()
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (s *UserService) RegisterAdmin(username, password string) (*models.User, error) {
	hasUser, err := s.HasAnyUser()
	if err != nil {
		return nil, err
	}
	if hasUser {
		return nil, errors.New("admin already exists")
	}

	if existing, _ := s.userRepo.GetByUsername(username); existing != nil {
		return nil, ErrUsernameExists
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	user := &models.User{
		Username:     username,
		PasswordHash: string(hash),
		Role:         "admin",
	}

	id, err := s.userRepo.Create(user)
	if err != nil {
		return nil, err
	}

	user.ID = id
	return user, nil
}

func (s *UserService) CountTodayUsers() (int64, error) {
	return s.userRepo.CountTodayUsers()
}

func (s *UserService) GetRecentUsers(limit int) ([]*models.User, error) {
	if limit <= 0 {
		limit = 5
	}
	return s.userRepo.GetRecentUsers(limit)
}

// IsBlocked returns true if the user is blocked
func (s *UserService) IsBlocked(user *models.User) bool {
	return user.Status == "blocked"
}

// ResetPasswordByAdmin allows admin to reset another user's password without old password check
func (s *UserService) ResetPasswordByAdmin(userID int64, newPassword string) error {
	user, err := s.userRepo.GetByID(userID)
	if err != nil {
		return err
	}
	if user == nil {
		return ErrUserNotFound
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	return s.userRepo.UpdatePassword(userID, string(hash))
}

// SetUserRole allows admin to set another user's role
func (s *UserService) SetUserRole(adminID, targetUserID int64, newRole string) error {
	if adminID == targetUserID {
		return ErrCannotModifySelfRole
	}
	if newRole != "admin" && newRole != "user" {
		return ErrInvalidRole
	}

	user, err := s.userRepo.GetByID(targetUserID)
	if err != nil {
		return err
	}
	if user == nil {
		return ErrUserNotFound
	}

	return s.userRepo.UpdateRole(targetUserID, newRole)
}

// SetUserStatus allows admin to set another user's status
func (s *UserService) SetUserStatus(adminID, targetUserID int64, newStatus string) error {
	if adminID == targetUserID && newStatus == "blocked" {
		return ErrCannotBlockSelf
	}
	if newStatus != "active" && newStatus != "blocked" {
		return ErrInvalidStatus
	}

	user, err := s.userRepo.GetByID(targetUserID)
	if err != nil {
		return err
	}
	if user == nil {
		return ErrUserNotFound
	}

	return s.userRepo.UpdateStatus(targetUserID, newStatus)
}
