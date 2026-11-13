package handlers

import (
	"bytes"
	"errors"
	"html/template"
	"log"
	"net/http"
	"strconv"

	"ai-later-nav/internal/models"
	"ai-later-nav/internal/services"
	"ai-later-nav/internal/utils"
	"crypto/md5"
	"github.com/gin-gonic/gin"
)

type PageHandler struct {
	siteService       *services.SiteService
	navService        *services.NavService
	submissionService *services.SiteSubmissionService
	categoryService   *services.CategoryService
	friendLinkService *services.FriendLinkService
	pageTemplates     map[string]*template.Template
	loginProviders    func() ([]*models.OAuthProvider, error)
}

func NewPageHandler(pageTemplates map[string]*template.Template) *PageHandler {
	return &PageHandler{
		siteService:       services.NewSiteService(),
		navService:        services.NewNavService(),
		submissionService: services.NewSiteSubmissionService(),
		categoryService:   services.NewCategoryService(),
		friendLinkService: services.NewFriendLinkService(),
		pageTemplates:     pageTemplates,
		loginProviders:    services.GetOAuthProviderService().LoginProviders,
	}
}

func (h *PageHandler) renderPage(c *gin.Context, name string, data gin.H) error {
	tmpl, ok := h.pageTemplates[name]
	if !ok {
		return errors.New("page template not found: " + name)
	}

	if data == nil {
		data = gin.H{}
	}
	if h.loginProviders != nil {
		if providers, err := h.loginProviders(); err == nil {
			data["oauthProviders"] = providers
		}
	}
	if v, exists := c.Get("Copyright"); exists {
		data["Copyright"] = v
	}
	if v, exists := c.Get("SiteName"); exists {
		data["SiteName"] = v
	}
	if v, exists := c.Get("isLoggedIn"); exists {
		data["isLoggedIn"] = v
	}
	if v, exists := c.Get("username"); exists {
		data["username"] = v
	}
	if v, exists := c.Get("displayName"); exists {
		data["displayName"] = v
	}
	if v, exists := c.Get("role"); exists {
		data["role"] = v
	}
	if _, exists := data["navItems"]; !exists {
		if h.navService != nil {
			if navItems, err := h.navService.ListEnabled(); err == nil {
				data["navItems"] = navItems
			}
		}
	}

	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, name, data); err != nil {
		return err
	}

	c.Data(http.StatusOK, "text/html; charset=utf-8", buf.Bytes())
	return nil
}

func (h *PageHandler) HomePage(c *gin.Context) {
	sites, err := h.siteService.GetAll()
	if err != nil {
		c.HTML(http.StatusInternalServerError, "error.html", gin.H{
			"error": "加载站点数据失败",
		})
		return
	}

	if c.GetBool("isLoggedIn") {
		userService := services.NewUserService()
		if favIDs, err := userService.GetFavoriteIDs(c.GetInt64("user_id")); err == nil && len(favIDs) > 0 {
			favSet := make(map[int64]bool, len(favIDs))
			for _, id := range favIDs {
				favSet[id] = true
			}
			for i := range sites {
				sites[i].IsFav = favSet[sites[i].ID]
			}
		}
	}

	categories, _ := h.categoryService.ListAll()
	friendLinks, err := h.friendLinkService.ListEnabled()
	if err != nil {
		log.Printf("list friend links: %v", err)
		friendLinks = nil
	}
	for i := range friendLinks {
		friendLinks[i].Color = friendLinkColor(friendLinks[i].Name)
		friendLinks[i].Initials = utils.GetInitialsFromName(friendLinks[i].Name)
	}
	copyright, _ := c.Get("Copyright")

	if err := h.renderPage(c, "index.html", gin.H{
		"sites":       sites,
		"categories":  categories,
		"friendLinks": friendLinks,
		"Copyright":   copyright,
		"isLoggedIn":  c.GetBool("isLoggedIn"),
		"username":    c.GetString("username"),
	}); err != nil {
		c.HTML(http.StatusInternalServerError, "error.html", gin.H{
			"error": "页面渲染失败",
		})
	}
}

// friendLinkColor 友链头像配色，限制在 5 种预设色中，避免整排过于花哨。
func friendLinkColor(name string) string {
	palette := []string{"#3b82f6", "#10b981", "#f59e0b", "#8b5cf6", "#ec4899"}
	hash := md5.Sum([]byte(name))
	return palette[int(hash[0])%len(palette)]
}

func (h *PageHandler) SearchPage(c *gin.Context) {
	query := c.Query("q")
	category := c.Query("category")
	sortBy := c.DefaultQuery("sort", "rating")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))

	sites, total, err := h.siteService.Search(query, category, sortBy, page, 20)
	if err != nil {
		c.HTML(http.StatusInternalServerError, "error.html", gin.H{
			"error": "搜索失败",
		})
		return
	}

	if c.GetBool("isLoggedIn") {
		userService := services.NewUserService()
		if favIDs, err := userService.GetFavoriteIDs(c.GetInt64("user_id")); err == nil && len(favIDs) > 0 {
			favSet := make(map[int64]bool, len(favIDs))
			for _, id := range favIDs {
				favSet[id] = true
			}
			for i := range sites {
				sites[i].IsFav = favSet[sites[i].ID]
			}
		}
	}

	categories, _ := h.categoryService.ListAll()
	copyright, _ := c.Get("Copyright")

	if err := h.renderPage(c, "index.html", gin.H{
		"sites":            sites,
		"categories":       categories,
		"query":            query,
		"selectedCategory": category,
		"selectedSort":     sortBy,
		"total":            total,
		"page":             page,
		"Copyright":        copyright,
		"isLoggedIn":       c.GetBool("isLoggedIn"),
		"username":         c.GetString("username"),
	}); err != nil {
		c.HTML(http.StatusInternalServerError, "error.html", gin.H{
			"error": "页面渲染失败",
		})
	}
}

func (h *PageHandler) LoginPage(c *gin.Context) {
	if err := h.renderPage(c, "login.html", gin.H{
		"Copyright": c.GetString("Copyright"),
	}); err != nil {
		c.HTML(http.StatusInternalServerError, "error.html", gin.H{
			"error": "页面渲染失败",
		})
	}
}

func (h *PageHandler) RegisterPage(c *gin.Context) {
	if err := h.renderPage(c, "register.html", gin.H{
		"Copyright": c.GetString("Copyright"),
	}); err != nil {
		c.HTML(http.StatusInternalServerError, "error.html", gin.H{
			"error": "页面渲染失败",
		})
	}
}

func (h *PageHandler) UserDashboard(c *gin.Context) {
	userID := c.GetInt64("user_id")
	userService := services.NewUserService()

	favoriteIDs, _ := userService.GetFavoriteIDs(userID)
	var favoriteSites []models.SiteDisplay
	if len(favoriteIDs) > 0 {
		favoriteSites, _ = h.siteService.GetByIDs(favoriteIDs)
	}
	user, _ := userService.GetByID(userID)
	initialCredentials := user != nil && user.PasswordHash == "!"
	mySubmissions, _ := h.submissionService.ListMine(userID, 20)
	copyright, _ := c.Get("Copyright")
	if err := h.renderPage(c, "dashboard.html", gin.H{
		"favoriteIDs":        favoriteIDs,
		"favoriteSites":      favoriteSites,
		"mySubmissions":      mySubmissions,
		"Copyright":          copyright,
		"isLoggedIn":         true,
		"username":           c.GetString("username"),
		"initialCredentials": initialCredentials,
		"csrfToken":          profileCSRF(c),
	}); err != nil {
		c.HTML(http.StatusInternalServerError, "error.html", gin.H{
			"error": "页面渲染失败",
		})
	}
}

func (h *PageHandler) SetupPage(c *gin.Context) {
	userService := services.NewUserService()
	hasUser, _ := userService.HasAnyUser()
	if hasUser {
		c.Redirect(http.StatusFound, "/")
		return
	}

	copyright, _ := c.Get("Copyright")
	if err := h.renderPage(c, "setup.html", gin.H{
		"Copyright": copyright,
	}); err != nil {
		c.HTML(http.StatusInternalServerError, "error.html", gin.H{
			"error": "页面渲染失败",
		})
	}
}
