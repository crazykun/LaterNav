package services

import (
	"ai-later-nav/internal/database"
	"ai-later-nav/internal/database/repository"
	"ai-later-nav/internal/models"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"log"
	"regexp"
	"strings"
	"sync"
)

var (
	providerInstance *OAuthProviderService
	providerOnce     sync.Once
	slugPattern      = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,31}$`)
)

// ErrInvalidProviderSlug is returned when a slug is empty, malformed, or reserved.
var ErrInvalidProviderSlug = errors.New("invalid provider slug")

// ErrProviderInUse is returned when deleting a provider that still has bound identities.
var ErrProviderInUse = errors.New("provider has bound identities")

type OAuthProviderService struct {
	repo *repository.OAuthProviderRepository
}

func GetOAuthProviderService() *OAuthProviderService {
	providerOnce.Do(func() {
		providerInstance = &OAuthProviderService{repo: repository.NewOAuthProviderRepository()}
	})
	return providerInstance
}

// ValidSlug reports whether s is a safe route segment and not a reserved word.
func ValidSlug(s string) bool {
	return slugPattern.MatchString(s) && s != "callback" && s != "login"
}

// IdentityKey derives the stable oauth_identities.provider key for a configuration.
// It is bound to the issuer (OIDC) or authorize endpoint (userinfo flows) plus the
// client ID, so renaming a slug never re-binds existing accounts.
func IdentityKey(typ, issuer, authorizeURL, clientID string) string {
	if typ == "oidc" && issuer == "https://linux.sb" {
		// Legacy Linux.sb identities predate hashed keys; keep their namespace.
		return "linux.sb"
	}
	base := issuer
	if base == "" {
		base = authorizeURL
	}
	sum := sha256.Sum256([]byte(base + "\x00" + clientID))
	prefix := "oidc_"
	if typ != "oidc" {
		prefix = typ + "_"
	}
	return prefix + base64.RawURLEncoding.EncodeToString(sum[:])[:43]
}

func (s *OAuthProviderService) GetBySlug(slug string) (*models.OAuthProvider, error) {
	return s.repo.GetBySlug(slug)
}

func (s *OAuthProviderService) List() ([]*models.OAuthProvider, error) {
	return s.repo.List()
}

// LoginProviders returns enabled providers suitable for rendering login buttons.
func (s *OAuthProviderService) LoginProviders() ([]*models.OAuthProvider, error) {
	all, err := s.repo.ListEnabled()
	if err != nil {
		return nil, err
	}
	ready := all[:0]
	for _, p := range all {
		if p.Name != "" && p.ClientID != "" && p.ClientSecret != "" && p.RedirectURL != "" {
			ready = append(ready, p)
		}
	}
	return ready, nil
}

// Save validates and persists a provider. ClientSecret empty keeps the existing secret.
func (s *OAuthProviderService) Save(p *models.OAuthProvider, secretProvided bool) error {
	if !ValidSlug(p.Slug) {
		return ErrInvalidProviderSlug
	}
	if p.Type != "oidc" && p.Type != "github" {
		return errors.New("unsupported provider type")
	}
	p.IdentityKey = IdentityKey(p.Type, p.Issuer, p.AuthorizeURL, p.ClientID)
	if !secretProvided {
		if old, err := s.repo.GetBySlug(p.Slug); err != nil {
			return err
		} else if old != nil {
			p.ClientSecret = old.ClientSecret
		}
	}
	return s.repo.Upsert(p)
}

// Delete removes a provider unless accounts are bound to it.
func (s *OAuthProviderService) Delete(slug string) error {
	if err := s.repo.Delete(slug); err != nil {
		if strings.Contains(err.Error(), "bound identities") {
			return ErrProviderInUse
		}
		return err
	}
	return nil
}

// MigrateLegacySettings copies the single-provider oidc_* system settings into a
// default provider row. Idempotent: runs only when the row is absent and the
// settings carry real values. Safe to call on every boot.
func (s *OAuthProviderService) MigrateLegacySettings(ss *SettingService) error {
	if database.DB == nil {
		return nil
	}
	existing, err := s.repo.GetBySlug("default")
	if err != nil || existing != nil {
		return err
	}
	clientID := ss.GetSetting("oidc_client_id")
	if clientID == "" {
		return nil // nothing configured before the table existed
	}
	issuer := ss.GetSetting("oidc_issuer")
	p := &models.OAuthProvider{
		Slug:         "default",
		Name:         valueOr(ss.GetSetting("oidc_name"), "Linux.sb"),
		Type:         "oidc",
		Issuer:       issuer,
		DiscoveryURL: ss.GetSetting("oidc_discovery_url"),
		ClientID:     clientID,
		ClientSecret: ss.GetSetting("oidc_client_secret"),
		RedirectURL:  rewriteLegacyRedirect(ss.GetSetting("oidc_redirect_url")),
		Enabled:      ss.GetSetting("oidc_enabled") == "true",
	}
	p.IdentityKey = IdentityKey("oidc", issuer, "", clientID)
	if p.RedirectURL == "" || p.ClientSecret == "" {
		// Incomplete legacy config: import disabled so the admin finishes it in the UI.
		p.Enabled = false
	}
	if err := s.repo.Upsert(p); err != nil {
		return err
	}
	log.Printf("Migrated legacy oidc_* settings into oauth_providers row 'default'")
	return nil
}

func valueOr(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}

// rewriteLegacyRedirect points the imported row at the new default-slug callback.
func rewriteLegacyRedirect(u string) string {
	if strings.HasSuffix(u, "/oauth/callback") {
		return strings.TrimSuffix(u, "/oauth/callback") + "/oauth/default/callback"
	}
	return u
}
