package repository

import (
	"ai-later-nav/internal/database"
	"ai-later-nav/internal/models"
	"database/sql"
	"fmt"
)

type OAuthProviderRepository struct {
	db *sql.DB
}

func NewOAuthProviderRepository() *OAuthProviderRepository {
	return &OAuthProviderRepository{db: database.DB}
}

const providerColumns = "slug, name, type, identity_key, issuer, discovery_url, authorize_url, token_url, userinfo_url, client_id, client_secret, redirect_url, scopes, enabled, sort"

func scanProvider(row interface{ Scan(...any) error }) (*models.OAuthProvider, error) {
	var p models.OAuthProvider
	err := row.Scan(&p.Slug, &p.Name, &p.Type, &p.IdentityKey, &p.Issuer, &p.DiscoveryURL, &p.AuthorizeURL, &p.TokenURL, &p.UserinfoURL, &p.ClientID, &p.ClientSecret, &p.RedirectURL, &p.Scopes, &p.Enabled, &p.Sort)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (r *OAuthProviderRepository) GetBySlug(slug string) (*models.OAuthProvider, error) {
	p, err := scanProvider(r.db.QueryRow("SELECT "+providerColumns+" FROM oauth_providers WHERE slug = ?", slug))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get provider %s: %w", slug, err)
	}
	return p, nil
}

func (r *OAuthProviderRepository) List() ([]*models.OAuthProvider, error) {
	rows, err := r.db.Query("SELECT " + providerColumns + " FROM oauth_providers ORDER BY sort, slug")
	if err != nil {
		return nil, fmt.Errorf("list providers: %w", err)
	}
	defer rows.Close()
	var out []*models.OAuthProvider
	for rows.Next() {
		p, err := scanProvider(rows)
		if err != nil {
			return nil, fmt.Errorf("scan provider: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// ListEnabled returns providers shown as login buttons, ordered for display.
func (r *OAuthProviderRepository) ListEnabled() ([]*models.OAuthProvider, error) {
	rows, err := r.db.Query("SELECT " + providerColumns + " FROM oauth_providers WHERE enabled = 1 ORDER BY sort, slug")
	if err != nil {
		return nil, fmt.Errorf("list enabled providers: %w", err)
	}
	defer rows.Close()
	var out []*models.OAuthProvider
	for rows.Next() {
		p, err := scanProvider(rows)
		if err != nil {
			return nil, fmt.Errorf("scan provider: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *OAuthProviderRepository) Upsert(p *models.OAuthProvider) error {
	_, err := r.db.Exec(`INSERT INTO oauth_providers (slug, name, type, identity_key, issuer, discovery_url, authorize_url, token_url, userinfo_url, client_id, client_secret, redirect_url, scopes, enabled, sort)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE name=VALUES(name), type=VALUES(type), identity_key=VALUES(identity_key), issuer=VALUES(issuer), discovery_url=VALUES(discovery_url), authorize_url=VALUES(authorize_url), token_url=VALUES(token_url), userinfo_url=VALUES(userinfo_url), client_id=VALUES(client_id), client_secret=VALUES(client_secret), redirect_url=VALUES(redirect_url), scopes=VALUES(scopes), enabled=VALUES(enabled), sort=VALUES(sort)`,
		p.Slug, p.Name, p.Type, p.IdentityKey, p.Issuer, p.DiscoveryURL, p.AuthorizeURL, p.TokenURL, p.UserinfoURL, p.ClientID, p.ClientSecret, p.RedirectURL, p.Scopes, p.Enabled, p.Sort)
	if err != nil {
		return fmt.Errorf("upsert provider %s: %w", p.Slug, err)
	}
	return nil
}

// Delete removes a provider only when no identity is bound to it.
func (r *OAuthProviderRepository) Delete(slug string) error {
	p, err := r.GetBySlug(slug)
	if err != nil {
		return err
	}
	if p == nil {
		return nil
	}
	var bound bool
	if err := r.db.QueryRow("SELECT EXISTS(SELECT 1 FROM oauth_identities WHERE provider = ? LIMIT 1)", p.IdentityKey).Scan(&bound); err != nil {
		return fmt.Errorf("check provider bindings: %w", err)
	}
	if bound {
		return fmt.Errorf("provider %s still has bound identities", slug)
	}
	if _, err := r.db.Exec("DELETE FROM oauth_providers WHERE slug = ?", slug); err != nil {
		return fmt.Errorf("delete provider %s: %w", slug, err)
	}
	return nil
}

// HasBindings reports whether any account is bound to the given identity key.
func (r *OAuthProviderRepository) HasBindings(identityKey string) (bool, error) {
	var bound bool
	err := r.db.QueryRow("SELECT EXISTS(SELECT 1 FROM oauth_identities WHERE provider = ? LIMIT 1)", identityKey).Scan(&bound)
	return bound, err
}
