package handlers

import (
	"ai-later-nav/internal/database"
	"ai-later-nav/internal/services"
	"crypto/hmac"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
)

var oidcAdminMu sync.Mutex

// oidcUpdates validates the whole submission before any settings are persisted.
func oidcUpdates(form url.Values, old map[string]string, identitiesExist func() (bool, error)) (map[string]string, error) {
	enabled := form.Get("oidc_enabled") == "true"
	name := strings.TrimSpace(form.Get("oidc_name"))
	issuer := strings.TrimSpace(form.Get("oidc_issuer"))
	client := strings.TrimSpace(form.Get("oidc_client_id"))
	redirect := strings.TrimSpace(form.Get("oidc_redirect_url"))
	discovery := strings.TrimSpace(form.Get("oidc_discovery_url"))
	secret := form.Get("oidc_client_secret")
	// Locked fields are rendered read-only; ignore the submitted value entirely
	// once configured, so invisible differences (whitespace, trailing slash)
	// can never be mistaken for a re-binding attempt.
	if old["oidc_issuer"] != "" {
		issuer = old["oidc_issuer"]
	}
	if old["oidc_client_id"] != "" {
		client = old["oidc_client_id"]
	}
	if (issuer != "" && !oidcAdminURL(issuer, true)) || (redirect != "" && !oidcAdminURL(redirect, false)) || (discovery != "" && !oidcAdminURL(discovery, false)) {
		return nil, errors.New("Issuer、Discovery 和回调地址必须是有效的 HTTPS URL（本地开发允许 localhost HTTP），不得包含用户信息、查询参数或片段。")
	}
	if enabled && (name == "" || issuer == "" || client == "" || redirect == "" || (secret == "" && old["oidc_client_secret"] == "")) {
		return nil, errors.New("启用登录前必须填写所有配置和客户端密钥。")
	}
	if redirect != "" {
		u, _ := url.Parse(redirect)
		if u == nil || u.Path != "/oauth/callback" {
			return nil, errors.New("回调路径必须为 /oauth/callback。")
		}
	}
	if old["oidc_issuer"] == "" && old["oidc_client_id"] == "" && (issuer != "" || client != "") {
		{
			exists, err := identitiesExist()
			if err != nil || exists {
				return nil, errors.New("无法确认不存在历史第三方账户绑定，禁止初始化 Issuer 和 Client ID；请先进行受控迁移。")
			}
		}
	}
	updates := map[string]string{"oidc_enabled": "false", "oidc_name": name, "oidc_issuer": issuer, "oidc_client_id": client, "oidc_redirect_url": redirect, "oidc_discovery_url": discovery}
	if enabled {
		updates["oidc_enabled"] = "true"
	}
	if secret != "" {
		updates["oidc_client_secret"] = secret
	}
	return updates, nil
}

func oidcAdminURL(raw string, issuer bool) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Opaque != "" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || strings.Contains(raw, "#") || u.RawPath != "" {
		return false
	}
	if u.Scheme != "https" && !(u.Scheme == "http" && !issuer && u.Hostname() == "localhost") {
		return false
	}
	if issuer && strings.HasSuffix(u.Path, "/") {
		return false
	}
	return true
}

func oidcIdentitiesExist() (bool, error) {
	if database.DB == nil {
		return false, errors.New("database unavailable")
	}
	var exists bool
	err := database.DB.QueryRow("SELECT EXISTS(SELECT 1 FROM oauth_identities LIMIT 1)").Scan(&exists)
	return exists, err
}

func renderOIDCAdmin(c *gin.Context, status int, key, message string) {
	settings := services.GetSettingService().GetAllSettings()
	if settings["oidc_name"] == "" {
		settings["oidc_name"] = "Linux.sb"
	}
	if settings["oidc_issuer"] == "" {
		settings["oidc_issuer"] = "https://linux.sb"
	}
	if settings["oidc_discovery_url"] == "" {
		settings["oidc_discovery_url"] = "https://linux.sb/forum_oauth_discovery"
	}
	if settings["oidc_client_id"] == "" {
		settings["oidc_client_id"] = "foa_3_LbaAtj2WGHnKmNDgUw55pE"
	}
	if settings["oidc_redirect_url"] == "" {
		settings["oidc_redirect_url"] = "http://localhost:8080/oauth/callback"
	}

	hasSecret := settings["oidc_client_secret"] != ""
	delete(settings, "oidc_client_secret")
	data := getCommonAdminData(c, "oidc", "三方登录", "配置标准 OpenID Connect 登录。")
	data["settings"] = settings
	data["hasSecret"] = hasSecret
	data["csrfToken"] = settingsCSRF(c)
	if key != "" {
		data[key] = message
	}
	c.Header("Cache-Control", "no-store")
	c.HTML(status, "admin-oidc.html", data)
}

// AdminOIDCForm renders the independent OIDC configuration page.
func (h *AdminHandler) AdminOIDCForm(c *gin.Context) { renderOIDCAdmin(c, http.StatusOK, "", "") }

// AdminOIDCSave validates and atomically persists the OIDC configuration.
func (h *AdminHandler) AdminOIDCSave(c *gin.Context) {
	expected := settingsCSRF(c)
	if expected == "" || !hmac.Equal([]byte(expected), []byte(c.PostForm("csrf_token"))) {
		c.AbortWithStatus(http.StatusForbidden)
		return
	}
	if err := c.Request.ParseForm(); err != nil {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}
	oidcAdminMu.Lock()
	defer oidcAdminMu.Unlock()
	ss := services.GetSettingService()
	updates, err := oidcUpdates(c.Request.PostForm, ss.GetAllSettings(), oidcIdentitiesExist)
	if err != nil {
		renderOIDCAdmin(c, http.StatusBadRequest, "error", err.Error())
		return
	}
	if err := ss.UpdateMultiple(updates); err != nil {
		renderOIDCAdmin(c, http.StatusInternalServerError, "error", "保存失败，请稍后重试。")
		return
	}
	renderOIDCAdmin(c, http.StatusOK, "success", "设置已保存。")
}
