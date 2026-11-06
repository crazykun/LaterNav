package handlers

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"strings"
	"sync"

	"ai-later-nav/internal/middleware"
	"ai-later-nav/internal/models"
	"ai-later-nav/internal/services"

	"github.com/gin-gonic/gin"
)

// OAuthRegistry lazily builds and caches a login handler per provider slug.
// Handlers are rebuilt whenever the underlying provider configuration changes.
type OAuthRegistry struct {
	providers *services.OAuthProviderService
	mu        sync.Mutex
	handlers  map[string]any // slug -> *OIDCHandler | *GitHubHandler
	keys      map[string]string
}

func NewOAuthRegistry(ps *services.OAuthProviderService) *OAuthRegistry {
	return &OAuthRegistry{providers: ps, handlers: make(map[string]any), keys: make(map[string]string)}
}

func providerConfigKey(p *models.OAuthProvider) string {
	return strings.Join([]string{p.Type, p.IdentityKey, p.Issuer, p.DiscoveryURL, p.AuthorizeURL, p.TokenURL, p.UserinfoURL, p.ClientID, p.ClientSecret, p.RedirectURL, p.Scopes}, "\x00")
}

func (r *OAuthRegistry) handler(slug string) (any, *models.OAuthProvider, error) {
	p, err := r.providers.GetBySlug(slug)
	if err != nil {
		return nil, nil, err
	}
	if p == nil || !p.Enabled {
		return nil, nil, errProviderUnavailable
	}
	key := providerConfigKey(p)
	r.mu.Lock()
	defer r.mu.Unlock()
	if h, ok := r.handlers[slug]; ok && r.keys[slug] == key {
		return h, p, nil
	}
	var h any
	switch p.Type {
	case "oidc":
		scopes := strings.Fields(p.Scopes)
		if len(scopes) == 0 {
			scopes = nil // NewOIDCHandler applies its default openid profile email
		}
		oh, err := newOIDCHandlerWithScopes(context.Background(), OIDCConfig{Issuer: p.Issuer, DiscoveryURL: p.DiscoveryURL, ClientID: p.ClientID, ClientSecret: p.ClientSecret, RedirectURL: p.RedirectURL}, scopes)
		if err != nil {
			return nil, nil, err
		}
		h = oh
	case "github":
		gh, err := NewGitHubHandler(GitHubConfig{AuthorizeURL: p.AuthorizeURL, TokenURL: p.TokenURL, UserinfoURL: p.UserinfoURL, ClientID: p.ClientID, ClientSecret: p.ClientSecret, RedirectURL: p.RedirectURL})
		if err != nil {
			return nil, nil, err
		}
		h = gh
	default:
		return nil, nil, errProviderUnavailable
	}
	r.handlers[slug] = h
	r.keys[slug] = key
	return h, p, nil
}

var errProviderUnavailable = &oauthError{"三方登录未配置或未启用"}

type oauthError struct{ msg string }

func (e *oauthError) Error() string { return e.msg }

// StartLogin dispatches /oauth/:slug/login to the right provider flow.
func (r *OAuthRegistry) StartLogin(c *gin.Context) {
	h, _, err := r.handler(c.Param("slug"))
	if err != nil {
		c.String(http.StatusServiceUnavailable, "该登录方式暂不可用")
		return
	}
	switch v := h.(type) {
	case *OIDCHandler:
		v.Start(c)
	case *GitHubHandler:
		v.Start(c)
	}
}

// Callback dispatches /oauth/:slug/callback, resolves the identity, and issues the session.
func (r *OAuthRegistry) Callback(c *gin.Context) {
	h, p, err := r.handler(c.Param("slug"))
	if err != nil {
		c.String(http.StatusServiceUnavailable, "该登录方式暂不可用")
		return
	}
	var subject, preferred, display string
	secure := false
	switch v := h.(type) {
	case *OIDCHandler:
		result, err := v.VerifyCallback(c)
		if err != nil {
			c.String(http.StatusBadRequest, "三方登录失败，请重新登录")
			return
		}
		if result.Reauthenticated {
			c.AbortWithStatus(http.StatusBadRequest)
			return
		}
		subject, preferred, display = result.Subject, result.PreferredUsername, result.DisplayName
		secure = strings.HasPrefix(p.RedirectURL, "https://")
	case *GitHubHandler:
		result, err := v.VerifyCallback(c)
		if err != nil {
			c.String(http.StatusBadRequest, "三方登录失败，请重新登录")
			return
		}
		subject, preferred, display = result.Subject, result.PreferredUsername, result.DisplayName
		secure = strings.HasPrefix(p.RedirectURL, "https://")
	}
	user, err := databaseOIDCUser(p.IdentityKey, subject, preferred, display)
	if err != nil {
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}
	if user.Status != "active" {
		c.AbortWithStatus(http.StatusForbidden)
		return
	}
	token, err := middleware.GenerateToken(user)
	if err != nil {
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie("token", token, 86400*7, "/", "", secure, true)
	c.Redirect(http.StatusFound, "/")
}

// ProviderKeyForIssuer preserves the hashed-key derivation for tests and migration checks.
func ProviderKeyForIssuer(issuer, clientID string) string {
	sum := sha256.Sum256([]byte(issuer + "\x00" + clientID))
	return "oidc_" + base64.RawURLEncoding.EncodeToString(sum[:])
}


