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

func TestProviderFromFormValidation(t *testing.T) {
	valid := url.Values{
		"slug":         {"google"},
		"name":         {"Google"},
		"type":         {"oidc"},
		"issuer":       {"https://accounts.google.com"},
		"redirect_url": {"https://example.com/oauth/google/callback"},
		"client_id":    {"id"},
		"client_secret": {"secret"},
		"enabled":      {"true"},
	}
	if _, err := providerFromForm(valid, nil); err != nil {
		t.Fatalf("valid form rejected: %v", err)
	}
	for _, tc := range []struct{ field, value string }{
		{"slug", "Login"},
		{"slug", "callback"},
		{"slug", "bad slug"},
		{"issuer", "http://example.com"},
		{"issuer", "https://example.com/"},
		{"issuer", "https://user@example.com"},
		{"redirect_url", "http://example.com/oauth/google/callback"},
		{"redirect_url", "https://example.com/wrong/path"},
		{"redirect_url", "https://example.com/oauth/other/callback"},
	} {
		v := url.Values{}
		for k, x := range valid {
			v[k] = x
		}
		v.Set(tc.field, tc.value)
		if _, err := providerFromForm(v, nil); err == nil {
			t.Errorf("accepted %s=%s", tc.field, tc.value)
		}
	}
}

func TestProviderFromFormEnabledRequiresSecret(t *testing.T) {
	form := url.Values{
		"slug": {"google"}, "name": {"Google"}, "type": {"oidc"},
		"issuer": {"https://accounts.google.com"}, "redirect_url": {"https://example.com/oauth/google/callback"},
		"client_id": {"id"}, "enabled": {"true"},
	}
	if _, err := providerFromForm(form, nil); err == nil {
		t.Fatal("enabled without secret accepted")
	}
}

func TestProviderFromFormGitHubEndpoints(t *testing.T) {
	form := url.Values{
		"slug": {"github"}, "name": {"GitHub"}, "type": {"github"},
		"authorize_url": {"https://github.com/login/oauth/authorize"},
		"token_url":     {"https://github.com/login/oauth/access_token"},
		"userinfo_url":  {"https://api.github.com/user"},
		"redirect_url":  {"https://example.com/oauth/github/callback"},
		"client_id":     {"id"}, "client_secret": {"secret"}, "enabled": {"true"},
	}
	p, err := providerFromForm(form, nil)
	if err != nil {
		t.Fatalf("github form rejected: %v", err)
	}
	if p.Type != "github" || p.Issuer != "" {
		t.Fatalf("unexpected provider: %+v", p)
	}
}

func TestProviderAdminCSRFAndListTemplate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.SetHTMLTemplate(web.BuildSharedTemplates(os.DirFS("../../templates")))
	h := &AdminHandler{}
	r.POST("/admin/oidc/:slug", h.AdminProviderSave)
	r.POST("/admin/oidc/:slug/delete", h.AdminProviderDelete)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/admin/oidc/google", strings.NewReader("csrf_token=forged")))
	if w.Code != http.StatusForbidden {
		t.Fatalf("forged CSRF save status = %d", w.Code)
	}
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/admin/oidc/google/delete", strings.NewReader("csrf_token=forged")))
	if w.Code != http.StatusForbidden {
		t.Fatalf("forged CSRF delete status = %d", w.Code)
	}
}
