package test

import (
	"ai-later-nav/internal/database"
	"ai-later-nav/internal/handlers"
	"ai-later-nav/internal/models"
	"ai-later-nav/internal/web"
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
)

func TestAdminUsersTemplateRendersStatusActionsAndSelfRestrictions(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tmpl := web.BuildSharedTemplates(os.DirFS("../templates"))

	users := []*models.AdminUser{
		{User: models.User{
			ID:        1,
			Username:  "admin",
			Nickname:  "管理员昵称",
			Email:     "admin@example.com",
			Role:      "admin",
			Status:    "active",
			CreatedAt: time.Date(2026, 4, 29, 10, 0, 0, 0, time.UTC),
		}, OAuthIdentities: []models.OAuthIdentity{
			{Provider: "oidc_key", ProviderName: "Linux.sb", DisplayName: "三方管理员", PreferredUsername: "admin-oauth", Email: "oauth-admin@example.com", Picture: "https://example.com/admin.png"},
			{Provider: "github", ProviderName: "GitHub", PreferredUsername: "octoadmin"},
		}},
		{User: models.User{
			ID:        2,
			Username:  "blocked-user",
			Email:     "blocked@example.com",
			Role:      "user",
			Status:    "blocked",
			CreatedAt: time.Date(2026, 4, 29, 11, 0, 0, 0, time.UTC),
		}},
	}

	var out strings.Builder
	err := tmpl.ExecuteTemplate(&out, "admin-users.html", map[string]any{
		"SiteName":        "AI Later",
		"username":        "admin",
		"adminSection":    "users",
		"pageTitle":       "用户管理",
		"pageDescription": "查看注册用户和角色信息。",
		"currentUserID":   int64(1),
		"error":           "操作失败",
		"users":           users,
		"total":           int64(2),
		"basePath":        "/admin/users",
		"page":            1,
		"pageSize":        20,
		"totalPages":      1,
		"query":           "管理员",
	})
	if err != nil {
		t.Fatalf("ExecuteTemplate failed: %v", err)
	}

	html := out.String()
	for _, needle := range []string{
		"状态",
		"操作",
		"已禁用",
		"/admin/users/2/role",
		"/admin/users/2/status",
		"showPwdModal( 1 ",
		"showPwdModal( 2 ",
		"pwdModal",
		"pwdModalForm",
		"当前账号不可改角色或状态",
		"admin-alert-error",
		"管理员昵称",
		"登录名：admin",
		"邮箱：admin@example.com",
		"三方管理员",
		"oauth-admin@example.com",
		"https://example.com/admin.png",
		"octoadmin",
		"三方账号：admin-oauth",
		"邮箱未提供",
		"未绑定",
		`value="管理员"`,
		`name="page" value="1"`,
	} {
		if !strings.Contains(html, needle) {
			t.Fatalf("admin-users.html missing %q in rendered output: %s", needle, html)
		}
	}

	// 无三方昵称时名称直接回退到三方账号，不再重复展示一行。
	if strings.Contains(html, "三方账号：octoadmin") {
		t.Fatalf("admin-users.html duplicated octoadmin fallback line: %s", html)
	}
	// 未设置昵称的会员不重复展示登录名。
	if strings.Contains(html, "登录名：blocked-user") {
		t.Fatalf("admin-users.html duplicated login name for nickname-less user: %s", html)
	}

	for _, forbidden := range []string{
		"/admin/users/1/role",
		"/admin/users/1/status",
	} {
		if strings.Contains(html, forbidden) {
			t.Fatalf("admin-users.html rendered self action %q: %s", forbidden, html)
		}
	}
}

func TestAdminUsersTemplateRendersPagination(t *testing.T) {
	tmpl := web.BuildSharedTemplates(os.DirFS("../templates"))
	var out strings.Builder
	err := tmpl.ExecuteTemplate(&out, "admin-users.html", map[string]any{
		"users": []any{}, "total": int64(65), "page": 3, "pageSize": 20, "totalPages": 4, "query": "AI 工具", "basePath": "/admin/users",
	})
	if err != nil {
		t.Fatalf("ExecuteTemplate failed: %v", err)
	}
	html := out.String()
	for _, needle := range []string{"共 65 个用户", "上一页", "下一页", `aria-current="page">3</a>`, "page=2", "page=4", "q=AI&#43;%E5%B7%A5%E5%85%B7"} {
		if !strings.Contains(html, needle) {
			t.Fatalf("admin-users.html missing pagination %q: %s", needle, html)
		}
	}
}

func TestAdminUsersUpdateUserRoleRedirectsOnSuccess(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db, cleanup := openAdminUserStubDB(t, adminUserStubConfig{
		usersByID: map[int64]*models.User{
			2: {ID: 2, Username: "member", Role: "user", Status: "active"},
		},
	})
	defer cleanup()

	origDB := database.DB
	database.DB = db
	defer func() { database.DB = origDB }()

	h := handlers.NewAdminHandler()
	r := newAdminUserTestRouter(h, 1)

	form := url.Values{"role": {"admin"}, "q": {"成员 昵称"}, "page": {"2"}}
	req := httptest.NewRequest(http.MethodPost, "/admin/users/2/role", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusFound {
		t.Fatalf("status = %d, want %d, body=%s", w.Code, http.StatusFound, w.Body.String())
	}
	if location := w.Header().Get("Location"); location != "/admin/users?page=2&q=%E6%88%90%E5%91%98+%E6%98%B5%E7%A7%B0" {
		t.Fatalf("redirect location = %q, want filtered page", location)
	}
}

func TestAdminUsersResetPasswordRedirectsOnSuccess(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db, cleanup := openAdminUserStubDB(t, adminUserStubConfig{
		usersByID: map[int64]*models.User{
			2: {ID: 2, Username: "member", Role: "user", Status: "active"},
		},
	})
	defer cleanup()

	origDB := database.DB
	database.DB = db
	defer func() { database.DB = origDB }()

	h := handlers.NewAdminHandler()
	r := newAdminUserTestRouter(h, 1)

	form := url.Values{"new_password": {"new-secret-123"}}
	req := httptest.NewRequest(http.MethodPost, "/admin/users/2/password", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusFound {
		t.Fatalf("status = %d, want %d, body=%s", w.Code, http.StatusFound, w.Body.String())
	}
	if location := w.Header().Get("Location"); location != "/admin/users" {
		t.Fatalf("redirect location = %q, want /admin/users", location)
	}
}

func TestAdminUsersUpdateUserStatusRendersErrorOnSelfBlock(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db, cleanup := openAdminUserStubDB(t, adminUserStubConfig{
		allUsers: []*models.User{
			{ID: 1, Username: "admin", Role: "admin", Status: "active", CreatedAt: time.Date(2026, 4, 29, 10, 0, 0, 0, time.UTC)},
			{ID: 2, Username: "member", Role: "user", Status: "active", CreatedAt: time.Date(2026, 4, 29, 11, 0, 0, 0, time.UTC)},
		},
	})
	defer cleanup()

	origDB := database.DB
	database.DB = db
	defer func() { database.DB = origDB }()

	h := handlers.NewAdminHandler()
	r := newAdminUserTestRouter(h, 1)

	form := url.Values{"status": {"blocked"}, "q": {"member"}, "page": {"1"}}
	req := httptest.NewRequest(http.MethodPost, "/admin/users/1/status", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", w.Code, http.StatusOK, w.Body.String())
	}
	body := w.Body.String()
	for _, needle := range []string{"admin-alert-error", "用户管理", "member"} {
		if !strings.Contains(body, needle) {
			t.Fatalf("response missing %q: %s", needle, body)
		}
	}
}

func TestLoginShowsBlockedMessageForBlockedUsers(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db, cleanup := openAdminUserStubDB(t, adminUserStubConfig{
		usersByUsername: map[string]*models.User{
			"blocked-user": {
				ID:           7,
				Username:     "blocked-user",
				PasswordHash: "ignored",
				Role:         "user",
				Status:       "blocked",
			},
		},
	})
	defer cleanup()

	origDB := database.DB
	database.DB = db
	defer func() { database.DB = origDB }()

	h := handlers.NewAPIHandler()
	r := gin.New()
	r.SetHTMLTemplate(web.BuildSharedTemplates(os.DirFS("../templates")))
	r.POST("/api/auth/login", h.Login)

	form := url.Values{
		"username": {"blocked-user"},
		"password": {"secret123"},
	}
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if !strings.Contains(w.Body.String(), "账号已被禁用") {
		t.Fatalf("expected blocked login message, got body: %s", w.Body.String())
	}
}

func TestLoginKeepsGenericMessageForInvalidPassword(t *testing.T) {
	gin.SetMode(gin.TestMode)

	hash, err := bcrypt.GenerateFromPassword([]byte("secret123"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("GenerateFromPassword failed: %v", err)
	}

	db, cleanup := openAdminUserStubDB(t, adminUserStubConfig{
		usersByUsername: map[string]*models.User{
			"active-user": {
				ID:           8,
				Username:     "active-user",
				PasswordHash: string(hash),
				Role:         "user",
				Status:       "active",
			},
		},
	})
	defer cleanup()

	origDB := database.DB
	database.DB = db
	defer func() { database.DB = origDB }()

	h := handlers.NewAPIHandler()
	r := gin.New()
	r.SetHTMLTemplate(web.BuildSharedTemplates(os.DirFS("../templates")))
	r.POST("/api/auth/login", h.Login)

	form := url.Values{
		"username": {"active-user"},
		"password": {"wrong-password"},
	}
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if !strings.Contains(w.Body.String(), "用户名或密码错误") {
		t.Fatalf("expected generic login message, got body: %s", w.Body.String())
	}
}

func newAdminUserTestRouter(h *handlers.AdminHandler, actorUserID int64) *gin.Engine {
	r := gin.New()
	r.SetHTMLTemplate(web.BuildSharedTemplates(os.DirFS("../templates")))
	r.Use(func(c *gin.Context) {
		c.Set("user_id", actorUserID)
		c.Set("username", "admin")
		c.Set("SiteName", "AI Later")
		c.Next()
	})
	r.GET("/admin/users", h.AdminUsers)
	r.POST("/admin/users/:id/role", h.AdminUpdateUserRole)
	r.POST("/admin/users/:id/password", h.AdminResetUserPassword)
	r.POST("/admin/users/:id/status", h.AdminUpdateUserStatus)
	return r
}

type adminUserStubConfig struct {
	allUsers        []*models.User
	usersByID       map[int64]*models.User
	usersByUsername map[string]*models.User
}

type adminUserStubDriver struct {
	config adminUserStubConfig
}

type adminUserStubConn struct {
	config adminUserStubConfig
}

type adminUserStubRows struct {
	columns []string
	values  [][]driver.Value
	idx     int
}

var adminUserStubCounter int64

func openAdminUserStubDB(t *testing.T, config adminUserStubConfig) (*sql.DB, func()) {
	t.Helper()

	driverName := fmt.Sprintf("admin-user-stub-%d", atomic.AddInt64(&adminUserStubCounter, 1))
	sql.Register(driverName, &adminUserStubDriver{config: config})

	db, err := sql.Open(driverName, "")
	if err != nil {
		t.Fatalf("open stub db: %v", err)
	}

	cleanup := func() {
		if err := db.Close(); err != nil {
			t.Fatalf("close stub db: %v", err)
		}
	}

	return db, cleanup
}

func (d *adminUserStubDriver) Open(string) (driver.Conn, error) {
	return &adminUserStubConn{config: d.config}, nil
}

func (c *adminUserStubConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("prepare not implemented")
}

func (c *adminUserStubConn) Close() error {
	return nil
}

func (c *adminUserStubConn) Begin() (driver.Tx, error) {
	return nil, errors.New("transactions not implemented")
}

func (c *adminUserStubConn) ExecContext(_ context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	switch {
	case strings.Contains(query, "UPDATE users SET role"):
		if len(args) != 2 {
			return nil, fmt.Errorf("expected 2 args for role update, got %d", len(args))
		}
		return driver.RowsAffected(1), nil
	case strings.Contains(query, "UPDATE users SET password_hash"):
		if len(args) != 2 {
			return nil, fmt.Errorf("expected 2 args for password update, got %d", len(args))
		}
		return driver.RowsAffected(1), nil
	case strings.Contains(query, "UPDATE users SET status"):
		if len(args) != 2 {
			return nil, fmt.Errorf("expected 2 args for status update, got %d", len(args))
		}
		return driver.RowsAffected(1), nil
	default:
		return nil, fmt.Errorf("unexpected exec query: %s", query)
	}
}

func (c *adminUserStubConn) QueryContext(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	switch {
	case strings.Contains(query, "SELECT COUNT(*) FROM site_submissions"):
		return &adminUserStubRows{columns: []string{"count"}, values: [][]driver.Value{{int64(0)}}}, nil
	case strings.Contains(query, "SELECT COUNT(*) FROM users"):
		return &adminUserStubRows{columns: []string{"count"}, values: [][]driver.Value{{int64(len(c.config.allUsers))}}}, nil
	case strings.Contains(query, "FROM users") && strings.Contains(query, "LIMIT ? OFFSET ?"):
		users := c.config.allUsers
		if len(args) == 5 {
			queryText := strings.Trim(args[0].Value.(string), "%")
			filtered := make([]*models.User, 0, len(users))
			for _, user := range users {
				if strings.Contains(user.Username, queryText) || strings.Contains(user.Nickname, queryText) || strings.Contains(user.Email, queryText) {
					filtered = append(filtered, user)
				}
			}
			users = filtered
		}
		return newAdminUserPageRows(users), nil
	case strings.Contains(query, "FROM oauth_identities oi"):
		return &adminUserStubRows{columns: []string{"user_id", "provider", "name", "type", "preferred_username", "display_name", "picture", "email"}}, nil
	case strings.Contains(query, "FROM users WHERE id = ?"):
		if len(args) != 1 {
			return nil, fmt.Errorf("expected 1 arg for user by id, got %d", len(args))
		}
		userID, ok := args[0].Value.(int64)
		if !ok {
			return nil, fmt.Errorf("unexpected user id arg type %T", args[0].Value)
		}
		if c.config.usersByID == nil || c.config.usersByID[userID] == nil {
			return newAdminUserRows(nil), nil
		}
		return newAdminUserRows([]*models.User{c.config.usersByID[userID]}), nil
	case strings.Contains(query, "FROM users WHERE username = ?"):
		if len(args) != 1 {
			return nil, fmt.Errorf("expected 1 arg for user by username, got %d", len(args))
		}
		username, ok := args[0].Value.(string)
		if !ok {
			return nil, fmt.Errorf("unexpected username arg type %T", args[0].Value)
		}
		if c.config.usersByUsername == nil || c.config.usersByUsername[username] == nil {
			return newAdminUserRows(nil), nil
		}
		return newAdminUserRows([]*models.User{c.config.usersByUsername[username]}), nil
	default:
		return nil, fmt.Errorf("unexpected query: %s", query)
	}
}

func newAdminUserPageRows(users []*models.User) *adminUserStubRows {
	rows := &adminUserStubRows{
		columns: []string{"id", "username", "nickname", "email", "role", "status", "created_at", "updated_at"},
	}
	for _, user := range users {
		now := time.Date(2026, 4, 29, 12, 0, 0, 0, time.UTC)
		createdAt := user.CreatedAt
		if createdAt.IsZero() {
			createdAt = now
		}
		updatedAt := user.UpdatedAt
		if updatedAt.IsZero() {
			updatedAt = now
		}
		rows.values = append(rows.values, []driver.Value{
			user.ID, user.Username, user.Nickname, user.Email, user.Role, user.Status, createdAt, updatedAt,
		})
	}
	return rows
}

func newAdminUserRows(users []*models.User) *adminUserStubRows {
	rows := &adminUserStubRows{
		columns: []string{"id", "username", "nickname", "email", "password_hash", "role", "status", "created_at", "updated_at"},
	}
	for _, user := range users {
		now := time.Date(2026, 4, 29, 12, 0, 0, 0, time.UTC)
		createdAt := user.CreatedAt
		if createdAt.IsZero() {
			createdAt = now
		}
		updatedAt := user.UpdatedAt
		if updatedAt.IsZero() {
			updatedAt = now
		}
		rows.values = append(rows.values, []driver.Value{
			user.ID,
			user.Username,
			user.Nickname,
			user.Email,
			user.PasswordHash,
			user.Role,
			user.Status,
			createdAt,
			updatedAt,
		})
	}
	return rows
}

func (r *adminUserStubRows) Columns() []string { return r.columns }
func (r *adminUserStubRows) Close() error      { return nil }
func (r *adminUserStubRows) Next(dest []driver.Value) error {
	if r.idx >= len(r.values) {
		return io.EOF
	}
	copy(dest, r.values[r.idx])
	r.idx++
	return nil
}

var _ driver.Conn = (*adminUserStubConn)(nil)
var _ driver.Driver = (*adminUserStubDriver)(nil)
var _ driver.ExecerContext = (*adminUserStubConn)(nil)
var _ driver.QueryerContext = (*adminUserStubConn)(nil)
var _ driver.Rows = (*adminUserStubRows)(nil)
