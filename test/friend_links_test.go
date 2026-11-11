package test

import (
	"bytes"
	"os"
	"strings"
	"testing"
	"time"

	"ai-later-nav/internal/models"
	"ai-later-nav/internal/web"
)

func friendLinkFixture() []models.FriendLink {
	return []models.FriendLink{
		{ID: 1, Name: "示例博客", URL: "https://blog.example.com", Description: "技术与生活", Logo: "https://blog.example.com/logo.png", SortOrder: 10, Enabled: true, Color: "#3b82f6", Initials: "示", CreatedAt: time.Now()},
		{ID: 2, Name: "开发文档", URL: "https://docs.example.com", SortOrder: 20, Enabled: true, Color: "#10b981", Initials: "开", CreatedAt: time.Now()},
	}
}

func TestIndexTemplateRendersFriendLinks(t *testing.T) {
	tmpl := web.BuildPageTemplates(os.DirFS("../templates"))["index.html"]

	var out bytes.Buffer
	err := tmpl.ExecuteTemplate(&out, "index.html", map[string]any{
		"sites":       []any{},
		"friendLinks": friendLinkFixture(),
	})
	if err != nil {
		t.Fatalf("ExecuteTemplate failed: %v", err)
	}
	html := out.String()

	if !strings.Contains(html, `id="friend-links-title"`) || !strings.Contains(html, "友情链接") {
		t.Fatalf("index.html did not render friend links section: %s", html)
	}
	if !strings.Contains(html, `href="https://blog.example.com" target="_blank" rel="noopener noreferrer"`) {
		t.Fatalf("friend link missing safe external attributes: %s", html)
	}
	if !strings.Contains(html, "技术与生活") || !strings.Contains(html, "开发文档") {
		t.Fatalf("friend link names/descriptions missing: %s", html)
	}
	// 有 Logo 的项含图片回退，无 Logo 的项显示名称配色首字头像。
	if !strings.Contains(html, `src="https://blog.example.com/logo.png"`) {
		t.Fatalf("friend link logo missing: %s", html)
	}
	if !strings.Contains(html, `style="background-color: #10b981"`) || !strings.Contains(html, ">开</span>") {
		t.Fatalf("colored initials avatar missing: %s", html)
	}
	if !strings.Contains(html, `aria-labelledby="friend-links-title"`) || !strings.Contains(html, "2 个站点") {
		t.Fatalf("friend links header/count missing: %s", html)
	}
}

func TestIndexTemplateHidesFriendLinksWhenAbsent(t *testing.T) {
	tmpl := web.BuildPageTemplates(os.DirFS("../templates"))["index.html"]

	// 模拟搜索页：不传 friendLinks 时整个区块不渲染。
	var out bytes.Buffer
	if err := tmpl.ExecuteTemplate(&out, "index.html", map[string]any{
		"sites":            []any{},
		"query":            "",
		"selectedCategory": "AI对话",
	}); err != nil {
		t.Fatalf("ExecuteTemplate without friendLinks failed: %v", err)
	}
	if strings.Contains(out.String(), "友情链接") {
		t.Fatalf("friend links section should be hidden on search page: %s", out.String())
	}

	out.Reset()
	if err := tmpl.ExecuteTemplate(&out, "index.html", map[string]any{
		"sites":       []any{},
		"friendLinks": []models.FriendLink{},
	}); err != nil {
		t.Fatalf("ExecuteTemplate with empty friendLinks failed: %v", err)
	}
	if strings.Contains(out.String(), "友情链接") {
		t.Fatalf("empty friend links should render nothing: %s", out.String())
	}
}

func TestAdminFriendLinkTemplatesRender(t *testing.T) {
	tmpl := web.BuildSharedTemplates(os.DirFS("../templates"))

	var list bytes.Buffer
	links := append(friendLinkFixture(), models.FriendLink{ID: 3, Name: "已停用站点", URL: "https://off.example.com", Enabled: false})
	err := tmpl.ExecuteTemplate(&list, "admin-friend-links.html", map[string]any{
		"adminSection": "friend-links",
		"links":        links,
		"csrfToken":    "csrf",
	})
	if err != nil {
		t.Fatalf("admin-friend-links.html failed: %v", err)
	}
	listHTML := list.String()
	for _, want := range []string{
		"/admin/friend-links/add",
		"/admin/friend-links/1/toggle",
		"/admin/friend-links/1/move",
		"/admin/friend-links/edit/1",
		"/admin/friend-links/1/delete",
		`name="csrf_token" value="csrf"`,
		"admin-badge-danger",
	} {
		if !strings.Contains(listHTML, want) {
			t.Errorf("admin friend link list missing %q: %s", want, listHTML)
		}
	}

	var form bytes.Buffer
	err = tmpl.ExecuteTemplate(&form, "admin-friend-link-form.html", map[string]any{
		"adminSection": "friend-links",
		"formTitle":    "编辑友链",
		"formAction":   "/admin/friend-links/edit/1",
		"item":         friendLinkFixture()[0],
		"csrfToken":    "csrf",
	})
	if err != nil {
		t.Fatalf("admin-friend-link-form.html failed: %v", err)
	}
	formHTML := form.String()
	for _, want := range []string{
		`action="/admin/friend-links/edit/1"`,
		`value="https://blog.example.com"`,
		`value="https://blog.example.com/logo.png"`,
		`name="Enabled" checked`,
		`value="10"`,
	} {
		if !strings.Contains(formHTML, want) {
			t.Errorf("admin friend link form missing %q: %s", want, formHTML)
		}
	}
}
