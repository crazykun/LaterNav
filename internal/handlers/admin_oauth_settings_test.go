package handlers

import (
	"ai-later-nav/internal/web"
	"net/http"
	"net/http/httptest"
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

func TestAdminSettingsRejectsMissingCSRF(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/admin/settings", strings.NewReader("oauth_linux_redirect_url=http%3A%2F%2Flocalhost%3A8080%2Foauth%2Fcallback"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	adminSettingsRouter().ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", w.Code)
	}
}
