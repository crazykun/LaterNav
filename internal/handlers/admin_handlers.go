package handlers

import (
	"ai-later-nav/internal/config"
	"ai-later-nav/internal/models"
	"ai-later-nav/internal/services"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

// AdminPageData contains common fields for all admin pages
type AdminPageData struct {
	Username  string
	IsAdmin   bool
	PageTitle string
	PageDesc  string
	Section   string
	SiteName  string
}

// getCommonAdminData returns common template data for admin pages
func getCommonAdminData(c *gin.Context, section, title, description string) gin.H {
	return gin.H{
		"username":        c.GetString("username"),
		"adminSection":    section,
		"pageTitle":       title,
		"pageDescription": description,
		"SiteName":        c.GetString("SiteName"),
	}
}

// AdminIndexData contains data specific to admin dashboard
type AdminIndexData struct {
	AdminPageData
	SiteCount      int64
	UserCount      int64
	TodayUsers     int64
	DashboardStats *models.DashboardStats
	TopSites       []models.Site
	RecentUsers    []*models.User
	CategoryStats  []models.CategoryStat
}

type AdminHandler struct {
	siteService *services.SiteService
	userService *services.UserService
	navService  *services.NavService
}

func NewAdminHandler() *AdminHandler {
	return &AdminHandler{
		siteService: services.NewSiteService(),
		userService: services.NewUserService(),
		navService:  services.NewNavService(),
	}
}

func (h *AdminHandler) AdminIndex(c *gin.Context) {
	siteCount, err := h.siteService.Count()
	if err != nil {
		log.Printf("Failed to get site count: %v", err)
		siteCount = 0
	}

	userCount, err := h.userService.CountUsers()
	if err != nil {
		log.Printf("Failed to get user count: %v", err)
		userCount = 0
	}

	todayUsers, err := h.userService.CountTodayUsers()
	if err != nil {
		log.Printf("Failed to get today users: %v", err)
		todayUsers = 0
	}

	dashboardStats, err := h.siteService.GetDashboardStats()
	if err != nil {
		log.Printf("Failed to get dashboard stats: %v", err)
		dashboardStats = &models.DashboardStats{}
	}

	topSites, err := h.siteService.GetTopSites(5)
	if err != nil {
		log.Printf("Failed to get top sites: %v", err)
		topSites = []models.Site{}
	}

	recentUsers, err := h.userService.GetRecentUsers(5)
	if err != nil {
		log.Printf("Failed to get recent users: %v", err)
		recentUsers = []*models.User{}
	}

	categoryStats, err := h.siteService.GetCategoryStats()
	if err != nil {
		log.Printf("Failed to get category stats: %v", err)
		categoryStats = []models.CategoryStat{}
	}

	common := getCommonAdminData(c, "dashboard", "仪表盘", "查看后台关键指标和系统状态。")
	common["siteCount"] = siteCount
	common["userCount"] = userCount
	common["todayUsers"] = todayUsers
	common["dashboardStats"] = dashboardStats
	common["topSites"] = topSites
	common["recentUsers"] = recentUsers
	common["categoryStats"] = categoryStats
	c.HTML(http.StatusOK, "admin-index.html", common)
}

func (h *AdminHandler) AdminSites(c *gin.Context) {
	query := strings.TrimSpace(c.Query("q"))
	featuredFilter := c.Query("featured")

	var featured *bool
	switch featuredFilter {
	case "1":
		v := true
		featured = &v
	case "0":
		v := false
		featured = &v
	}

	sites, err := h.siteService.GetAllFiltered(query, featured)
	if err != nil {
		c.HTML(http.StatusInternalServerError, "error.html", gin.H{
			"error": "加载站点失败",
		})
		return
	}

	// 切换推荐后回跳时保留当前筛选条件；完整 URL 在模板中经 printf 整体输出，
	// 避免 html/template 对 query 片段做 percent-encode
	filterParams := url.Values{}
	if query != "" {
		filterParams.Set("q", query)
	}
	if featuredFilter != "" {
		filterParams.Set("featured", featuredFilter)
	}
	toggleQuery := ""
	if len(filterParams) > 0 {
		toggleQuery = "?" + filterParams.Encode()
	}

	common := getCommonAdminData(c, "sites", "站点管理", "集中维护站点资料、标签和推荐状态。")
	common["sites"] = sites
	common["query"] = query
	common["featuredFilter"] = featuredFilter
	common["toggleQuery"] = toggleQuery
	c.HTML(http.StatusOK, "admin-sites.html", common)
}

// AdminToggleSiteFeatured 列表内快捷切换推荐状态，回跳时保留筛选条件。
func (h *AdminHandler) AdminToggleSiteFeatured(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的站点 ID"})
		return
	}

	if err := h.siteService.ToggleFeatured(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "操作失败"})
		return
	}

	redirect := "/admin/sites"
	if c.Request.URL.RawQuery != "" {
		redirect += "?" + c.Request.URL.RawQuery
	}
	c.Redirect(http.StatusFound, redirect)
}

func (h *AdminHandler) AdminAddSiteForm(c *gin.Context) {
	categories, _ := h.siteService.GetCategories()
	common := getCommonAdminData(c, "sites", "添加站点", "创建新的站点条目并设置分类、标签和推荐状态。")
	common["categories"] = categories
	c.HTML(http.StatusOK, "admin-add-site.html", common)
}

func (h *AdminHandler) AdminAddSite(c *gin.Context) {
	site := &models.Site{
		Name:        strings.TrimSpace(c.PostForm("Name")),
		URL:         strings.TrimSpace(c.PostForm("URL")),
		Description: strings.TrimSpace(c.PostForm("Description")),
		Logo:        strings.TrimSpace(c.PostForm("Logo")),
		Category:    strings.TrimSpace(c.PostForm("Category")),
	}

	if ratingStr := c.PostForm("Rating"); ratingStr != "" {
		if rating, err := strconv.ParseFloat(ratingStr, 64); err == nil {
			site.Rating = rating
		}
	}

	site.Featured = c.PostForm("Featured") == "on"

	tagsStr := c.PostForm("Tags")
	var tags []string
	if tagsStr != "" {
		for _, t := range strings.Split(tagsStr, ",") {
			t = strings.TrimSpace(t)
			if t != "" {
				tags = append(tags, t)
			}
		}
	}

	if _, err := h.siteService.Create(site, tags); err != nil {
		categories, _ := h.siteService.GetCategories()
		common := getCommonAdminData(c, "sites", "添加站点", "创建新的站点条目并设置分类、标签和推荐状态。")
		common["error"] = "创建失败: " + err.Error()
		common["site"] = site
		common["tagsString"] = tagsStr
		common["categories"] = categories
		c.HTML(http.StatusOK, "admin-add-site.html", common)
		return
	}

	c.Redirect(http.StatusFound, "/admin/sites")
}

func (h *AdminHandler) AdminEditSiteForm(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.HTML(http.StatusBadRequest, "error.html", gin.H{
			"error": "无效的站点 ID",
		})
		return
	}

	site, err := h.siteService.GetByID(id)
	if err != nil || site == nil {
		c.HTML(http.StatusNotFound, "error.html", gin.H{
			"error": "站点不存在",
		})
		return
	}

	categories, _ := h.siteService.GetCategories()
	common := getCommonAdminData(c, "sites", "编辑站点", "调整站点信息并维护推荐状态。")
	common["site"] = site
	common["tagsString"] = strings.Join(site.Tags, ", ")
	common["categories"] = categories
	c.HTML(http.StatusOK, "admin-edit-site.html", common)
}

func (h *AdminHandler) AdminEditSite(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.HTML(http.StatusBadRequest, "error.html", gin.H{
			"error": "无效的站点 ID",
		})
		return
	}

	site := &models.Site{
		ID:          id,
		Name:        strings.TrimSpace(c.PostForm("Name")),
		URL:         strings.TrimSpace(c.PostForm("URL")),
		Description: strings.TrimSpace(c.PostForm("Description")),
		Logo:        strings.TrimSpace(c.PostForm("Logo")),
		Category:    strings.TrimSpace(c.PostForm("Category")),
	}

	if ratingStr := c.PostForm("Rating"); ratingStr != "" {
		if rating, err := strconv.ParseFloat(ratingStr, 64); err == nil {
			site.Rating = rating
		}
	}

	site.Featured = c.PostForm("Featured") == "on"

	tagsStr := c.PostForm("Tags")
	var tags []string
	if tagsStr != "" {
		for _, t := range strings.Split(tagsStr, ",") {
			t = strings.TrimSpace(t)
			if t != "" {
				tags = append(tags, t)
			}
		}
	}

	if err := h.siteService.Update(site, tags); err != nil {
		categories, _ := h.siteService.GetCategories()
		common := getCommonAdminData(c, "sites", "编辑站点", "调整站点信息并维护推荐状态。")
		common["error"] = "更新失败: " + err.Error()
		common["site"] = site
		common["tagsString"] = tagsStr
		common["categories"] = categories
		c.HTML(http.StatusOK, "admin-edit-site.html", common)
		return
	}

	c.Redirect(http.StatusFound, "/admin/sites")
}

func (h *AdminHandler) AdminDeleteSite(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的站点 ID"})
		return
	}

	if err := h.siteService.Delete(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除失败"})
		return
	}

	c.Redirect(http.StatusFound, "/admin/sites")
}

func (h *AdminHandler) AdminNav(c *gin.Context) {
	items, err := h.navService.GetAll()
	if err != nil {
		c.HTML(http.StatusInternalServerError, "error.html", gin.H{
			"error": "加载导航失败",
		})
		return
	}

	common := getCommonAdminData(c, "nav", "导航管理", "配置首页分类导航的名称、链接、图标和排序。")
	common["items"] = items
	c.HTML(http.StatusOK, "admin-nav.html", common)
}

func (h *AdminHandler) navFormData(c *gin.Context, title, action string, item *models.NavItem) gin.H {
	common := getCommonAdminData(c, "nav", title, "维护首页分类导航条目。")
	common["item"] = item
	common["formAction"] = action
	common["formTitle"] = title

	options := make([]gin.H, 0, len(services.IconKeys))
	labels := map[string]string{
		"grid":   "网格",
		"chat":   "对话",
		"create": "创作",
		"code":   "编程",
		"tool":   "工具",
		"star":   "星标",
		"link":   "链接",
	}
	for _, k := range services.IconKeys {
		options = append(options, gin.H{"Value": k, "Label": labels[k] + "（" + k + "）"})
	}
	common["iconOptions"] = options
	return common
}

func (h *AdminHandler) AdminNavAddForm(c *gin.Context) {
	// Default new items to the end of the current order.
	sortOrder := 1
	if items, err := h.navService.GetAll(); err == nil && len(items) > 0 {
		sortOrder = items[len(items)-1].SortOrder + 1
	}

	common := h.navFormData(c, "添加导航", "/admin/nav/add", &models.NavItem{SortOrder: sortOrder, Enabled: true})
	c.HTML(http.StatusOK, "admin-nav-form.html", common)
}

func parseNavForm(c *gin.Context, item *models.NavItem) {
	sortOrder, _ := strconv.Atoi(strings.TrimSpace(c.PostForm("SortOrder")))
	item.Name = strings.TrimSpace(c.PostForm("Name"))
	item.URL = strings.TrimSpace(c.PostForm("URL"))
	item.Description = strings.TrimSpace(c.PostForm("Description"))
	item.Icon = strings.TrimSpace(c.PostForm("Icon"))
	item.SortOrder = sortOrder
	item.Enabled = c.PostForm("Enabled") == "on"
}

func (h *AdminHandler) AdminNavAdd(c *gin.Context) {
	item := &models.NavItem{}
	parseNavForm(c, item)

	if _, err := h.navService.Create(item); err != nil {
		common := h.navFormData(c, "添加导航", "/admin/nav/add", item)
		common["error"] = "创建失败: " + err.Error()
		c.HTML(http.StatusOK, "admin-nav-form.html", common)
		return
	}

	c.Redirect(http.StatusFound, "/admin/nav")
}

func (h *AdminHandler) AdminNavEditForm(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.HTML(http.StatusBadRequest, "error.html", gin.H{"error": "无效的导航 ID"})
		return
	}

	item, err := h.navService.GetByID(id)
	if err != nil || item == nil {
		c.HTML(http.StatusNotFound, "error.html", gin.H{"error": "导航不存在"})
		return
	}

	common := h.navFormData(c, "编辑导航", "/admin/nav/edit/"+strconv.FormatInt(id, 10), item)
	c.HTML(http.StatusOK, "admin-nav-form.html", common)
}

func (h *AdminHandler) AdminNavEdit(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.HTML(http.StatusBadRequest, "error.html", gin.H{"error": "无效的导航 ID"})
		return
	}

	item := &models.NavItem{ID: id}
	parseNavForm(c, item)

	if err := h.navService.Update(item); err != nil {
		common := h.navFormData(c, "编辑导航", "/admin/nav/edit/"+strconv.FormatInt(id, 10), item)
		common["error"] = "更新失败: " + err.Error()
		c.HTML(http.StatusOK, "admin-nav-form.html", common)
		return
	}

	c.Redirect(http.StatusFound, "/admin/nav")
}

func (h *AdminHandler) AdminNavDelete(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的导航 ID"})
		return
	}

	if err := h.navService.Delete(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除失败"})
		return
	}

	c.Redirect(http.StatusFound, "/admin/nav")
}

func (h *AdminHandler) AdminUsers(c *gin.Context) {
	h.renderAdminUsers(c, http.StatusOK, "")
}

func (h *AdminHandler) AdminUpdateUserRole(c *gin.Context) {
	targetUserID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		h.renderAdminUsers(c, http.StatusOK, "无效的用户 ID")
		return
	}

	role := strings.TrimSpace(c.PostForm("role"))
	if err := h.userService.SetUserRole(c.GetInt64("user_id"), targetUserID, role); err != nil {
		h.renderAdminUsers(c, http.StatusOK, "操作失败: "+err.Error())
		return
	}

	c.Redirect(http.StatusFound, "/admin/users")
}

func (h *AdminHandler) AdminResetUserPassword(c *gin.Context) {
	targetUserID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		h.renderAdminUsers(c, http.StatusOK, "无效的用户 ID")
		return
	}

	newPassword := c.PostForm("new_password")
	if len(newPassword) < 6 {
		h.renderAdminUsers(c, http.StatusOK, "新密码至少 6 位")
		return
	}

	if err := h.userService.ResetPasswordByAdmin(targetUserID, newPassword); err != nil {
		h.renderAdminUsers(c, http.StatusOK, "操作失败: "+err.Error())
		return
	}

	c.Redirect(http.StatusFound, "/admin/users")
}

func (h *AdminHandler) AdminUpdateUserStatus(c *gin.Context) {
	targetUserID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		h.renderAdminUsers(c, http.StatusOK, "无效的用户 ID")
		return
	}

	status := strings.TrimSpace(c.PostForm("status"))
	if err := h.userService.SetUserStatus(c.GetInt64("user_id"), targetUserID, status); err != nil {
		h.renderAdminUsers(c, http.StatusOK, "操作失败: "+err.Error())
		return
	}

	c.Redirect(http.StatusFound, "/admin/users")
}

func (h *AdminHandler) renderAdminUsers(c *gin.Context, statusCode int, errorMsg string) {
	users, err := h.userService.GetAllUsers()
	if err != nil {
		c.HTML(http.StatusInternalServerError, "error.html", gin.H{
			"error": "加载用户失败",
		})
		return
	}

	common := getCommonAdminData(c, "users", "用户管理", "查看注册用户和角色信息。")
	common["users"] = users
	common["currentUserID"] = c.GetInt64("user_id")
	if errorMsg != "" {
		common["error"] = errorMsg
	}
	c.HTML(statusCode, "admin-users.html", common)
}

func settingsCSRF(c *gin.Context) string {
	token, err := c.Cookie("token")
	if err != nil || token == "" {
		return ""
	}
	mac := hmac.New(sha256.New, []byte(config.AppConfig.JWT.Secret))
	mac.Write([]byte("admin-settings:" + token))
	return hex.EncodeToString(mac.Sum(nil))
}

func renderAdminSettings(c *gin.Context, messageKey, message string) {
	settings := services.GetSettingService().GetAllSettings()
	delete(settings, "oauth_linux_client_secret")
	if settings["oauth_linux_client_id"] == "" {
		settings["oauth_linux_client_id"] = "foa_3_LbaAtj2WGHnKmNDgUw55pE"
	}
	if settings["oauth_linux_redirect_url"] == "" {
		settings["oauth_linux_redirect_url"] = "http://localhost:8080/oauth/callback"
	}
	common := getCommonAdminData(c, "settings", "系统设置", "维护站点信息和登录设置。")
	common["settings"] = settings
	common["csrfToken"] = settingsCSRF(c)
	if messageKey != "" {
		common[messageKey] = message
	}
	c.HTML(http.StatusOK, "admin-settings.html", common)
}

func (h *AdminHandler) AdminSettingsForm(c *gin.Context) {
	renderAdminSettings(c, "", "")
}

func (h *AdminHandler) AdminSettingsSave(c *gin.Context) {
	expected := settingsCSRF(c)
	if expected == "" || !hmac.Equal([]byte(expected), []byte(c.PostForm("csrf_token"))) {
		c.AbortWithStatus(http.StatusForbidden)
		return
	}
	ss := services.GetSettingService()
	updates := map[string]string{
		"site_name": strings.TrimSpace(c.PostForm("site_name")),
		"copyright": strings.TrimSpace(c.PostForm("copyright")),
	}

	redirect := strings.TrimSpace(c.PostForm("oauth_linux_redirect_url"))
	u, err := url.Parse(redirect)
	if err != nil || u.Hostname() == "" || u.User != nil || u.Opaque != "" || u.Path != "/oauth/callback" || u.RawPath != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || !(u.Scheme == "https" || (u.Scheme == "http" && u.Hostname() == "localhost")) {
		renderAdminSettings(c, "error", "回调地址必须使用 HTTPS 或 localhost HTTP，路径必须为 /oauth/callback。")
		return
	}
	updates["oauth_linux_redirect_url"] = redirect
	updates["oauth_linux_client_id"] = strings.TrimSpace(c.PostForm("oauth_linux_client_id"))
	updates["oauth_linux_enabled"] = "false"
	if c.PostForm("oauth_linux_enabled") == "true" {
		updates["oauth_linux_enabled"] = "true"
	}
	if secret := c.PostForm("oauth_linux_client_secret"); secret != "" {
		updates["oauth_linux_client_secret"] = secret
	}
	if err := ss.UpdateMultiple(updates); err != nil {
		renderAdminSettings(c, "error", "保存失败，请稍后重试。")
		return
	}
	renderAdminSettings(c, "success", "设置已保存")
}

func (h *AdminHandler) AdminStats(c *gin.Context) {
	sites, err := h.siteService.GetAll()
	if err != nil {
		c.HTML(http.StatusInternalServerError, "error.html", gin.H{
			"error": "加载站点失败",
		})
		return
	}

	allStats, err := h.siteService.GetAllSitesStats()
	if err != nil {
		c.HTML(http.StatusInternalServerError, "error.html", gin.H{
			"error": "加载统计失败",
		})
		return
	}

	statsMap := make(map[int64]models.SiteStats)
	for _, s := range allStats {
		statsMap[s.SiteID] = s
	}

	type SiteWithStats struct {
		models.SiteDisplay
		Stats *models.SiteStats
	}

	var sitesWithStats []SiteWithStats
	for _, site := range sites {
		sws := SiteWithStats{SiteDisplay: site}
		if stats, ok := statsMap[site.ID]; ok {
			sws.Stats = &stats
		}
		sitesWithStats = append(sitesWithStats, sws)
	}

	common := getCommonAdminData(c, "stats", "访问统计", "按站点查看 PV / UV 趋势概览。")
	common["sites"] = sitesWithStats
	c.HTML(http.StatusOK, "admin-stats.html", common)
}
