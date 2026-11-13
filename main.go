package main

import (
	"ai-later-nav/internal/config"
	"ai-later-nav/internal/database"
	"ai-later-nav/internal/database/repository"
	"ai-later-nav/internal/handlers"
	"ai-later-nav/internal/middleware"
	"ai-later-nav/internal/services"
	"ai-later-nav/internal/web"
	"embed"
	"io/fs"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
)

//go:embed templates/*
var templateFS embed.FS

//go:embed static
var staticFS embed.FS

func main() {
	if err := config.LoadConfig(); err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	if err := database.Init(database.MySQLConfig{
		Host:     config.AppConfig.MySQL.Host,
		Port:     config.AppConfig.MySQL.Port,
		Username: config.AppConfig.MySQL.Username,
		Password: config.AppConfig.MySQL.Password,
		Database: config.AppConfig.MySQL.Database,
	}); err != nil {
		log.Fatalf("Failed to init database: %v", err)
	}
	defer database.Close()

	if err := database.RunMigrations(database.DB, "internal/database/migrations"); err != nil {
		log.Fatalf("Failed to run migrations: %v", err)
	}

	if err := middleware.InitAccessLogger(); err != nil {
		log.Fatalf("Failed to init access logger: %v", err)
	}

	if err := os.MkdirAll(config.AppConfig.UploadDir, 0o755); err != nil {
		log.Fatalf("Failed to create upload dir %q: %v", config.AppConfig.UploadDir, err)
	}

	settingService := services.GetSettingService()
	if err := settingService.LoadCache(); err != nil {
		log.Printf("Warning: could not load settings: %v", err)
	}
	if err := services.GetOAuthProviderService().MigrateLegacySettings(settingService); err != nil {
		log.Printf("Warning: could not migrate legacy OIDC settings: %v", err)
	}

	leaderboardRepo := repository.NewLeaderboardRepository(database.DB)
	cronRepo := repository.NewCronTaskRepository(database.DB)
	leaderboardTasks := services.NewLeaderboardTasks(leaderboardRepo, cronRepo, &http.Client{Timeout: 20 * time.Second}, services.BenchLMLeaderboardURL)
	if err := leaderboardTasks.Start(); err != nil {
		log.Fatalf("Failed to start leaderboard tasks: %v", err)
	}
	defer leaderboardTasks.Stop()

	r := gin.New()
	// Authorization codes and state must not be written to Gin's request log.
	r.Use(gin.LoggerWithConfig(gin.LoggerConfig{SkipPaths: []string{"/oauth/callback"}}), gin.Recovery())

	tmplSub, _ := fs.Sub(templateFS, "templates")
	pageTemplates := web.BuildPageTemplates(tmplSub)
	sharedTemplates := web.BuildSharedTemplates(tmplSub)
	r.SetHTMLTemplate(sharedTemplates)

	staticSub, _ := fs.Sub(staticFS, "static")
	staticHandler := handlers.NewStaticHandler(staticSub, config.AppConfig.UploadDir)
	r.GET("/static/*filepath", gin.WrapH(staticHandler))
	r.HEAD("/static/*filepath", gin.WrapH(staticHandler))

	r.Use(middleware.OptionalAuth())
	r.Use(middleware.AddGlobalContext())
	r.Use(middleware.SetupRequired())
	r.Use(middleware.AccessLog())

	pageHandler := handlers.NewPageHandler(pageTemplates)
	leaderboardHandler := handlers.NewLeaderboardPageHandler(pageHandler, leaderboardRepo)
	apiHandler := handlers.NewAPIHandler()
	adminHandler := handlers.NewAdminHandler()
	cronHandler := handlers.NewAdminCronTaskHandler(cronRepo, leaderboardTasks.Scheduler)
	oauthRegistry := handlers.NewOAuthRegistry(services.GetOAuthProviderService())
	// Legacy single-provider URLs keep working for the already-registered
	// Linux.sb callback; new providers use /oauth/<slug>/*.
	r.GET("/oauth/login", func(c *gin.Context) {
		c.Redirect(http.StatusMovedPermanently, "/oauth/default/login")
	})
	r.GET("/oauth/callback", func(c *gin.Context) {
		target := "/oauth/default/callback"
		if c.Request.URL.RawQuery != "" {
			target += "?" + c.Request.URL.RawQuery
		}
		c.Redirect(http.StatusMovedPermanently, target)
	})
	r.GET("/oauth/:slug/login", oauthRegistry.StartLogin)
	r.GET("/oauth/:slug/callback", oauthRegistry.Callback)

	r.GET("/setup", pageHandler.SetupPage)
	r.POST("/api/setup", apiHandler.Setup)

	r.GET("/", pageHandler.HomePage)
	r.GET("/search", pageHandler.SearchPage)
	r.GET("/leaderboard", leaderboardHandler.LeaderboardPage)

	r.GET("/login", pageHandler.LoginPage)
	r.GET("/register", pageHandler.RegisterPage)

	r.POST("/api/auth/login", apiHandler.Login)
	r.POST("/api/auth/register", apiHandler.Register)
	r.POST("/api/auth/logout", apiHandler.Logout)

	r.GET("/api/search", apiHandler.SearchSites)
	r.GET("/api/search/suggest", apiHandler.SearchSuggestions)
	r.GET("/api/sites/:id", apiHandler.SiteDetail)
	r.GET("/api/sites/:id/stats", apiHandler.SiteStats)
	r.POST("/api/sites/:id/visit", apiHandler.RecordVisit)

	auth := r.Group("/")
	auth.Use(middleware.AuthMiddleware())
	{
		auth.GET("/dashboard", pageHandler.UserDashboard)
		auth.GET("/api/profile", handlers.GetProfile)
		auth.POST("/api/profile", handlers.UpdateProfile)
		auth.POST("/api/auth/set-initial-credentials", apiHandler.SetInitialCredentials)
		auth.POST("/api/auth/change-password", apiHandler.ChangePassword)
		auth.POST("/api/favorites/:id", apiHandler.ToggleFavorite)
		auth.GET("/submit", pageHandler.SubmitPage)
		auth.POST("/api/submissions", handlers.CreateSubmission)
	}

	admin := r.Group("/admin")
	admin.Use(middleware.AuthMiddleware())
	admin.Use(middleware.AdminRequired())
	{
		admin.GET("/", adminHandler.AdminIndex)
		admin.GET("/sites", adminHandler.AdminSites)
		admin.GET("/sites/add", adminHandler.AdminAddSiteForm)
		admin.POST("/sites/add", adminHandler.AdminAddSite)
		admin.GET("/sites/edit/:id", adminHandler.AdminEditSiteForm)
		admin.POST("/sites/edit/:id", adminHandler.AdminEditSite)
		admin.GET("/sites/delete/:id", adminHandler.AdminDeleteSite)
		admin.GET("/sites/toggle-featured/:id", adminHandler.AdminToggleSiteFeatured)
		admin.GET("/nav", adminHandler.AdminNav)
		admin.GET("/nav/add", adminHandler.AdminNavAddForm)
		admin.POST("/nav/add", adminHandler.AdminNavAdd)
		admin.GET("/nav/edit/:id", adminHandler.AdminNavEditForm)
		admin.POST("/nav/edit/:id", adminHandler.AdminNavEdit)
		admin.GET("/nav/delete/:id", adminHandler.AdminNavDelete)
		admin.GET("/friend-links", adminHandler.AdminFriendLinks)
		admin.GET("/friend-links/add", adminHandler.AdminFriendLinkAddForm)
		admin.POST("/friend-links/add", adminHandler.AdminFriendLinkAdd)
		admin.GET("/friend-links/edit/:id", adminHandler.AdminFriendLinkEditForm)
		admin.POST("/friend-links/edit/:id", adminHandler.AdminFriendLinkEdit)
		admin.POST("/friend-links/:id/toggle", adminHandler.AdminFriendLinkToggle)
		admin.POST("/friend-links/:id/move", adminHandler.AdminFriendLinkMove)
		admin.POST("/friend-links/:id/delete", adminHandler.AdminFriendLinkDelete)
		admin.POST("/upload", adminHandler.AdminUploadImage)
		admin.GET("/users", adminHandler.AdminUsers)
		admin.POST("/users/:id/role", adminHandler.AdminUpdateUserRole)
		admin.POST("/users/:id/password", adminHandler.AdminResetUserPassword)
		admin.POST("/users/:id/status", adminHandler.AdminUpdateUserStatus)
		admin.GET("/categories", adminHandler.AdminCategories)
		admin.POST("/categories/add", adminHandler.AdminCategoryAdd)
		admin.POST("/categories/:id/rename", adminHandler.AdminCategoryRename)
		admin.POST("/categories/:id/move", adminHandler.AdminCategoryMove)
		admin.POST("/categories/:id/delete", adminHandler.AdminCategoryDelete)
		admin.GET("/tags", adminHandler.AdminTags)
		admin.POST("/tags/add", adminHandler.AdminTagAdd)
		admin.POST("/tags/:id/rename", adminHandler.AdminTagRename)
		admin.POST("/tags/:id/merge", adminHandler.AdminTagMerge)
		admin.POST("/tags/:id/delete", adminHandler.AdminTagDelete)
		admin.GET("/submissions", adminHandler.AdminSubmissions)
		admin.POST("/submissions/:id/approve", adminHandler.AdminApproveSubmission)
		admin.POST("/submissions/:id/reject", adminHandler.AdminRejectSubmission)
		admin.GET("/stats", adminHandler.AdminStats)
		admin.GET("/settings", adminHandler.AdminSettingsForm)
		admin.POST("/settings", adminHandler.AdminSettingsSave)
		admin.GET("/cron-tasks", cronHandler.List)
		admin.POST("/cron-tasks/:key", cronHandler.Update)
		admin.POST("/cron-tasks/:key/run", cronHandler.RunManual)
		admin.GET("/oidc", adminHandler.AdminProviderList)
		admin.GET("/oidc/new", adminHandler.AdminProviderNew)
		admin.POST("/oidc/new", adminHandler.AdminProviderSave)
		admin.GET("/oidc/:slug", adminHandler.AdminProviderEdit)
		admin.POST("/oidc/:slug", adminHandler.AdminProviderSave)
		admin.POST("/oidc/:slug/delete", adminHandler.AdminProviderDelete)
	}

	port := config.AppConfig.Port
	if port == "" {
		port = "8080"
	}
	log.Printf("Server starting on http://localhost:%s", port)
	r.Run(":" + port)
}
