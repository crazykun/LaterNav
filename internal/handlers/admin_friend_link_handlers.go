package handlers

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log"
	"net/http"
	"strconv"

	"ai-later-nav/internal/config"
	"ai-later-nav/internal/models"
	"ai-later-nav/internal/services"
	"github.com/gin-gonic/gin"
)

func friendLinkCSRF(c *gin.Context) string {
	token, err := c.Cookie("token")
	if err != nil || token == "" {
		return ""
	}
	mac := hmac.New(sha256.New, []byte(config.AppConfig.JWT.Secret))
	mac.Write([]byte("admin-friend-links:" + token))
	return hex.EncodeToString(mac.Sum(nil))
}

func validFriendLinkRequest(c *gin.Context) bool {
	expected := friendLinkCSRF(c)
	return expected != "" && hmac.Equal([]byte(expected), []byte(c.PostForm("csrf_token")))
}

func friendLinkErrorMessage(err error) string {
	switch {
	case errors.Is(err, services.ErrFriendLinkNotFound),
		errors.Is(err, services.ErrFriendLinkName),
		errors.Is(err, services.ErrFriendLinkURL),
		errors.Is(err, services.ErrFriendLinkDesc),
		errors.Is(err, services.ErrFriendLinkLogo):
		return err.Error()
	default:
		return "操作失败，请稍后重试"
	}
}

func (h *AdminHandler) renderFriendLinkList(c *gin.Context, errorMsg string) {
	links, err := h.friendLinkService.GetAll()
	if err != nil {
		log.Printf("list friend links: %v", err)
		c.HTML(http.StatusInternalServerError, "error.html", gin.H{"error": "加载友情链接失败"})
		return
	}
	common := getCommonAdminData(c, "friend-links", "友情链接", "配置首页底部的合作伙伴链接。")
	common["links"] = links
	common["csrfToken"] = friendLinkCSRF(c)
	if errorMsg != "" {
		common["error"] = errorMsg
	}
	c.HTML(http.StatusOK, "admin-friend-links.html", common)
}

func (h *AdminHandler) AdminFriendLinks(c *gin.Context) { h.renderFriendLinkList(c, "") }

// friendLinkFormData 准备新增/编辑共用表单数据。
func friendLinkFormData(c *gin.Context, title, action string, link *models.FriendLink) gin.H {
	common := getCommonAdminData(c, "friend-links", title, "配置首页底部的合作伙伴链接。")
	common["formTitle"] = title
	common["formAction"] = action
	common["item"] = link
	if link != nil {
		common["logoValue"] = link.Logo
	}
	common["csrfToken"] = friendLinkCSRF(c)
	return common
}

func (h *AdminHandler) AdminFriendLinkAddForm(c *gin.Context) {
	last := 0
	if links, err := h.friendLinkService.GetAll(); err == nil && len(links) > 0 {
		last = links[len(links)-1].SortOrder
	}
	common := friendLinkFormData(c, "添加友链", "/admin/friend-links/add", &models.FriendLink{Enabled: true, SortOrder: last + 10})
	c.HTML(http.StatusOK, "admin-friend-link-form.html", common)
}

// parseFriendLinkForm 解析表单并保留原始输入供失败回填。
func parseFriendLinkForm(c *gin.Context) (*models.FriendLink, string) {
	sortOrder, sortErr := strconv.Atoi(c.PostForm("SortOrder"))
	if sortErr != nil || sortOrder < 0 {
		sortOrder = 0
	}
	link := &models.FriendLink{
		Name:        c.PostForm("Name"),
		URL:         c.PostForm("URL"),
		Description: c.PostForm("Description"),
		Logo:        c.PostForm("Logo"),
		SortOrder:   sortOrder,
		Enabled:     c.PostForm("Enabled") == "on",
	}
	return link, link.URL
}

func (h *AdminHandler) AdminFriendLinkAdd(c *gin.Context) {
	if !validFriendLinkRequest(c) {
		c.AbortWithStatus(http.StatusForbidden)
		return
	}
	link, _ := parseFriendLinkForm(c)
	if _, err := h.friendLinkService.Create(link); err != nil {
		log.Printf("create friend link: %v", err)
		common := friendLinkFormData(c, "添加友链", "/admin/friend-links/add", link)
		common["error"] = friendLinkErrorMessage(err)
		c.HTML(http.StatusOK, "admin-friend-link-form.html", common)
		return
	}
	c.Redirect(http.StatusFound, "/admin/friend-links")
}

func (h *AdminHandler) AdminFriendLinkEditForm(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.HTML(http.StatusBadRequest, "error.html", gin.H{"error": "无效的友情链接 ID"})
		return
	}
	link, err := h.friendLinkService.GetByID(id)
	if err != nil {
		c.HTML(http.StatusNotFound, "error.html", gin.H{"error": "友情链接不存在"})
		return
	}
	common := friendLinkFormData(c, "编辑友链", "/admin/friend-links/edit/"+strconv.FormatInt(id, 10), link)
	c.HTML(http.StatusOK, "admin-friend-link-form.html", common)
}

func (h *AdminHandler) AdminFriendLinkEdit(c *gin.Context) {
	if !validFriendLinkRequest(c) {
		c.AbortWithStatus(http.StatusForbidden)
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.HTML(http.StatusBadRequest, "error.html", gin.H{"error": "无效的友情链接 ID"})
		return
	}
	link, _ := parseFriendLinkForm(c)
	link.ID = id
	if err := h.friendLinkService.Update(link); err != nil {
		log.Printf("update friend link: %v", err)
		common := friendLinkFormData(c, "编辑友链", "/admin/friend-links/edit/"+strconv.FormatInt(id, 10), link)
		common["error"] = friendLinkErrorMessage(err)
		c.HTML(http.StatusOK, "admin-friend-link-form.html", common)
		return
	}
	c.Redirect(http.StatusFound, "/admin/friend-links")
}

func (h *AdminHandler) AdminFriendLinkToggle(c *gin.Context) {
	if !validFriendLinkRequest(c) {
		c.AbortWithStatus(http.StatusForbidden)
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		h.renderFriendLinkList(c, services.ErrFriendLinkNotFound.Error())
		return
	}
	if err := h.friendLinkService.ToggleEnabled(id); err != nil {
		h.renderFriendLinkList(c, friendLinkErrorMessage(err))
		return
	}
	c.Redirect(http.StatusFound, "/admin/friend-links")
}

func (h *AdminHandler) AdminFriendLinkMove(c *gin.Context) {
	if !validFriendLinkRequest(c) {
		c.AbortWithStatus(http.StatusForbidden)
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		h.renderFriendLinkList(c, services.ErrFriendLinkNotFound.Error())
		return
	}
	if err := h.friendLinkService.Move(id, c.PostForm("direction")); err != nil {
		h.renderFriendLinkList(c, friendLinkErrorMessage(err))
		return
	}
	c.Redirect(http.StatusFound, "/admin/friend-links")
}

func (h *AdminHandler) AdminFriendLinkDelete(c *gin.Context) {
	if !validFriendLinkRequest(c) {
		c.AbortWithStatus(http.StatusForbidden)
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		h.renderFriendLinkList(c, services.ErrFriendLinkNotFound.Error())
		return
	}
	if err := h.friendLinkService.Delete(id); err != nil {
		h.renderFriendLinkList(c, friendLinkErrorMessage(err))
		return
	}
	c.Redirect(http.StatusFound, "/admin/friend-links")
}
