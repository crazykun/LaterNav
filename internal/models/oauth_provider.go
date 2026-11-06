package models

import "time"

// OAuthProvider is one configured third-party login provider.
// IdentityKey is the value stored in oauth_identities.provider; it is derived
// from the issuer/endpoints + client ID, so renaming the slug never re-binds accounts.
type OAuthProvider struct {
	Slug         string
	Name         string
	Type         string // "oidc" | "github"
	IdentityKey  string
	Issuer       string
	DiscoveryURL string
	AuthorizeURL string
	TokenURL     string
	UserinfoURL  string
	ClientID     string
	ClientSecret string
	RedirectURL  string
	Scopes       string
	Enabled      bool
	Sort         int
	CreatedAt    time.Time
	UpdatedAt    time.Time
}
