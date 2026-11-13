package web

import (
	"html/template"
	"io/fs"
	"net/url"
	"strconv"
	"strings"
)

var funcMap = template.FuncMap{
	"upper":       strings.ToUpper,
	"lower":       strings.ToLower,
	"add":         func(a, b int) int { return a + b },
	"sub":         func(a, b int) int { return a - b },
	"categoryURL": func(name string) string { return "/search?category=" + url.QueryEscape(name) },
	"adminPageURL": func(basePath, query string, page int) string {
		params := url.Values{}
		if query != "" {
			params.Set("q", query)
		}
		if page > 1 {
			params.Set("page", strconv.Itoa(page))
		}
		if encoded := params.Encode(); encoded != "" {
			return basePath + "?" + encoded
		}
		return basePath
	},
	"userPageURL": func(query string, page int) string {
		params := url.Values{}
		if query != "" {
			params.Set("q", query)
		}
		if page > 1 {
			params.Set("page", strconv.Itoa(page))
		}
		if encoded := params.Encode(); encoded != "" {
			return "/admin/users?" + encoded
		}
		return "/admin/users"
	},
	// pageNumbers returns a compact, bounded window around the current page.
	"pageNumbers": func(current, total int) []int {
		start := current - 2
		if start < 1 {
			start = 1
		}
		end := start + 4
		if end > total {
			end = total
		}
		start = end - 4
		if start < 1 {
			start = 1
		}
		pages := make([]int, 0, end-start+1)
		for page := start; page <= end; page++ {
			pages = append(pages, page)
		}
		return pages
	},
	// categoryIconData 分类图标：管理员显式设置的优先，否则按名称关键词自动匹配。
	"categoryIconData": func(name, icon string) map[string]string {
		effective := ""
		switch icon {
		case "grid", "chat", "create", "code", "tool", "star", "link":
			effective = icon
		}
		if effective == "" {
			switch {
			case strings.Contains(name, "对话"), strings.Contains(name, "聊天"), strings.Contains(name, "助手"):
				effective = "chat"
			case strings.Contains(name, "编程"), strings.Contains(name, "代码"), strings.Contains(name, "开发"):
				effective = "code"
			case strings.Contains(name, "创作"), strings.Contains(name, "绘画"), strings.Contains(name, "图像"), strings.Contains(name, "设计"):
				effective = "create"
			default:
				effective = "tool"
			}
		}
		return map[string]string{"Icon": effective}
	},
	// iconPicker 组装可视化图标选择器的数据：当前值、自动匹配预览名、可选项。
	"iconPicker": func(current, previewName string, options interface{}) map[string]interface{} {
		return map[string]interface{}{"Current": current, "PreviewName": previewName, "Options": options}
	},
	// splitTags 把逗号分隔的标签字符串拆成去空去重的切片，供审核页展示。
	"splitTags": func(raw string) []string {
		seen := make(map[string]struct{})
		var tags []string
		for _, t := range strings.Split(raw, ",") {
			t = strings.TrimSpace(t)
			if t == "" {
				continue
			}
			if _, ok := seen[t]; ok {
				continue
			}
			seen[t] = struct{}{}
			tags = append(tags, t)
		}
		return tags
	},
}

func BuildPageTemplates(root fs.FS) map[string]*template.Template {
	return map[string]*template.Template{
		"index.html":       template.Must(template.New("index.html").Funcs(funcMap).ParseFS(root, "layout.html", "index.html", "partials/*.html")),
		"leaderboard.html": template.Must(template.New("leaderboard.html").Funcs(funcMap).ParseFS(root, "layout.html", "leaderboard.html", "partials/*.html")),
		"login.html":       template.Must(template.New("login.html").Funcs(funcMap).ParseFS(root, "layout.html", "login.html", "partials/*.html")),
		"register.html":    template.Must(template.New("register.html").Funcs(funcMap).ParseFS(root, "layout.html", "register.html", "partials/*.html")),
		"dashboard.html":   template.Must(template.New("dashboard.html").Funcs(funcMap).ParseFS(root, "layout.html", "dashboard.html", "partials/*.html")),
		"submit.html":      template.Must(template.New("submit.html").Funcs(funcMap).ParseFS(root, "layout.html", "submit.html", "partials/*.html")),
		"setup.html":       template.Must(template.New("setup.html").Funcs(funcMap).ParseFS(root, "layout.html", "setup.html", "partials/*.html")),
	}
}

func BuildSharedTemplates(root fs.FS) *template.Template {
	return template.Must(template.New("shared.html").Funcs(funcMap).ParseFS(root, "error.html", "admin/admin-index.html", "admin/admin-sites.html", "admin/admin-add-site.html", "admin/admin-edit-site.html", "admin/admin-nav.html", "admin/admin-nav-form.html", "admin/admin-users.html", "admin/admin-settings.html", "admin/admin-providers.html", "admin/admin-provider-edit.html", "admin/admin-stats.html", "admin/admin-submissions.html", "admin/admin-categories.html", "admin/admin-tags.html", "admin/admin-friend-links.html", "admin/admin-friend-link-form.html", "admin/admin-cron-tasks.html", "partials/*.html"))
}
