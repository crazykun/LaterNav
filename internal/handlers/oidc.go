package handlers

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"ai-later-nav/internal/database"
	"ai-later-nav/internal/middleware"
	"ai-later-nav/internal/models"
	"ai-later-nav/internal/services"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/gin-gonic/gin"
	"golang.org/x/oauth2"
)

// The issuer must identify the same Linux.sb identity namespace as existing accounts.
type OIDCConfig struct{ Issuer, DiscoveryURL, ClientID, ClientSecret, RedirectURL string }

// OIDCResult contains only verified ID-token claims. AuthTime is zero when absent.
type OIDCResult struct {
	Issuer, Subject   string
	PreferredUsername string
	DisplayName       string
	AuthTime          time.Time
	Reauthenticated   bool
}
type oidcTransaction struct {
	nonce, verifier  string
	expires          time.Time
	maxAge           *time.Duration
	reauthentication bool
}
type OIDCHandler struct {
	config       OIDCConfig
	provider     *oidc.Provider
	oauth        oauth2.Config
	client       *http.Client
	mu           sync.Mutex
	transactions map[string]oidcTransaction
}

func oidcPublicIP(ip netip.Addr) bool {
	ip = ip.Unmap()
	if !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
		return false
	}
	for _, p := range []string{"0.0.0.0/8", "100.64.0.0/10", "192.0.0.0/24", "192.0.2.0/24", "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "224.0.0.0/4", "240.0.0.0/4", "2001:db8::/32", "2001::/23", "64:ff9b::/96", "64:ff9b:1::/48", "2002::/16"} {
		if netip.MustParsePrefix(p).Contains(ip) {
			return false
		}
	}
	return true
}
func oidcURL(s string) bool {
	u, e := url.Parse(s)
	return e == nil && u.Scheme == "https" && u.Hostname() != "" && u.User == nil && u.Fragment == "" && u.RawQuery == "" && u.Opaque == ""
}
func oidcRedirectURL(s string) bool {
	u, e := url.Parse(s)
	return e == nil && u.Hostname() != "" && u.User == nil && u.Fragment == "" && u.RawQuery == "" && u.Opaque == "" && (u.Scheme == "https" || (u.Scheme == "http" && u.Hostname() == "localhost"))
}

func oidcHTTPClient() *http.Client {
	d := &net.Dialer{Timeout: 5 * time.Second}
	tr := &http.Transport{Proxy: nil, TLSHandshakeTimeout: 5 * time.Second, ResponseHeaderTimeout: 10 * time.Second, DisableKeepAlives: true}
	tr.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, errors.New("invalid address")
		}
		ips, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
		if err != nil || len(ips) == 0 {
			return nil, errors.New("DNS resolution failed")
		}
		for _, ip := range ips {
			if !oidcPublicIP(ip) {
				return nil, errors.New("non-public address")
			}
		}
		return d.DialContext(ctx, network, net.JoinHostPort(ips[0].String(), port))
	}
	return &http.Client{Transport: oidcLimitTransport{tr}, Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

type oidcLimitTransport struct{ base http.RoundTripper }

func (t oidcLimitTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if !oidcURL(r.URL.String()) {
		return nil, errors.New("invalid endpoint")
	}
	resp, err := t.base.RoundTrip(r)
	if err == nil {
		resp.Body = struct {
			io.Reader
			io.Closer
		}{io.LimitReader(resp.Body, 1<<20), resp.Body}
	}
	return resp, err
}

// NewOIDCHandler performs discovery and constructs a handler. Use a fixed issuer per identity namespace.
func NewOIDCHandler(ctx context.Context, cfg OIDCConfig) (*OIDCHandler, error) {
	return newOIDCHandlerWithScopes(ctx, cfg, nil)
}

func newOIDCHandlerWithScopes(ctx context.Context, cfg OIDCConfig, scopes []string) (*OIDCHandler, error) {
	if !oidcURL(cfg.Issuer) || !oidcRedirectURL(cfg.RedirectURL) || cfg.ClientID == "" || cfg.ClientSecret == "" {
		return nil, errors.New("invalid OIDC configuration")
	}
	if len(scopes) == 0 {
		scopes = []string{oidc.ScopeOpenID, "profile", "email"}
	}
	client := oidcHTTPClient()
	ctx = oidc.ClientContext(ctx, client)
	var p *oidc.Provider
	var err error
	var jwksURL string
	if cfg.DiscoveryURL == "" {
		p, err = oidc.NewProvider(ctx, cfg.Issuer)
	} else {
		if !oidcURL(cfg.DiscoveryURL) {
			return nil, errors.New("invalid discovery URL")
		}
		resp, fetchErr := client.Get(cfg.DiscoveryURL)
		if fetchErr != nil {
			return nil, errors.New("OIDC discovery failed")
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, errors.New("OIDC discovery failed")
		}
		var metadata oidc.ProviderConfig
		if json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&metadata) != nil || metadata.IssuerURL != cfg.Issuer || !oidcURL(metadata.AuthURL) || !oidcURL(metadata.TokenURL) || !oidcURL(metadata.JWKSURL) {
			return nil, errors.New("invalid OIDC metadata")
		}
		jwksURL = metadata.JWKSURL
		p = metadata.NewProvider(ctx)
	}
	if err != nil {
		return nil, errors.New("OIDC discovery failed")
	}
	ep := p.Endpoint()
	if cfg.DiscoveryURL == "" {
		var metadata struct {
			JWKS string `json:"jwks_uri"`
		}
		if p.Claims(&metadata) != nil {
			return nil, errors.New("invalid OIDC metadata")
		}
		jwksURL = metadata.JWKS
	}
	if !oidcURL(ep.AuthURL) || !oidcURL(ep.TokenURL) || !oidcURL(jwksURL) {
		return nil, errors.New("invalid OIDC metadata")
	}
	return &OIDCHandler{config: cfg, provider: p, client: client, oauth: oauth2.Config{ClientID: cfg.ClientID, ClientSecret: cfg.ClientSecret, RedirectURL: cfg.RedirectURL, Endpoint: ep, Scopes: scopes}, transactions: make(map[string]oidcTransaction)}, nil
}
func (h *OIDCHandler) stateCookieName() string {
	if strings.HasPrefix(h.config.RedirectURL, "https://") {
		return "__Host-oidc_state"
	}
	return "oidc_state"
}
func (h *OIDCHandler) secureCookie() bool { return strings.HasPrefix(h.config.RedirectURL, "https://") }
func oidcRandom() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
func oidcHeaders(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.Header("Referrer-Policy", "no-referrer")
}

// Start initiates normal login. For sensitive reauthentication, use StartReauthentication.
func (h *OIDCHandler) Start(c *gin.Context) { h.start(c, nil) }

// StartReauthentication requires the provider to freshly authenticate and asserts auth_time on callback.
// A caller must separately bind the verified subject to its current account and intended operation.
func (h *OIDCHandler) StartReauthentication(c *gin.Context, maxAge time.Duration) {
	if maxAge < 0 || maxAge > 24*time.Hour || maxAge%time.Second != 0 {
		c.AbortWithStatus(400)
		return
	}
	h.start(c, &maxAge)
}
func (h *OIDCHandler) start(c *gin.Context, maxAge *time.Duration) {
	oidcHeaders(c)
	state, e := oidcRandom()
	if e != nil {
		c.AbortWithStatus(500)
		return
	}
	nonce, e := oidcRandom()
	if e != nil {
		c.AbortWithStatus(500)
		return
	}
	verifier, e := oidcRandom()
	if e != nil {
		c.AbortWithStatus(500)
		return
	}
	h.mu.Lock()
	now := time.Now()
	for k, v := range h.transactions {
		if now.After(v.expires) {
			delete(h.transactions, k)
		}
	}
	if len(h.transactions) >= 10000 {
		h.mu.Unlock()
		c.AbortWithStatus(503)
		return
	}
	h.transactions[state] = oidcTransaction{nonce: nonce, verifier: verifier, expires: now.Add(10 * time.Minute), maxAge: maxAge}
	h.mu.Unlock()
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(h.stateCookieName(), state, 600, "/", "", h.secureCookie(), true)
	sum := sha256.Sum256([]byte(verifier))
	opts := []oauth2.AuthCodeOption{oidc.Nonce(nonce), oauth2.SetAuthURLParam("code_challenge", base64.RawURLEncoding.EncodeToString(sum[:])), oauth2.SetAuthURLParam("code_challenge_method", "S256")}
	if maxAge != nil {
		opts = append(opts, oauth2.SetAuthURLParam("max_age", strconv.FormatInt(int64(maxAge.Seconds()), 10)), oauth2.SetAuthURLParam("prompt", "login"))
	}
	c.Redirect(302, h.oauth.AuthCodeURL(state, opts...))
}

// VerifyCallback consumes the browser-bound transaction and returns verified claims, never tokens.
func (h *OIDCHandler) VerifyCallback(c *gin.Context) (OIDCResult, error) {
	oidcHeaders(c)
	var out OIDCResult
	state, err := c.Cookie(h.stateCookieName())
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(h.stateCookieName(), "", -1, "/", "", h.secureCookie(), true)
	q := c.Request.URL.Query()
	if err != nil || len(q["state"]) != 1 || state == "" || subtle.ConstantTimeCompare([]byte(state), []byte(q.Get("state"))) != 1 {
		return out, errors.New("invalid transaction")
	}
	h.mu.Lock()
	tx, ok := h.transactions[state]
	delete(h.transactions, state)
	h.mu.Unlock()
	if !ok || !time.Now().Before(tx.expires) || len(q["code"]) != 1 || q.Get("code") == "" || q.Get("error") != "" {
		return out, errors.New("invalid transaction")
	}
	ctx := oidc.ClientContext(c.Request.Context(), h.client)
	tok, err := h.oauth.Exchange(ctx, q.Get("code"), oauth2.SetAuthURLParam("code_verifier", tx.verifier))
	if err != nil {
		return out, errors.New("OIDC exchange failed")
	}
	raw, ok := tok.Extra("id_token").(string)
	if !ok || raw == "" {
		return out, errors.New("missing ID token")
	}
	id, err := h.provider.Verifier(&oidc.Config{ClientID: h.config.ClientID}).Verify(ctx, raw)
	if err != nil || id.Nonce == "" || subtle.ConstantTimeCompare([]byte(id.Nonce), []byte(tx.nonce)) != 1 || id.Subject == "" || len(id.Subject) > 255 {
		return out, errors.New("invalid ID token")
	}
	var claims struct {
		AuthTime          *int64 `json:"auth_time"`
		AuthorizedParty   string `json:"azp"`
		PreferredUsername string `json:"preferred_username"`
		DisplayName       string `json:"name"`
	}
	if id.Claims(&claims) != nil || (len(id.Audience) > 1 && claims.AuthorizedParty != h.config.ClientID) || (claims.AuthorizedParty != "" && claims.AuthorizedParty != h.config.ClientID) {
		return out, errors.New("invalid ID token claims")
	}
	out = OIDCResult{Issuer: id.Issuer, Subject: id.Subject, PreferredUsername: claims.PreferredUsername, DisplayName: claims.DisplayName}
	if claims.AuthTime != nil {
		out.AuthTime = time.Unix(*claims.AuthTime, 0)
		if out.AuthTime.After(time.Now().Add(time.Minute)) {
			return OIDCResult{}, errors.New("invalid authentication time")
		}
	}
	if tx.maxAge != nil && (claims.AuthTime == nil || time.Since(out.AuthTime) > *tx.maxAge+time.Minute) {
		return OIDCResult{}, errors.New("authentication too old")
	}
	out.Reauthenticated = tx.maxAge != nil
	return out, nil
}

func databaseOIDCUser(provider, subject, preferredUsername, displayName string) (*models.User, error) {
	db := database.DB
	if db == nil {
		return nil, errors.New("database unavailable")
	}
	find := func() (*models.User, error) {
		var u models.User
		err := db.QueryRow(`SELECT u.id, u.username, u.nickname, u.role, u.status FROM oauth_identities o JOIN users u ON u.id=o.user_id WHERE o.provider=? AND o.subject=?`, provider, subject).Scan(&u.ID, &u.Username, &u.Nickname, &u.Role, &u.Status)
		return &u, err
	}
	if user, err := find(); err == nil {
		return user, nil
	} else if err != sql.ErrNoRows {
		return nil, err
	}
	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	random, err := oidcRandom()
	if err != nil {
		return nil, err
	}
	username := "oidc_" + random[:24]
	result, err := tx.Exec(`INSERT INTO users (username, nickname, password_hash, role) VALUES (?, ?, ?, 'user')`, username, truncateOIDCString(displayName, 100), "!")
	if err != nil {
		return nil, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}
	if _, err = tx.Exec(`INSERT INTO oauth_identities (provider, subject, user_id, preferred_username, display_name) VALUES (?, ?, ?, ?, ?)`, provider, subject, id, truncateOIDCString(preferredUsername, 255), truncateOIDCString(displayName, 255)); err != nil {
		tx.Rollback()
		if existing, e := find(); e == nil {
			return existing, nil
		}
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return &models.User{ID: id, Username: username, Role: "user", Status: "active", Nickname: truncateOIDCString(displayName, 100)}, nil
}

func truncateOIDCString(s string, max int) string {
	r := []rune(s)
	if len(r) > max {
		return string(r[:max])
	}
	return s
}

// Callback is superseded by OAuthRegistry.Callback, which derives the identity
// key from the provider row so multiple providers cannot collide.
func (h *OIDCHandler) Callback(c *gin.Context) {
	result, err := h.VerifyCallback(c)
	if err != nil {
		c.String(http.StatusBadRequest, "三方登录失败，请重新登录")
		return
	}
	if result.Reauthenticated {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}
	provider := services.IdentityKey("oidc", result.Issuer, "", h.config.ClientID)
	user, err := databaseOIDCUser(provider, result.Subject, result.PreferredUsername, result.DisplayName)
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
	c.SetCookie("token", token, 86400*7, "/", "", h.secureCookie(), true)
	c.Redirect(http.StatusFound, "/")
}
