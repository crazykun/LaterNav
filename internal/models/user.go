package models

import "time"

type User struct {
	ID           int64     `json:"id" db:"id"`
	Username     string    `json:"username" db:"username"`
	Nickname     string    `json:"nickname" db:"nickname"`
	AvatarURL    string    `json:"avatar_url" db:"avatar_url"`
	Email        string    `json:"email" db:"email"`
	PasswordHash string    `json:"-" db:"password_hash"`
	Role         string    `json:"role" db:"role"`
	Status       string    `json:"status" db:"status"`
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time `json:"updated_at" db:"updated_at"`
}

type UserClaims struct {
	UserID   int64  `json:"user_id"`
	Username string `json:"username"`
	Role     string `json:"role"`
}

// DisplayName prefers the user-chosen nickname over the login name.
func (u User) DisplayName() string {
	if u.Nickname != "" {
		return u.Nickname
	}
	return u.Username
}

// OAuthIdentity is the profile persisted for one linked third-party login.
type OAuthIdentity struct {
	Provider          string `json:"provider"`
	ProviderName      string `json:"provider_name"`
	ProviderType      string `json:"provider_type"`
	ProviderSlug      string `json:"provider_slug"`
	Subject           string `json:"subject"`
	PreferredUsername string `json:"preferred_username"`
	DisplayName       string `json:"display_name"`
	Picture           string `json:"picture"`
	Email             string `json:"email"`
}

func (i OAuthIdentity) Name() string {
	if i.DisplayName != "" {
		return i.DisplayName
	}
	if i.PreferredUsername != "" {
		return i.PreferredUsername
	}
	return "未提供"
}

func (i OAuthIdentity) ProviderLabel() string {
	if i.ProviderName != "" {
		return i.ProviderName
	}
	return i.Provider
}

// AdminUser combines account data with all linked identities for the admin list.
type AdminUser struct {
	User
	OAuthIdentities []OAuthIdentity
}

type UserPage struct {
	Users      []*AdminUser
	Total      int64
	Page       int
	PageSize   int
	TotalPages int
}
