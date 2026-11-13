package handlers

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log"
	"net/http"

	"ai-later-nav/internal/config"
	"ai-later-nav/internal/database/repository"
	"ai-later-nav/internal/models"
	"ai-later-nav/internal/services"

	"github.com/gin-gonic/gin"
)

// bindingCSRF is scoped to account linking so a token from another form
// domain cannot authorise a bind/unbind.
func bindingCSRF(c *gin.Context) string {
	token, err := c.Cookie("token")
	if err != nil || token == "" {
		return ""
	}
	mac := hmac.New(sha256.New, []byte(config.AppConfig.JWT.Secret))
	mac.Write([]byte("user-binding:" + token))
	return hex.EncodeToString(mac.Sum(nil))
}

func validBindingCSRF(c *gin.Context) bool {
	expected := bindingCSRF(c)
	return expected != "" && hmac.Equal([]byte(expected), []byte(c.PostForm("csrf_token")))
}

type bindingIdentities interface {
	List(userID int64) ([]models.OAuthIdentity, error)
	Unbind(userID int64, providerKey string) error
	IsProviderBound(userID int64, providerKey string) (bool, error)
}

type bindingUsers interface {
	GetByID(id int64) (*models.User, error)
}

type bindingProviders interface {
	LoginProviders() ([]*models.OAuthProvider, error)
	GetBySlug(slug string) (*models.OAuthProvider, error)
}

// AccountBindingHandler renders the personal-center linking page and handles
// the CSRF-protected bind/unbind actions. Dependencies are interfaces so the
// page and handlers can be tested without a live database.
type AccountBindingHandler struct {
	page       *PageHandler
	identities bindingIdentities
	users      bindingUsers
	providers  bindingProviders
	starter    func(c *gin.Context, slug string, userID int64)
}

func NewAccountBindingHandler(page *PageHandler, registry *OAuthRegistry) *AccountBindingHandler {
	return &AccountBindingHandler{
		page:       page,
		identities: services.NewOAuthIdentityService(),
		users:      services.NewUserService(),
		providers:  services.GetOAuthProviderService(),
		starter:    registry.StartBind,
	}
}

// viewData builds the data shared by the full page and the HTMX partial.
func (h *AccountBindingHandler) viewData(c *gin.Context, success, errMsg string) (gin.H, error) {
	userID := c.GetInt64("user_id")
	identities, err := h.identities.List(userID)
	if err != nil {
		return nil, err
	}
	providers, err := h.providers.LoginProviders()
	if err != nil {
		return nil, err
	}
	bound := make(map[string]bool, len(identities))
	for _, identity := range identities {
		bound[identity.Provider] = true
	}
	var available []*models.OAuthProvider
	for _, p := range providers {
		if !bound[p.IdentityKey] {
			available = append(available, p)
		}
	}
	user, err := h.users.GetByID(userID)
	if err != nil {
		return nil, err
	}
	initialCredentials := user != nil && user.PasswordHash == "!"
	canUnbind := !(len(identities) <= 1 && initialCredentials)
	return gin.H{
		"identities":         identities,
		"availableProviders": available,
		"csrfToken":          bindingCSRF(c),
		"canUnbind":          canUnbind,
		"initialCredentials": initialCredentials,
		"success":            success,
		"error":              errMsg,
	}, nil
}

// AccountBindingsPage is the front-end personal-center page.
func (h *AccountBindingHandler) AccountBindingsPage(c *gin.Context) {
	data, err := h.viewData(c, "", "")
	if err != nil {
		log.Printf("account bindings page: %v", err)
		c.HTML(http.StatusInternalServerError, "error.html", gin.H{"error": "页面加载失败"})
		return
	}
	if c.Query("bound") == "1" {
		data["pageNotice"] = "绑定成功，已关联到当前账号。"
	}
	data["pageError"] = bindingPageError(c.Query("error"))
	if err := h.page.renderPage(c, "account-bindings.html", data); err != nil {
		log.Printf("render account bindings: %v", err)
		c.HTML(http.StatusInternalServerError, "error.html", gin.H{"error": "页面渲染失败"})
	}
}

// StartBind is a CSRF-protected POST that launches the OAuth flow with a
// server-side binding intent. No unauthenticated GET entry point is exposed.
func (h *AccountBindingHandler) StartBind(c *gin.Context) {
	userID := c.GetInt64("user_id")
	if userID <= 0 {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	if !validBindingCSRF(c) {
		c.AbortWithStatus(http.StatusForbidden)
		return
	}
	slug := c.Param("slug")
	p, err := h.providers.GetBySlug(slug)
	if err != nil {
		log.Printf("account bind start lookup %s: %v", slug, err)
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}
	if p == nil || !p.Enabled {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	bound, err := h.identities.IsProviderBound(userID, p.IdentityKey)
	if err != nil {
		log.Printf("account bind check %s: %v", slug, err)
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}
	if bound {
		c.Redirect(http.StatusSeeOther, "/account/bindings?error=already")
		return
	}
	h.starter(c, slug, userID)
}

// Unbind removes one identity and returns the refreshed list partial for HTMX.
func (h *AccountBindingHandler) Unbind(c *gin.Context) {
	userID := c.GetInt64("user_id")
	if userID <= 0 {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	if !validBindingCSRF(c) {
		c.AbortWithStatus(http.StatusForbidden)
		return
	}
	slug := c.Param("slug")
	p, err := h.providers.GetBySlug(slug)
	if err != nil {
		log.Printf("account unbind lookup %s: %v", slug, err)
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}
	if p == nil {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	success, errMsg := "", ""
	if err := h.identities.Unbind(userID, p.IdentityKey); err != nil {
		log.Printf("account unbind %s: %v", slug, err)
		errMsg = unbindErrorMessage(err)
	} else {
		success = "已解绑。"
	}
	data, err := h.viewData(c, success, errMsg)
	if err != nil {
		log.Printf("account unbind view %s: %v", slug, err)
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.HTML(http.StatusOK, "partials/account-bindings-list.html", data)
}

// bindingPageError maps callback error codes to whitelisted messages.
func bindingPageError(code string) string {
	switch code {
	case "owned":
		return "该三方账号已被其他账号绑定。"
	case "already":
		return "该平台已绑定，请先解绑后再绑定新账号。"
	case "session":
		return "登录状态已变化，请重新发起绑定。"
	case "failed":
		return "绑定失败，请稍后重试。"
	}
	return ""
}

func unbindErrorMessage(err error) string {
	switch {
	case errors.Is(err, services.ErrLastLoginMethod):
		return "这是你唯一的登录方式，请先设置本站密码后再解绑。"
	case errors.Is(err, repository.ErrIdentityNotFound), errors.Is(err, services.ErrBindingNotFound):
		return "未找到该绑定记录。"
	default:
		return "解绑失败，请稍后重试。"
	}
}
