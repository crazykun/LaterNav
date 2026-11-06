package handlers

import (
	"ai-later-nav/internal/web"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestOIDCValidateConfiguration(t *testing.T) {
	valid := url.Values{"oidc_enabled": {"true"}, "oidc_name": {"Linux.sb"}, "oidc_issuer": {"https://linux.sb"}, "oidc_client_id": {"client"}, "oidc_redirect_url": {"https://example.com/oauth/callback"}}
	for _, tc := range []struct{ field, value string }{{"oidc_issuer", "http://example.com"}, {"oidc_issuer", "https://user@example.com"}, {"oidc_issuer", "https://example.com?x=1"}, {"oidc_redirect_url", "http://example.com/callback"}, {"oidc_redirect_url", "https://example.com/callback#x"}} {
		v := url.Values{}
		for k, x := range valid {
			v[k] = x
		}
		v.Set(tc.field, tc.value)
		if _, err := oidcUpdates(v, map[string]string{"oidc_client_secret": "secret"}, func() (bool, error) { return false, nil }); err == nil {
			t.Errorf("accepted %s=%s", tc.field, tc.value)
		}
	}
	if _, err := oidcUpdates(valid, map[string]string{"oidc_client_secret": "secret"}, func() (bool, error) { return false, nil }); err != nil {
		t.Fatal(err)
	}
}

func TestOIDCIdentityBindingAndSecret(t *testing.T) {
	// Locked fields render read-only; a tampered submission must be ignored,
	// not rejected, so the rest of the form still saves.
	form := url.Values{"oidc_enabled": {"true"}, "oidc_name": {"name"}, "oidc_issuer": {"https://evil.example"}, "oidc_client_id": {"new"}, "oidc_redirect_url": {"https://app.example/oauth/callback"}}
	old := map[string]string{"oidc_issuer": "https://id.example", "oidc_client_id": "old", "oidc_client_secret": "never reveal"}
	updates, err := oidcUpdates(form, old, func() (bool, error) { return false, nil })
	if err != nil || updates["oidc_issuer"] != "https://id.example" || updates["oidc_client_id"] != "old" {
		t.Fatalf("locked fields not enforced: %v %v", updates, err)
	}
	form.Set("oidc_client_id", "old")
	form.Set("oidc_issuer", "https://id.example")
	updates, err = oidcUpdates(form, old, func() (bool, error) { return false, nil })
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := updates["oidc_client_secret"]; ok {
		t.Fatal("blank secret overwrites existing secret")
	}
	for _, check := range []func() (bool, error){func() (bool, error) { return true, nil }, func() (bool, error) { return false, errors.New("db down") }} {
		if _, err := oidcUpdates(form, map[string]string{}, check); err == nil {
			t.Fatal("unverified existing identities accepted")
		}
	}
}

func TestOIDCAdminCSRFAndTemplate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.SetHTMLTemplate(web.BuildSharedTemplates(os.DirFS("../../templates")))
	h := &AdminHandler{}
	r.GET("/admin/oidc", h.AdminOIDCForm)
	r.POST("/admin/oidc", h.AdminOIDCSave)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/admin/oidc", nil))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "oidc_client_secret") || !strings.Contains(w.Body.String(), "错误绑定") {
		t.Fatalf("page: %d %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/admin/oidc", strings.NewReader("csrf_token=forged")))
	if w.Code != http.StatusForbidden {
		t.Fatalf("forged CSRF status = %d", w.Code)
	}
}

func TestOIDCDiscoveryURLValidation(t *testing.T) {
	form := url.Values{"oidc_discovery_url": {"http://evil.example/discovery"}}
	if _, err := oidcUpdates(form, nil, func() (bool, error) { return false, nil }); err == nil {
		t.Fatal("insecure discovery URL accepted")
	}
	form.Set("oidc_discovery_url", "https://linux.sb/forum_oauth_discovery")
	values, err := oidcUpdates(form, nil, func() (bool, error) { return false, nil })
	if err != nil || values["oidc_discovery_url"] != form.Get("oidc_discovery_url") {
		t.Fatalf("discovery not saved: %v %v", values, err)
	}
}
