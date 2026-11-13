package handlers

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"log"
	"net/http"
	"strings"
	"sync"

	"ai-later-nav/internal/database/repository"
	"ai-later-nav/internal/middleware"
	"ai-later-nav/internal/models"
	"ai-later-nav/internal/services"

	"github.com/gin-gonic/gin"
)

// identityBinder is the subset of the OAuth identity service used by the bind
// callback. An interface keeps callback tests independent of a live database.
type identityBinder interface {
	Bind(userID int64, providerKey string, profile repository.OAuthIdentityProfile) error
}

// oauthVerifier resolves a callback into a verified profile plus the binding
// intent stored in the one-time state transaction. Injectable for tests.
type oauthVerifier func(c *gin.Context, h any, p *models.OAuthProvider) (oauthIdentityProfile, int64, bool, error)

// OAuthRegistry lazily builds and caches a login handler per provider slug.
// Handlers are rebuilt whenever the underlying provider configuration changes.
type OAuthRegistry struct {
	providers   *services.OAuthProviderService
	mu          sync.Mutex
	handlers    map[string]any // slug -> *OIDCHandler | *GitHubHandler
	keys        map[string]string
	bindService identityBinder // nil -> services.NewOAuthIdentityService()
	verify      oauthVerifier  // nil -> defaultOAuthVerify
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

// StartLogin dispatches /oauth/:slug/login to the right provider flow (login only).
// Binding is never started from this GET: it must come from a CSRF-protected POST.
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

// StartBind launches a link-to-current-account flow. It is only called from the
// authenticated, CSRF-checked POST start handler; the binding intent lives in
// the one-time in-memory state transaction, not in any URL.
func (r *OAuthRegistry) StartBind(c *gin.Context, slug string, userID int64) {
	h, _, err := r.handler(slug)
	if err != nil {
		c.String(http.StatusServiceUnavailable, "该登录方式暂不可用")
		return
	}
	switch v := h.(type) {
	case *OIDCHandler:
		v.StartBind(c, userID)
	case *GitHubHandler:
		v.StartBind(c, userID)
	}
}

// defaultOAuthVerify runs the real callback verification for OIDC/GitHub.
func defaultOAuthVerify(c *gin.Context, h any, _ *models.OAuthProvider) (oauthIdentityProfile, int64, bool, error) {
	switch v := h.(type) {
	case *OIDCHandler:
		result, err := v.VerifyCallback(c)
		if err != nil {
			return oauthIdentityProfile{}, 0, false, err
		}
		profile := oauthIdentityProfile{Subject: result.Subject, PreferredUsername: result.PreferredUsername, DisplayName: result.DisplayName, Email: result.Email, Picture: result.Picture}
		return profile, result.BindUserID, result.Reauthenticated, nil
	case *GitHubHandler:
		result, err := v.VerifyCallback(c)
		if err != nil {
			return oauthIdentityProfile{}, 0, false, err
		}
		profile := oauthIdentityProfile{Subject: result.Subject, PreferredUsername: result.PreferredUsername, DisplayName: result.DisplayName, Email: result.Email, Picture: result.Picture}
		return profile, result.BindUserID, false, nil
	}
	return oauthIdentityProfile{}, 0, false, errors.New("unknown oauth handler")
}

// Callback dispatches /oauth/:slug/callback. A callback carrying a binding
// intent links the identity to that account without issuing a new session;
// everything else follows the original login path unchanged.
func (r *OAuthRegistry) Callback(c *gin.Context) {
	h, p, err := r.handler(c.Param("slug"))
	if err != nil {
		c.String(http.StatusServiceUnavailable, "该登录方式暂不可用")
		return
	}
	verify := r.verify
	if verify == nil {
		verify = defaultOAuthVerify
	}
	profile, bindUserID, reauthenticated, err := verify(c, h, p)
	if err != nil {
		c.String(http.StatusBadRequest, "三方登录失败，请重新登录")
		return
	}
	if bindUserID > 0 {
		r.finishBind(c, p, profile, bindUserID)
		return
	}
	if reauthenticated {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}
	user, err := databaseOIDCUser(p.IdentityKey, profile)
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
	c.SetCookie("token", token, 86400*7, "/", "", strings.HasPrefix(p.RedirectURL, "https://"), true)
	c.Redirect(http.StatusFound, "/")
}

// finishBind links a verified identity to the current account. It never issues
// a token or overwrites the session cookie.
func (r *OAuthRegistry) finishBind(c *gin.Context, p *models.OAuthProvider, profile oauthIdentityProfile, bindUserID int64) {
	// The binding intent must still belong to the logged-in user that started it.
	if bindUserID != c.GetInt64("user_id") {
		c.Redirect(http.StatusFound, "/account/bindings?error=session")
		return
	}
	svc := r.bindService
	if svc == nil {
		svc = services.NewOAuthIdentityService()
	}
	err := svc.Bind(bindUserID, p.IdentityKey, repository.OAuthIdentityProfile{
		Subject:           profile.Subject,
		PreferredUsername: profile.PreferredUsername,
		DisplayName:       profile.DisplayName,
		Email:             profile.Email,
		Picture:           profile.Picture,
	})
	if err != nil {
		// SQL details go to the log only; the browser gets a whitelisted code.
		log.Printf("bind oauth identity (provider=%s): %v", p.IdentityKey, err)
		code := "failed"
		switch {
		case errors.Is(err, repository.ErrIdentityOwnedElsewhere):
			code = "owned"
		case errors.Is(err, repository.ErrProviderAlreadyBound):
			code = "already"
		}
		c.Redirect(http.StatusFound, "/account/bindings?error="+code)
		return
	}
	c.Redirect(http.StatusFound, "/account/bindings?bound=1")
}

// ProviderKeyForIssuer preserves the hashed-key derivation for tests and migration checks.
func ProviderKeyForIssuer(issuer, clientID string) string {
	sum := sha256.Sum256([]byte(issuer + "\x00" + clientID))
	return "oidc_" + base64.RawURLEncoding.EncodeToString(sum[:])
}
