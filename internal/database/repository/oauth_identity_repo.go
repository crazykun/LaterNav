package repository

import (
	"database/sql"
	"errors"
	"fmt"

	"ai-later-nav/internal/database"
	"ai-later-nav/internal/models"
)

var (
	ErrIdentityOwnedElsewhere = errors.New("该三方账号已被其他账号绑定")
	ErrProviderAlreadyBound   = errors.New("该平台已绑定，请先解绑后再绑定新账号")
	ErrIdentityNotFound       = errors.New("未找到该绑定记录")
)

// OAuthIdentityProfile 是绑定时写入身份表的资料快照，字段与 handlers 的 oauthIdentityProfile 对齐。
type OAuthIdentityProfile struct {
	Subject           string
	PreferredUsername string
	DisplayName       string
	Email             string
	Picture           string
}

type OAuthIdentityRepository struct{ db *sql.DB }

func NewOAuthIdentityRepository() *OAuthIdentityRepository {
	return &OAuthIdentityRepository{db: database.DB}
}

func (r *OAuthIdentityRepository) ListByUserID(userID int64) ([]models.OAuthIdentity, error) {
	rows, err := r.db.Query(`
		SELECT oi.provider, COALESCE(op.name, ''), COALESCE(op.type, ''), COALESCE(op.slug, ''),
			CAST(oi.subject AS CHAR), oi.preferred_username, oi.display_name, oi.picture, oi.email
		FROM oauth_identities oi
		LEFT JOIN oauth_providers op ON op.identity_key = oi.provider
		WHERE oi.user_id = ?
		ORDER BY op.sort, oi.provider`, userID)
	if err != nil {
		return nil, fmt.Errorf("list oauth identities by user: %w", err)
	}
	defer rows.Close()

	var identities []models.OAuthIdentity
	for rows.Next() {
		var identity models.OAuthIdentity
		if err := rows.Scan(&identity.Provider, &identity.ProviderName, &identity.ProviderType,
			&identity.ProviderSlug, &identity.Subject, &identity.PreferredUsername,
			&identity.DisplayName, &identity.Picture, &identity.Email); err != nil {
			return nil, fmt.Errorf("scan oauth identity: %w", err)
		}
		identities = append(identities, identity)
	}
	return identities, rows.Err()
}

func (r *OAuthIdentityRepository) CountByUserID(userID int64) (int, error) {
	var count int
	err := r.db.QueryRow(`SELECT COUNT(*) FROM oauth_identities WHERE user_id = ?`, userID).Scan(&count)
	return count, err
}

// Bind 把一个三方身份绑定到指定用户，处理三类冲突。
func (r *OAuthIdentityRepository) Bind(userID int64, providerKey string, profile OAuthIdentityProfile) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var ownerID int64
	err = tx.QueryRow(`SELECT user_id FROM oauth_identities WHERE provider = ? AND subject = ? FOR UPDATE`,
		providerKey, profile.Subject).Scan(&ownerID)
	switch {
	case err == nil && ownerID == userID:
		// 已经绑定到当前用户，仅刷新资料。
		_, err = tx.Exec(`UPDATE oauth_identities SET preferred_username=?, display_name=?, picture=?, email=? WHERE provider=? AND subject=?`,
			profile.PreferredUsername, profile.DisplayName, profile.Picture, profile.Email, providerKey, profile.Subject)
		if err != nil {
			return fmt.Errorf("refresh bound identity: %w", err)
		}
		return tx.Commit()
	case err == nil:
		return ErrIdentityOwnedElsewhere
	case err != sql.ErrNoRows:
		return fmt.Errorf("query existing identity: %w", err)
	}

	var bound int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM oauth_identities WHERE user_id = ? AND provider = ?`, userID, providerKey).Scan(&bound); err != nil {
		return fmt.Errorf("check provider binding: %w", err)
	}
	if bound > 0 {
		return ErrProviderAlreadyBound
	}

	_, err = tx.Exec(`INSERT INTO oauth_identities (provider, subject, user_id, preferred_username, display_name, picture, email)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		providerKey, profile.Subject, userID, profile.PreferredUsername, profile.DisplayName, profile.Picture, profile.Email)
	if err != nil {
		return fmt.Errorf("insert oauth identity: %w", err)
	}
	return tx.Commit()
}

func (r *OAuthIdentityRepository) Unbind(userID int64, providerKey string) error {
	result, err := r.db.Exec(`DELETE FROM oauth_identities WHERE user_id = ? AND provider = ?`, userID, providerKey)
	if err != nil {
		return fmt.Errorf("delete oauth identity: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("unbind rows affected: %w", err)
	}
	if affected == 0 {
		return ErrIdentityNotFound
	}
	return nil
}
