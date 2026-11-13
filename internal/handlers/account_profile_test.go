package handlers

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"ai-later-nav/internal/config"
	"ai-later-nav/internal/models"
	"ai-later-nav/internal/web"
	"github.com/gin-gonic/gin"
)

type accountTestIdentities struct{ listErr error }

func (stub accountTestIdentities) List(int64) ([]models.OAuthIdentity, error) {
	return []models.OAuthIdentity{}, stub.listErr
}
func (accountTestIdentities) Unbind(int64, string) error                  { return nil }
func (accountTestIdentities) IsProviderBound(int64, string) (bool, error) { return false, nil }

type accountTestUsers struct{ user *models.User }

func (u accountTestUsers) GetByID(int64) (*models.User, error) { return u.user, nil }

type accountTestProviders struct{}

func (accountTestProviders) LoginProviders() ([]*models.OAuthProvider, error) {
	return []*models.OAuthProvider{{Slug: "github", Name: "GitHub", IdentityKey: "github", Enabled: true}}, nil
}
func (accountTestProviders) GetBySlug(string) (*models.OAuthProvider, error) { return nil, nil }

func TestAccountProfilePageUsesSeparateCSRFScopes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	templates := web.BuildPageTemplates(os.DirFS("../../templates"))
	h := &AccountBindingHandler{page: &PageHandler{pageTemplates: templates}, identities: accountTestIdentities{}, providers: accountTestProviders{}, users: accountTestUsers{user: &models.User{ID: 7, Username: "alice", Nickname: "Alice", AvatarURL: "/static/img/avatar.png", PasswordHash: "hash"}}}
	r := gin.New()
	r.GET("/account/profile", func(c *gin.Context) {
		c.Set("user_id", int64(7))
		c.Set("isLoggedIn", true)
		c.Set("displayName", "Alice")
		h.AccountProfilePage(c)
	})
	req := httptest.NewRequest(http.MethodGet, "/account/profile?bound=1", nil)
	req.AddCookie(&http.Cookie{Name: "token", Value: "test-session"})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("profile page = %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "绑定成功") {
		t.Fatal("binding callback notice missing")
	}
	profile := profileCSRFForTest("test-session")
	binding := bindingCSRFForTest("test-session")
	if profile == binding {
		t.Fatal("separate CSRF domains returned same token")
	}
	for _, token := range []string{profile, binding} {
		if !strings.Contains(w.Body.String(), token) {
			t.Fatalf("form token %s missing", token)
		}
	}
	if !strings.Contains(w.Body.String(), "/static/img/avatar.png") {
		t.Fatal("saved avatar missing")
	}
}

func TestAccountProfilePageDegradesWhenBindingServiceUnavailable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &AccountBindingHandler{page: &PageHandler{pageTemplates: web.BuildPageTemplates(os.DirFS("../../templates"))}, identities: accountTestIdentities{listErr: errors.New("unavailable")}, users: accountTestUsers{user: &models.User{ID: 7, Username: "alice", PasswordHash: "!"}}}
	r := gin.New()
	r.GET("/account/profile", func(c *gin.Context) { c.Set("user_id", int64(7)); h.AccountProfilePage(c) })
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/account/profile", nil)
	req.AddCookie(&http.Cookie{Name: "token", Value: "session"})
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "第三方账号暂时无法加载") || !strings.Contains(w.Body.String(), "修改密码") {
		t.Fatalf("expected usable profile despite binding failure, status %d: %s", w.Code, w.Body.String())
	}
}

func TestLegacyBindingsPagePreservesCallbackQuery(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &AccountBindingHandler{}
	r := gin.New()
	r.GET("/account/bindings", h.AccountBindingsPage)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/account/bindings?error=owned", nil))
	if w.Code != http.StatusFound || w.Header().Get("Location") != "/account/profile?error=owned" {
		t.Fatalf("redirect: %d %q", w.Code, w.Header().Get("Location"))
	}
}

func profileCSRFForTest(token string) string { return csrfForDomain("user-profile:", token) }
func bindingCSRFForTest(token string) string { return csrfForDomain("user-binding:", token) }
func csrfForDomain(domain, token string) string {
	mac := hmac.New(sha256.New, []byte(config.AppConfig.JWT.Secret))
	mac.Write([]byte(domain + token))
	return hex.EncodeToString(mac.Sum(nil))
}
