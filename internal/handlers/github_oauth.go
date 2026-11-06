package handlers

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// GitHubConfig holds a GitHub OAuth App's endpoints and credentials.
// GitHub issues no ID token; identity comes from the userinfo API's numeric id.
type GitHubConfig struct {
	AuthorizeURL string // https://github.com/login/oauth/authorize
	TokenURL     string // https://github.com/login/oauth/access_token
	UserinfoURL  string // https://api.github.com/user
	ClientID     string
	ClientSecret string
	RedirectURL  string
}

// GitHubResult is a verified GitHub identity. Subject is the stable numeric user id.
type GitHubResult struct {
	Subject           string
	PreferredUsername string
	DisplayName       string
}

type GitHubHandler struct {
	config       GitHubConfig
	client       *http.Client
	mu           sync.Mutex
	transactions map[string]time.Time // state -> expiry (CSRF only; no nonce/PKCE needed)
}

func validGitHubConfig(cfg GitHubConfig) bool {
	return oidcURL(cfg.AuthorizeURL) && oidcURL(cfg.TokenURL) && oidcURL(cfg.UserinfoURL) && oidcRedirectURL(cfg.RedirectURL) && cfg.ClientID != "" && cfg.ClientSecret != ""
}

func NewGitHubHandler(cfg GitHubConfig) (*GitHubHandler, error) {
	if !validGitHubConfig(cfg) {
		return nil, errors.New("invalid GitHub configuration")
	}
	return &GitHubHandler{config: cfg, client: oidcHTTPClient(), transactions: make(map[string]time.Time)}, nil
}

func (h *GitHubHandler) secureCookie() bool { return strings.HasPrefix(h.config.RedirectURL, "https://") }

func (h *GitHubHandler) stateCookieName() string {
	if h.secureCookie() {
		return "__Host-gh_state"
	}
	return "gh_state"
}

func (h *GitHubHandler) Start(c *gin.Context) {
	oidcHeaders(c)
	state, err := oidcRandom()
	if err != nil {
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}
	h.mu.Lock()
	now := time.Now()
	for k, exp := range h.transactions {
		if now.After(exp) {
			delete(h.transactions, k)
		}
	}
	if len(h.transactions) >= 10000 {
		h.mu.Unlock()
		c.AbortWithStatus(http.StatusServiceUnavailable)
		return
	}
	h.transactions[state] = now.Add(10 * time.Minute)
	h.mu.Unlock()
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(h.stateCookieName(), state, 600, "/", "", h.secureCookie(), true)
	u, _ := url.Parse(h.config.AuthorizeURL)
	q := u.Query()
	q.Set("client_id", h.config.ClientID)
	q.Set("redirect_uri", h.config.RedirectURL)
	q.Set("scope", "read:user")
	q.Set("state", state)
	u.RawQuery = q.Encode()
	c.Redirect(http.StatusFound, u.String())
}

// VerifyCallback consumes the state and fetches the user profile, returning a verified identity.
func (h *GitHubHandler) VerifyCallback(c *gin.Context) (GitHubResult, error) {
	oidcHeaders(c)
	var out GitHubResult
	state, err := c.Cookie(h.stateCookieName())
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(h.stateCookieName(), "", -1, "/", "", h.secureCookie(), true)
	q := c.Request.URL.Query()
	if err != nil || len(q["state"]) != 1 || state == "" || subtle.ConstantTimeCompare([]byte(state), []byte(q.Get("state"))) != 1 {
		return out, errors.New("invalid transaction")
	}
	h.mu.Lock()
	exp, ok := h.transactions[state]
	delete(h.transactions, state)
	h.mu.Unlock()
	if !ok || !time.Now().Before(exp) || len(q["code"]) != 1 || q.Get("code") == "" || q.Get("error") != "" {
		return out, errors.New("invalid transaction")
	}
	token, err := h.exchange(c.Request.Context(), q.Get("code"))
	if err != nil {
		return out, err
	}
	return h.fetchUser(c.Request.Context(), token)
}

func (h *GitHubHandler) exchange(ctx context.Context, code string) (string, error) {
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {h.config.RedirectURL},
		"client_id":     {h.config.ClientID},
		"client_secret": {h.config.ClientSecret},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.config.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", errors.New("token request failed")
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := h.client.Do(req)
	if err != nil {
		return "", errors.New("token request failed")
	}
	defer resp.Body.Close()
	var body struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
	}
	if resp.StatusCode != http.StatusOK || json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&body) != nil || body.Error != "" || body.AccessToken == "" {
		return "", errors.New("token exchange failed")
	}
	return body.AccessToken, nil
}

func (h *GitHubHandler) fetchUser(ctx context.Context, token string) (GitHubResult, error) {
	var out GitHubResult
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, h.config.UserinfoURL, nil)
	if err != nil {
		return out, errors.New("userinfo request failed")
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := h.client.Do(req)
	if err != nil {
		return out, errors.New("userinfo request failed")
	}
	defer resp.Body.Close()
	var profile struct {
		ID    int64  `json:"id"`
		Login string `json:"login"`
		Name  string `json:"name"`
	}
	if resp.StatusCode != http.StatusOK || json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&profile) != nil || profile.ID <= 0 {
		return out, fmt.Errorf("invalid userinfo response (status %d)", resp.StatusCode)
	}
	out.Subject = strconv.FormatInt(profile.ID, 10)
	out.PreferredUsername = truncateOIDCString(profile.Login, 255)
	out.DisplayName = truncateOIDCString(profile.Name, 255)
	if out.DisplayName == "" {
		out.DisplayName = out.PreferredUsername
	}
	return out, nil
}
