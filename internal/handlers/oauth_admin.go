package handlers

import (
	"crypto/hmac"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"

	"ai-later-nav/internal/models"
	"ai-later-nav/internal/services"

	"github.com/gin-gonic/gin"
)

var providerAdminMu sync.Mutex

// providerPresets seed the "new provider" form with well-known endpoints.
var providerPresets = map[string]models.OAuthProvider{
	"google": {
		Slug: "google", Name: "Google", Type: "oidc",
		Issuer: "https://accounts.google.com", Scopes: "openid profile email",
	},
	"linuxdo": {
		Slug: "linuxdo", Name: "Linux.do", Type: "oidc",
		Issuer:       "https://linux.do",
		DiscoveryURL: "https://linux.do/forum_oauth_discovery", Scopes: "openid profile email",
	},
	"github": {
		Slug: "github", Name: "GitHub", Type: "github",
		AuthorizeURL: "https://github.com/login/oauth/authorize",
		TokenURL:     "https://github.com/login/oauth/access_token",
		UserinfoURL:  "https://api.github.com/user",
	},
}

func providerTypeEndpointsValid(p *models.OAuthProvider) bool {
	switch p.Type {
	case "oidc":
		if !oidcAdminURL(p.Issuer, true) {
			return false
		}
		if p.DiscoveryURL != "" && !oidcAdminURL(p.DiscoveryURL, false) {
			return false
		}
		return true
	case "github":
		return oidcAdminURL(p.AuthorizeURL, false) && oidcAdminURL(p.TokenURL, false) && oidcAdminURL(p.UserinfoURL, false)
	}
	return false
}

func providerRedirectValid(p *models.OAuthProvider) bool {
	if !oidcAdminURL(p.RedirectURL, false) {
		return false
	}
	u, err := url.Parse(p.RedirectURL)
	return err == nil && u.Path == "/oauth/"+p.Slug+"/callback"
}

// providerFromForm builds and validates a provider from the admin submission.
// Identity-affecting fields (slug/type/endpoints/client ID) are immutable once the
// row exists: changing them would silently fork or hijack bound accounts, so the
// stored values win regardless of what the form posts.
func providerFromForm(form url.Values, old *models.OAuthProvider) (*models.OAuthProvider, error) {
	p := &models.OAuthProvider{
		Slug:         strings.TrimSpace(form.Get("slug")),
		Name:         strings.TrimSpace(form.Get("name")),
		Type:         form.Get("type"),
		Issuer:       strings.TrimSpace(form.Get("issuer")),
		DiscoveryURL: strings.TrimSpace(form.Get("discovery_url")),
		AuthorizeURL: strings.TrimSpace(form.Get("authorize_url")),
		TokenURL:     strings.TrimSpace(form.Get("token_url")),
		UserinfoURL:  strings.TrimSpace(form.Get("userinfo_url")),
		ClientID:     strings.TrimSpace(form.Get("client_id")),
		ClientSecret: form.Get("client_secret"),
		RedirectURL:  strings.TrimSpace(form.Get("redirect_url")),
		Scopes:       strings.TrimSpace(form.Get("scopes")),
		Enabled:      form.Get("enabled") == "true",
	}
	p.Sort, _ = strconv.Atoi(form.Get("sort"))
	if old != nil {
		p.Slug = old.Slug
		p.Type = old.Type
		p.Issuer = old.Issuer
		p.AuthorizeURL = old.AuthorizeURL
		p.TokenURL = old.TokenURL
		p.UserinfoURL = old.UserinfoURL
		p.ClientID = old.ClientID
		if p.ClientSecret == "" {
			p.ClientSecret = old.ClientSecret
		}
	}
	if !services.ValidSlug(p.Slug) {
		return nil, errText("slug 只能包含小写字母、数字和连字符，且不能是 login/callback。")
	}
	if p.Name == "" {
		return nil, errText("显示名不能为空。")
	}
	if !providerTypeEndpointsValid(p) {
		return nil, errText("端点必须是有效的 HTTPS URL，不得包含用户信息、查询参数或片段。")
	}
	if !providerRedirectValid(p) {
		return nil, errText("回调地址必须是 HTTPS（本地允许 localhost HTTP），路径为 /oauth/" + p.Slug + "/callback。")
	}
	if p.Enabled && (p.ClientID == "" || p.ClientSecret == "") {
		return nil, errText("启用前必须填写 Client ID 和 Client Secret。")
	}
	return p, nil
}

type adminError struct{ msg string }

func errText(s string) error        { return &adminError{s} }
func (e *adminError) Error() string { return e.msg }

func renderProviderList(c *gin.Context, status int, key, message string) {
	list, err := services.GetOAuthProviderService().List()
	if err != nil {
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}
	data := getCommonAdminData(c, "oidc", "三方登录", "管理所有第三方登录方式。")
	data["providers"] = list
	data["csrfToken"] = settingsCSRF(c)
	if key != "" {
		data[key] = message
	}
	c.Header("Cache-Control", "no-store")
	c.HTML(status, "admin-providers.html", data)
}

// AdminProviderList renders all configured providers.
func (h *AdminHandler) AdminProviderList(c *gin.Context) {
	renderProviderList(c, http.StatusOK, "", "")
}

func renderProviderForm(c *gin.Context, status int, p *models.OAuthProvider, hasSecret, isNew bool, key, message string) {
	data := getCommonAdminData(c, "oidc", "三方登录", "配置登录提供方。")
	data["provider"] = p
	data["hasSecret"] = hasSecret
	data["isNew"] = isNew
	data["csrfToken"] = settingsCSRF(c)
	if key != "" {
		data[key] = message
	}
	c.Header("Cache-Control", "no-store")
	c.HTML(status, "admin-provider-edit.html", data)
}

// AdminProviderNew renders a blank or preset-seeded creation form.
func (h *AdminHandler) AdminProviderNew(c *gin.Context) {
	var p models.OAuthProvider
	if preset, ok := providerPresets[c.Query("preset")]; ok {
		p = preset
	}
	p.RedirectURL = redirectBase(c) + "/oauth/" + p.Slug + "/callback"
	renderProviderForm(c, http.StatusOK, &p, false, true, "", "")
}

// AdminProviderEdit renders the edit form for an existing provider.
func (h *AdminHandler) AdminProviderEdit(c *gin.Context) {
	p, err := services.GetOAuthProviderService().GetBySlug(c.Param("slug"))
	if err != nil {
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}
	if p == nil {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	hasSecret := p.ClientSecret != ""
	p.ClientSecret = ""
	renderProviderForm(c, http.StatusOK, p, hasSecret, false, "", "")
}

// AdminProviderSave validates and persists a provider (create or update).
func (h *AdminHandler) AdminProviderSave(c *gin.Context) {
	if !validSettingsCSRF(c) {
		c.AbortWithStatus(http.StatusForbidden)
		return
	}
	if err := c.Request.ParseForm(); err != nil {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}
	providerAdminMu.Lock()
	defer providerAdminMu.Unlock()
	ps := services.GetOAuthProviderService()
	old, err := ps.GetBySlug(c.Param("slug"))
	if err != nil {
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}
	p, err := providerFromForm(c.Request.PostForm, old)
	if err != nil {
		renderProviderForm(c, http.StatusBadRequest, p, p.ClientSecret != "", old == nil, "error", err.Error())
		return
	}
	if err := ps.Save(p, c.PostForm("client_secret") != ""); err != nil {
		renderProviderForm(c, http.StatusBadRequest, p, p.ClientSecret != "", old == nil, "error", "保存失败："+err.Error())
		return
	}
	c.Redirect(http.StatusSeeOther, "/admin/oidc")
}

// AdminProviderDelete removes a provider with no bound identities.
func (h *AdminHandler) AdminProviderDelete(c *gin.Context) {
	if !validSettingsCSRF(c) {
		c.AbortWithStatus(http.StatusForbidden)
		return
	}
	if err := services.GetOAuthProviderService().Delete(c.Param("slug")); err != nil {
		if err == services.ErrProviderInUse {
			renderProviderList(c, http.StatusBadRequest, "error", "该登录方式已有账户绑定，不能删除；可改为停用。")
			return
		}
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}
	c.Redirect(http.StatusSeeOther, "/admin/oidc")
}

// validSettingsCSRF checks the admin form token.
func validSettingsCSRF(c *gin.Context) bool {
	expected := settingsCSRF(c)
	return expected != "" && hmac.Equal([]byte(expected), []byte(c.PostForm("csrf_token")))
}

// redirectBase derives the site's external base URL from the request;
// localhost development uses the request host.
func redirectBase(c *gin.Context) string {
	scheme := "https"
	if c.Request.TLS == nil {
		scheme = "http"
	}
	return scheme + "://" + c.Request.Host
}
