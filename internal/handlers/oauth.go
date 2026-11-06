package handlers

import (
	"ai-later-nav/internal/database"
	"ai-later-nav/internal/middleware"
	"ai-later-nav/internal/models"
	"ai-later-nav/internal/services"
	"crypto/rand"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

func oauthEnabled() bool {
	s := services.GetSettingService()
	return s.GetSetting("oauth_linux_enabled") == "true" && s.GetSetting("oauth_linux_client_id") != "" && s.GetSetting("oauth_linux_client_secret") != "" && s.GetSetting("oauth_linux_redirect_url") != ""
}

type OAuthHandler struct {
	ClientID, ClientSecret, RedirectURL, AuthorizeURL, TokenURL, UserInfoURL string
}

func (h *OAuthHandler) configured() bool {
	return h.ClientID != "" && h.ClientSecret != "" && h.RedirectURL != ""
}
func (h *OAuthHandler) secure() bool { return strings.HasPrefix(h.RedirectURL, "https://") }
func (h *OAuthHandler) Start(c *gin.Context) {
	if !h.configured() {
		c.String(http.StatusServiceUnavailable, "第三方登录尚未配置")
		return
	}
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}
	state := base64.RawURLEncoding.EncodeToString(b)
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie("oauth_state", state, 600, "/oauth", "", h.secure(), true)
	u, err := url.Parse(h.AuthorizeURL)
	if err != nil {
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}
	q := u.Query()
	q.Set("response_type", "code")
	q.Set("client_id", h.ClientID)
	q.Set("redirect_uri", h.RedirectURL)
	q.Set("scope", "openid profile email")
	q.Set("state", state)
	u.RawQuery = q.Encode()
	c.Redirect(http.StatusFound, u.String())
}

type oauthProfile struct {
	Sub      string `json:"sub"`
	Username string `json:"preferred_username"`
	Name     string `json:"name"`
	Picture  string `json:"picture"`
	Email    string `json:"email"`
}

func (h *OAuthHandler) Callback(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.Header("Referrer-Policy", "no-referrer")
	state, err := c.Cookie("oauth_state")
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie("oauth_state", "", -1, "/oauth", "", h.secure(), true)
	supplied := c.Query("state")
	if err != nil || state == "" || supplied == "" || subtle.ConstantTimeCompare([]byte(state), []byte(supplied)) != 1 {
		c.String(http.StatusBadRequest, "登录状态无效，请重新登录")
		return
	}
	if !h.configured() || c.Query("error") != "" || c.Query("code") == "" {
		c.String(http.StatusBadRequest, "授权失败，请重新登录")
		return
	}
	profile, err := h.fetchProfile(c, c.Query("code"))
	if err != nil {
		log.Printf("OAuth provider request failed: %v", err)
		c.String(http.StatusBadGateway, "第三方登录失败，请稍后重试")
		return
	}
	user, err := databaseOAuthUser(profile)
	if err != nil {
		log.Printf("OAuth account lookup failed: %v", err)
		c.String(http.StatusInternalServerError, "登录失败，请稍后重试")
		return
	}
	if user.Status != "active" {
		c.String(http.StatusForbidden, "账号已被禁用")
		return
	}
	token, err := middleware.GenerateToken(user)
	if err != nil {
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie("token", token, 86400*7, "/", "", h.secure(), true)
	c.Redirect(http.StatusFound, "/")
}

func (h *OAuthHandler) fetchProfile(c *gin.Context, code string) (oauthProfile, error) {
	var p oauthProfile
	client := &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	form := url.Values{"grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {h.RedirectURL}}
	req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodPost, h.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return p, fmt.Errorf("construct token request")
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(h.ClientID, h.ClientSecret)
	resp, err := client.Do(req)
	if err != nil {
		return p, fmt.Errorf("token request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return p, fmt.Errorf("token endpoint returned %d", resp.StatusCode)
	}
	var token struct {
		AccessToken string `json:"access_token"`
	}
	if json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&token) != nil || token.AccessToken == "" {
		return p, fmt.Errorf("invalid token response")
	}
	req, err = http.NewRequestWithContext(c.Request.Context(), http.MethodGet, h.UserInfoURL, nil)
	if err != nil {
		return p, fmt.Errorf("construct userinfo request")
	}
	req.Header.Set("Authorization", "Bearer "+token.AccessToken)
	resp, err = client.Do(req)
	if err != nil {
		return p, fmt.Errorf("userinfo request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return p, fmt.Errorf("userinfo endpoint returned %d", resp.StatusCode)
	}
	if json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&p) != nil || p.Sub == "" || len(p.Sub) > 255 {
		return p, fmt.Errorf("invalid userinfo response")
	}
	return p, nil
}

// databaseOAuthUser creates an isolated account and identity atomically. Profile email is never used for account matching.
func databaseOAuthUser(p oauthProfile) (*models.User, error) {
	db := database.DB
	if db == nil {
		return nil, fmt.Errorf("database unavailable")
	}
	find := func() (*models.User, error) {
		var u models.User
		err := db.QueryRow(`SELECT u.id, u.username, u.role, u.status FROM oauth_identities o JOIN users u ON u.id=o.user_id WHERE o.provider=? AND o.subject=?`, "linux.sb", p.Sub).Scan(&u.ID, &u.Username, &u.Role, &u.Status)
		return &u, err
	}
	user, err := find()
	if err == nil {
		return user, nil
	}
	if err != sql.ErrNoRows {
		return nil, err
	}
	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	random := make([]byte, 18)
	if _, err = rand.Read(random); err != nil {
		return nil, err
	}
	username := "linux_" + base64.RawURLEncoding.EncodeToString(random)
	result, err := tx.Exec(`INSERT INTO users (username, password_hash, role) VALUES (?, ?, 'user')`, username, "!")
	if err != nil {
		return nil, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}
	_, err = tx.Exec(`INSERT INTO oauth_identities (provider, subject, user_id, preferred_username, display_name, picture, email) VALUES (?, ?, ?, ?, ?, ?, ?)`, "linux.sb", p.Sub, id, truncate(p.Username, 255), truncate(p.Name, 255), truncate(p.Picture, 2048), truncate(p.Email, 255))
	if err != nil {
		tx.Rollback()
		existing, e := find()
		if e == nil {
			return existing, nil
		}
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &models.User{ID: id, Username: username, Role: "user", Status: "active"}, nil
}
func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n])
	}
	return s
}
