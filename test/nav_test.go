package test

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"ai-later-nav/internal/models"
	"ai-later-nav/internal/web"
)

func TestIndexTemplateRendersConfiguredNavItems(t *testing.T) {
	tmpl := web.BuildPageTemplates(os.DirFS("../templates"))["index.html"]

	var out bytes.Buffer
	err := tmpl.ExecuteTemplate(&out, "index.html", map[string]any{
		"sites": []any{},
		"navItems": []models.NavItem{
			{Name: "测试导航", URL: "/search?category=code", Icon: "code"},
			{Name: "空图标项", URL: "/x", Icon: ""},
		},
		"isLoggedIn": false,
		"username":   "",
	})
	if err != nil {
		t.Fatalf("ExecuteTemplate failed: %v", err)
	}

	html := out.String()
	if !strings.Contains(html, `href="/search?category=code"`) {
		t.Fatalf("index.html did not render configured nav url: %s", html)
	}
	if !strings.Contains(html, "测试导航") {
		t.Fatalf("index.html did not render configured nav name")
	}
	if !strings.Contains(html, `href="/search?category=AI对话"`) {
		t.Fatalf("index.html lost the fixed left category sidebar")
	}
}

func TestIndexTemplateHandlesMissingNavItems(t *testing.T) {
	tmpl := web.BuildPageTemplates(os.DirFS("../templates"))["index.html"]

	var out bytes.Buffer
	err := tmpl.ExecuteTemplate(&out, "index.html", map[string]any{
		"sites":      []any{},
		"isLoggedIn": false,
		"username":   "",
	})
	if err != nil {
		t.Fatalf("ExecuteTemplate failed without navItems data: %v", err)
	}
}

func TestAdminNavTemplatesRender(t *testing.T) {
	tmpl := web.BuildSharedTemplates(os.DirFS("../templates"))

	var list bytes.Buffer
	if err := tmpl.ExecuteTemplate(&list, "admin-nav.html", map[string]any{
		"username":     "admin",
		"adminSection": "nav",
		"items": []models.NavItem{
			{ID: 1, Name: "对话", URL: "/search?category=AI对话", Icon: "chat", SortOrder: 2, Enabled: true},
			{ID: 2, Name: "隐藏项", URL: "/", Icon: "grid", SortOrder: 3, Enabled: false},
		},
	}); err != nil {
		t.Fatalf("admin-nav.html ExecuteTemplate failed: %v", err)
	}
	listHTML := list.String()
	if !strings.Contains(listHTML, "/admin/nav/edit/1") || !strings.Contains(listHTML, "对话") {
		t.Fatalf("admin-nav.html did not render nav items")
	}
	if !strings.Contains(listHTML, "停用") {
		t.Fatalf("admin-nav.html did not render disabled badge")
	}

	var form bytes.Buffer
	if err := tmpl.ExecuteTemplate(&form, "admin-nav-form.html", map[string]any{
		"username":     "admin",
		"adminSection": "nav",
		"formTitle":    "编辑导航",
		"formAction":   "/admin/nav/edit/1",
		"item":         models.NavItem{ID: 1, Name: "对话", URL: "/search?category=AI对话", Icon: "chat", SortOrder: 2, Enabled: false},
		"iconOptions": []map[string]any{
			{"Value": "grid", "Label": "网格（grid）"},
			{"Value": "chat", "Label": "对话（chat）"},
		},
	}); err != nil {
		t.Fatalf("admin-nav-form.html ExecuteTemplate failed: %v", err)
	}
	formHTML := form.String()
	if !strings.Contains(formHTML, `value="对话"`) || !strings.Contains(formHTML, `action="/admin/nav/edit/1"`) {
		t.Fatalf("admin-nav-form.html did not render item values")
	}
	if !strings.Contains(formHTML, `value="chat" selected`) {
		t.Fatalf("admin-nav-form.html did not select current icon")
	}
}
