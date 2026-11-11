package handlers

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"ai-later-nav/internal/config"
	"ai-later-nav/internal/services"
	"github.com/gin-gonic/gin"
)

func taxonomyCSRF(c *gin.Context) string {
	token, err := c.Cookie("token")
	if err != nil || token == "" {
		return ""
	}
	mac := hmac.New(sha256.New, []byte(config.AppConfig.JWT.Secret))
	mac.Write([]byte("admin-taxonomy:" + token))
	return hex.EncodeToString(mac.Sum(nil))
}

func validTaxonomyRequest(c *gin.Context) bool {
	expected := taxonomyCSRF(c)
	return expected != "" && hmac.Equal([]byte(expected), []byte(c.PostForm("csrf_token")))
}

func taxonomyPage(c *gin.Context) (string, int) {
	query := strings.TrimSpace(c.Query("q"))
	page, _ := strconv.Atoi(c.Query("page"))
	if c.Request.Method == http.MethodPost {
		query = strings.TrimSpace(c.PostForm("q"))
		page, _ = strconv.Atoi(c.PostForm("page"))
	}
	if page < 1 {
		page = 1
	}
	return query, page
}

func taxonomyReturn(c *gin.Context, basePath string) {
	query, page := taxonomyPage(c)
	values := make([]string, 0, 2)
	if query != "" {
		values = append(values, "q="+urlQueryEscape(query))
	}
	if page > 1 {
		values = append(values, "page="+strconv.Itoa(page))
	}
	if len(values) > 0 {
		basePath += "?" + strings.Join(values, "&")
	}
	c.Redirect(http.StatusFound, basePath)
}

// Kept small and local so redirect parameters are always URL-encoded.
func urlQueryEscape(value string) string { return url.QueryEscape(value) }

func taxonomyError(err error) string {
	switch {
	case errors.Is(err, services.ErrInvalidCategoryName),
		errors.Is(err, services.ErrCategoryExists),
		errors.Is(err, services.ErrCategoryNotFound),
		errors.Is(err, services.ErrInvalidTagName),
		errors.Is(err, services.ErrTagExists),
		errors.Is(err, services.ErrTagNotFound),
		errors.Is(err, services.ErrInvalidMerge):
		return err.Error()
	default:
		if strings.Contains(err.Error(), "still in use") {
			return services.ErrCategoryInUse.Error()
		}
		return "操作失败，请稍后重试"
	}
}

// categoryIconOptions 与导航管理共用图标集，首位"自动"表示按分类名匹配。
func categoryIconOptions() []gin.H {
	labels := map[string]string{
		"grid":   "网格",
		"chat":   "对话",
		"create": "创作",
		"code":   "编程",
		"tool":   "工具",
		"star":   "星标",
		"link":   "链接",
	}
	options := make([]gin.H, 0, len(services.IconKeys)+1)
	options = append(options, gin.H{"Value": "", "Label": "自动匹配"})
	for _, k := range services.IconKeys {
		options = append(options, gin.H{"Value": k, "Label": labels[k] + "（" + k + "）"})
	}
	return options
}

func (h *AdminHandler) renderAdminCategories(c *gin.Context, errorMsg string) {
	query, page := taxonomyPage(c)
	data, err := h.categoryService.Page(query, page, 20)
	if err != nil {
		log.Printf("list categories: %v", err)
		c.HTML(http.StatusInternalServerError, "error.html", gin.H{"error": "加载分类失败"})
		return
	}
	common := getCommonAdminData(c, "categories", "分类管理", "维护前台分类、显示顺序和站点归属。")
	common["categories"] = data.Categories
	common["total"] = data.Total
	common["page"] = data.Page
	common["pageSize"] = data.PageSize
	common["totalPages"] = data.TotalPages
	common["query"] = query
	common["basePath"] = "/admin/categories"
	common["iconOptions"] = categoryIconOptions()
	common["csrfToken"] = taxonomyCSRF(c)
	if errorMsg != "" {
		common["error"] = errorMsg
	}
	c.HTML(http.StatusOK, "admin-categories.html", common)
}

func (h *AdminHandler) AdminCategories(c *gin.Context) { h.renderAdminCategories(c, "") }

func (h *AdminHandler) AdminCategoryAdd(c *gin.Context) {
	if !validTaxonomyRequest(c) {
		c.AbortWithStatus(http.StatusForbidden)
		return
	}
	if err := h.categoryService.Create(c.PostForm("name"), c.PostForm("icon")); err != nil {
		h.renderAdminCategories(c, taxonomyError(err))
		return
	}
	taxonomyReturn(c, "/admin/categories")
}

func (h *AdminHandler) AdminCategoryRename(c *gin.Context) {
	if !validTaxonomyRequest(c) {
		c.AbortWithStatus(http.StatusForbidden)
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		h.renderAdminCategories(c, services.ErrCategoryNotFound.Error())
		return
	}
	if err := h.categoryService.Rename(id, c.PostForm("name"), c.PostForm("icon")); err != nil {
		h.renderAdminCategories(c, taxonomyError(err))
		return
	}
	taxonomyReturn(c, "/admin/categories")
}

func (h *AdminHandler) AdminCategoryDelete(c *gin.Context) {
	if !validTaxonomyRequest(c) {
		c.AbortWithStatus(http.StatusForbidden)
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		h.renderAdminCategories(c, services.ErrCategoryNotFound.Error())
		return
	}
	if err := h.categoryService.Delete(id); err != nil {
		h.renderAdminCategories(c, taxonomyError(err))
		return
	}
	taxonomyReturn(c, "/admin/categories")
}

func (h *AdminHandler) AdminCategoryMove(c *gin.Context) {
	if !validTaxonomyRequest(c) {
		c.AbortWithStatus(http.StatusForbidden)
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		h.renderAdminCategories(c, services.ErrCategoryNotFound.Error())
		return
	}
	if err := h.categoryService.Move(id, c.PostForm("direction")); err != nil {
		h.renderAdminCategories(c, taxonomyError(err))
		return
	}
	taxonomyReturn(c, "/admin/categories")
}

func (h *AdminHandler) renderAdminTags(c *gin.Context, errorMsg string) {
	query, page := taxonomyPage(c)
	data, err := h.tagService.Page(query, page, 20)
	if err != nil {
		log.Printf("list tags: %v", err)
		c.HTML(http.StatusInternalServerError, "error.html", gin.H{"error": "加载标签失败"})
		return
	}
	allTags, err := h.tagService.ListAll()
	if err != nil {
		log.Printf("list merge targets: %v", err)
		c.HTML(http.StatusInternalServerError, "error.html", gin.H{"error": "加载标签失败"})
		return
	}
	common := getCommonAdminData(c, "tags", "标签管理", "整理站点标签、合并重复项并清理无用标签。")
	common["tags"] = data.Tags
	common["allTags"] = allTags
	common["total"] = data.Total
	common["page"] = data.Page
	common["pageSize"] = data.PageSize
	common["totalPages"] = data.TotalPages
	common["query"] = query
	common["basePath"] = "/admin/tags"
	common["csrfToken"] = taxonomyCSRF(c)
	if errorMsg != "" {
		common["error"] = errorMsg
	}
	c.HTML(http.StatusOK, "admin-tags.html", common)
}

func (h *AdminHandler) AdminTags(c *gin.Context) { h.renderAdminTags(c, "") }

func (h *AdminHandler) AdminTagAdd(c *gin.Context) {
	if !validTaxonomyRequest(c) {
		c.AbortWithStatus(http.StatusForbidden)
		return
	}
	if err := h.tagService.Create(c.PostForm("name")); err != nil {
		h.renderAdminTags(c, taxonomyError(err))
		return
	}
	taxonomyReturn(c, "/admin/tags")
}

func (h *AdminHandler) AdminTagRename(c *gin.Context) {
	if !validTaxonomyRequest(c) {
		c.AbortWithStatus(http.StatusForbidden)
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		h.renderAdminTags(c, services.ErrTagNotFound.Error())
		return
	}
	if err := h.tagService.Rename(id, c.PostForm("name")); err != nil {
		h.renderAdminTags(c, taxonomyError(err))
		return
	}
	taxonomyReturn(c, "/admin/tags")
}

func (h *AdminHandler) AdminTagMerge(c *gin.Context) {
	if !validTaxonomyRequest(c) {
		c.AbortWithStatus(http.StatusForbidden)
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		h.renderAdminTags(c, services.ErrTagNotFound.Error())
		return
	}
	targetID, err := strconv.ParseInt(c.PostForm("target_id"), 10, 64)
	if err != nil {
		h.renderAdminTags(c, services.ErrInvalidMerge.Error())
		return
	}
	if err := h.tagService.Merge(id, targetID); err != nil {
		h.renderAdminTags(c, taxonomyError(err))
		return
	}
	taxonomyReturn(c, "/admin/tags")
}

func (h *AdminHandler) AdminTagDelete(c *gin.Context) {
	if !validTaxonomyRequest(c) {
		c.AbortWithStatus(http.StatusForbidden)
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		h.renderAdminTags(c, services.ErrTagNotFound.Error())
		return
	}
	if err := h.tagService.Delete(id); err != nil {
		h.renderAdminTags(c, taxonomyError(err))
		return
	}
	taxonomyReturn(c, "/admin/tags")
}
