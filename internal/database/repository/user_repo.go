package repository

import (
	"ai-later-nav/internal/database"
	"ai-later-nav/internal/models"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

type UserRepository struct {
	db *sql.DB
}

func NewUserRepository() *UserRepository {
	return &UserRepository{db: database.DB}
}

func (r *UserRepository) Create(user *models.User) (int64, error) {
	result, err := r.db.Exec(`
		INSERT INTO users (username, password_hash, role)
		VALUES (?, ?, ?)
	`, user.Username, user.PasswordHash, user.Role)
	if err != nil {
		return 0, fmt.Errorf("insert user: %w", err)
	}
	return result.LastInsertId()
}

func (r *UserRepository) GetByID(id int64) (*models.User, error) {
	var u models.User
	err := r.db.QueryRow(`
		SELECT id, username, nickname, COALESCE(email, ''), password_hash, role, status, created_at, updated_at
		FROM users WHERE id = ?
	`, id).Scan(&u.ID, &u.Username, &u.Nickname, &u.Email, &u.PasswordHash, &u.Role, &u.Status, &u.CreatedAt, &u.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("query user by id: %w", err)
	}
	return &u, nil
}

func (r *UserRepository) GetByUsername(username string) (*models.User, error) {
	var u models.User
	err := r.db.QueryRow(`
		SELECT id, username, nickname, COALESCE(email, ''), password_hash, role, status, created_at, updated_at
		FROM users WHERE username = ?
	`, username).Scan(&u.ID, &u.Username, &u.Nickname, &u.Email, &u.PasswordHash, &u.Role, &u.Status, &u.CreatedAt, &u.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("query user by username: %w", err)
	}
	return &u, nil
}

func (r *UserRepository) UpdateProfile(userID int64, nickname string) error {
	_, err := r.db.Exec("UPDATE users SET nickname = ?, updated_at = NOW() WHERE id = ?", nickname, userID)
	if err != nil {
		return fmt.Errorf("update profile: %w", err)
	}
	return nil
}

func (r *UserRepository) UpdateInitialCredentials(userID int64, passwordHash string) error {
	result, err := r.db.Exec("UPDATE users SET password_hash = ?, updated_at = NOW() WHERE id = ? AND password_hash = '!'", passwordHash, userID)
	if err != nil {
		return fmt.Errorf("update initial credentials: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("update initial credentials rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("initial credentials already set or user not found")
	}
	return nil
}
func (r *UserRepository) UpdatePassword(userID int64, passwordHash string) error {
	_, err := r.db.Exec(`
		UPDATE users SET password_hash = ?, updated_at = NOW() WHERE id = ?
	`, passwordHash, userID)
	if err != nil {
		return fmt.Errorf("update password: %w", err)
	}
	return nil
}

func (r *UserRepository) UpdateRole(userID int64, role string) error {
	_, err := r.db.Exec("UPDATE users SET role = ?, updated_at = NOW() WHERE id = ?", role, userID)
	if err != nil {
		return fmt.Errorf("update role: %w", err)
	}
	return nil
}

func (r *UserRepository) UpdateStatus(userID int64, status string) error {
	result, err := r.db.Exec("UPDATE users SET status = ?, updated_at = NOW() WHERE id = ?", status, userID)
	if err != nil {
		return fmt.Errorf("update status: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("update status rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("update status: user %d not found", userID)
	}
	return nil
}

// ErrUserDeleteNotFound 表示事务内目标用户行已不存在（并发删除）。
var ErrUserDeleteNotFound = errors.New("user not found during delete")

// CountActiveAdmins 统计仍可登录后台的管理员数量，用于防止删掉最后一个管理员。
func (r *UserRepository) CountActiveAdmins() (int64, error) {
	var count int64
	err := r.db.QueryRow(`SELECT COUNT(*) FROM users WHERE role = 'admin' AND status = 'active'`).Scan(&count)
	return count, err
}

// DeleteByID 在单事务内删除用户及其关联数据，避免留下孤儿记录。
// favorites / oauth_identities 由外键 ON DELETE CASCADE 自动清除；
// site_submissions 和 visits 无外键，需显式处理：
//   - 该用户提交的投稿一并删除（审核页 ListByStatus 用 INNER JOIN users，保留会静默丢失）
//   - 别人投稿中 reviewed_by 指向该用户的置空
//   - 访问记录保留统计价值，仅把 user_id 匿名化
func (r *UserRepository) DeleteByID(userID int64) error {
	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("begin delete user: %w", err)
	}
	defer tx.Rollback()

	// 锁住目标行，防止并发下重复删除或删到一半被其他操作改动。
	var exists int
	if err := tx.QueryRow(`SELECT 1 FROM users WHERE id = ? FOR UPDATE`, userID).Scan(&exists); err == sql.ErrNoRows {
		return ErrUserDeleteNotFound
	} else if err != nil {
		return fmt.Errorf("lock user for delete: %w", err)
	}

	steps := []struct {
		query string
		args  []any
	}{
		{`DELETE FROM site_submissions WHERE user_id = ?`, []any{userID}},
		{`UPDATE site_submissions SET reviewed_by = NULL WHERE reviewed_by = ?`, []any{userID}},
		{`UPDATE visits SET user_id = NULL WHERE user_id = ?`, []any{userID}},
		{`DELETE FROM users WHERE id = ?`, []any{userID}},
	}
	for _, step := range steps {
		if _, err := tx.Exec(step.query, step.args...); err != nil {
			return fmt.Errorf("delete user %d step %q: %w", userID, step.query, err)
		}
	}
	return tx.Commit()
}

func (r *UserRepository) AddFavorite(userID, siteID int64) error {
	_, err := r.db.Exec("INSERT IGNORE INTO favorites (user_id, site_id) VALUES (?, ?)", userID, siteID)
	return err
}

func (r *UserRepository) RemoveFavorite(userID, siteID int64) error {
	_, err := r.db.Exec("DELETE FROM favorites WHERE user_id = ? AND site_id = ?", userID, siteID)
	return err
}

func (r *UserRepository) IsFavorite(userID, siteID int64) (bool, error) {
	var count int
	err := r.db.QueryRow("SELECT COUNT(*) FROM favorites WHERE user_id = ? AND site_id = ?", userID, siteID).Scan(&count)
	return count > 0, err
}

func (r *UserRepository) GetFavorites(userID int64) ([]int64, error) {
	rows, err := r.db.Query("SELECT site_id FROM favorites WHERE user_id = ? ORDER BY created_at DESC", userID)
	if err != nil {
		return nil, fmt.Errorf("query favorites: %w", err)
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan favorite: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (r *UserRepository) CountUsers() (int64, error) {
	var count int64
	err := r.db.QueryRow("SELECT COUNT(*) FROM users").Scan(&count)
	return count, err
}

func (r *UserRepository) GetAllUsers() ([]*models.User, error) {
	rows, err := r.db.Query(`
		SELECT id, username, nickname, COALESCE(email, ''), password_hash, role, status, created_at, updated_at
		FROM users ORDER BY id DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("query all users: %w", err)
	}
	defer rows.Close()

	var users []*models.User
	for rows.Next() {
		var u models.User
		if err := rows.Scan(&u.ID, &u.Username, &u.Nickname, &u.Email, &u.PasswordHash, &u.Role, &u.Status, &u.CreatedAt, &u.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan user: %w", err)
		}
		users = append(users, &u)
	}
	return users, rows.Err()
}

// adminUserFilter returns a WHERE clause shared by the page query and its count.
// EXISTS keeps users with multiple OAuth identities from being counted twice.
func adminUserFilter(query string) (string, []interface{}) {
	if query == "" {
		return "", nil
	}
	pattern := "%" + query + "%"
	return ` WHERE (
		username LIKE ? OR nickname LIKE ? OR COALESCE(email, '') LIKE ?
		OR EXISTS (SELECT 1 FROM oauth_identities oi WHERE oi.user_id = users.id AND oi.email LIKE ?)
	)`, []interface{}{pattern, pattern, pattern, pattern}
}

func (r *UserRepository) CountUsersMatching(query string) (int64, error) {
	where, args := adminUserFilter(query)
	var count int64
	err := r.db.QueryRow(`SELECT COUNT(*) FROM users`+where, args...).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count filtered users: %w", err)
	}
	return count, nil
}

func (r *UserRepository) ListUsersPage(query string, limit, offset int) ([]*models.AdminUser, error) {
	where, args := adminUserFilter(query)
	args = append(args, limit, offset)
	rows, err := r.db.Query(`
		SELECT id, username, nickname, COALESCE(email, ''), role, status, created_at, updated_at
		FROM users`+where+` ORDER BY id DESC LIMIT ? OFFSET ?`, args...)
	if err != nil {
		return nil, fmt.Errorf("query users page: %w", err)
	}
	defer rows.Close()

	users := make([]*models.AdminUser, 0, limit)
	for rows.Next() {
		u := &models.AdminUser{}
		if err := rows.Scan(&u.ID, &u.Username, &u.Nickname, &u.Email, &u.Role, &u.Status, &u.CreatedAt, &u.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan user page: %w", err)
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

// ListOAuthIdentitiesByUserIDs loads all profiles for one page in a single query.
func (r *UserRepository) ListOAuthIdentitiesByUserIDs(userIDs []int64) (map[int64][]models.OAuthIdentity, error) {
	identities := make(map[int64][]models.OAuthIdentity)
	if len(userIDs) == 0 {
		return identities, nil
	}
	placeholders := make([]string, len(userIDs))
	args := make([]interface{}, len(userIDs))
	for i, id := range userIDs {
		placeholders[i] = "?"
		args[i] = id
	}
	rows, err := r.db.Query(`
		SELECT oi.user_id, oi.provider, COALESCE(op.name, ''), COALESCE(op.type, ''),
			oi.preferred_username, oi.display_name, oi.picture, oi.email
		FROM oauth_identities oi
		LEFT JOIN oauth_providers op ON op.identity_key = oi.provider
		WHERE oi.user_id IN (`+strings.Join(placeholders, ",")+`)
		ORDER BY oi.user_id, op.sort, oi.provider`, args...)
	if err != nil {
		return nil, fmt.Errorf("query user oauth identities: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var userID int64
		var identity models.OAuthIdentity
		if err := rows.Scan(&userID, &identity.Provider, &identity.ProviderName, &identity.ProviderType,
			&identity.PreferredUsername, &identity.DisplayName, &identity.Picture, &identity.Email); err != nil {
			return nil, fmt.Errorf("scan user oauth identity: %w", err)
		}
		identities[userID] = append(identities[userID], identity)
	}
	return identities, rows.Err()
}

func (r *UserRepository) CountTodayUsers() (int64, error) {
	var count int64
	err := r.db.QueryRow("SELECT COUNT(*) FROM users WHERE DATE(created_at) = CURDATE()").Scan(&count)
	return count, err
}

func (r *UserRepository) GetRecentUsers(limit int) ([]*models.User, error) {
	rows, err := r.db.Query(`
		SELECT id, username, nickname, COALESCE(email, ''), password_hash, role, status, created_at, updated_at
		FROM users ORDER BY created_at DESC LIMIT ?
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("query recent users: %w", err)
	}
	defer rows.Close()

	var users []*models.User
	for rows.Next() {
		var u models.User
		if err := rows.Scan(&u.ID, &u.Username, &u.Nickname, &u.Email, &u.PasswordHash, &u.Role, &u.Status, &u.CreatedAt, &u.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan user: %w", err)
		}
		users = append(users, &u)
	}
	return users, rows.Err()
}
