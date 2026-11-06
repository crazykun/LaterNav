package handlers

import (
	"ai-later-nav/internal/web"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func adminSettingsRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.SetHTMLTemplate(web.BuildSharedTemplates(os.DirFS("../../templates")))
	h := &AdminHandler{}
	r.GET("/admin/settings", h.AdminSettingsForm)
	r.POST("/admin/settings", h.AdminSettingsSave)
	return r
}

func TestAdminOAuthSettingsDefaults(t *testing.T) {
	w := httptest.NewRecorder()
	adminSettingsRouter().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/admin/settings", nil))
	body := w.Body.String()
	for _, value := range []string{"oauth_linux_enabled", "foa_3_LbaAtj2WGHnKmNDgUw55pE", "http://localhost:8080/oauth/callback", `type="password"`} {
		if !strings.Contains(body, value) {
			t.Errorf("page missing %q", value)
		}
	}
}

func TestAdminSettingsRejectsMissingCSRF(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/admin/settings", strings.NewReader("oauth_linux_redirect_url=http%3A%2F%2Flocalhost%3A8080%2Foauth%2Fcallback"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	adminSettingsRouter().ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", w.Code)
	}
}

func TestAdminOAuthRejectsInvalidRedirect(t *testing.T) {
	for _, redirect := range []string{"http://example.com/oauth/callback", "http://localhost.evil/oauth/callback", "https://example.com/other", "https://example.com/oauth/callback?foo=bar", "https://user:pass@example.com/oauth/callback"} {
		t.Run(redirect, func(t *testing.T) {
			form := url.Values{"oauth_linux_redirect_url": {redirect}}
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx.Request = httptest.NewRequest(http.MethodGet, "/", nil)
			ctx.Request.AddCookie(&http.Cookie{Name: "token", Value: "test-token"})
			form.Set("csrf_token", settingsCSRF(ctx))
			req := httptest.NewRequest(http.MethodPost, "/admin/settings", strings.NewReader(form.Encode()))
			req.AddCookie(&http.Cookie{Name: "token", Value: "test-token"})
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			w := httptest.NewRecorder()
			adminSettingsRouter().ServeHTTP(w, req)
			if !strings.Contains(w.Body.String(), "回调地址必须使用 HTTPS") {
				t.Errorf("invalid callback was not rejected: %d", w.Code)
			}
		})
	}
}
