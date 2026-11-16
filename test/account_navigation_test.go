package test

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"ai-later-nav/internal/web"
)

func TestAccountPagesUseSharedHeaderAndBreadcrumbs(t *testing.T) {
	pages := []struct {
		name  string
		title string
		data  map[string]any
	}{
		{"dashboard.html", "个人中心", map[string]any{"favoriteSites": []any{}, "mySubmissions": []any{}}},
		{"account-profile.html", "账号资料", map[string]any{"identities": []any{}, "availableProviders": []any{}, "canUnbind": true}},
		{"submit.html", "投稿站点", map[string]any{}},
	}
	for _, page := range pages {
		t.Run(page.name, func(t *testing.T) {
			var out bytes.Buffer
			page.data["displayName"] = "Alice"
			page.data["isLoggedIn"] = true
			tmpl := web.BuildPageTemplates(os.DirFS("../templates"))[page.name]
			if err := tmpl.ExecuteTemplate(&out, page.name, page.data); err != nil {
				t.Fatal(err)
			}
			html := out.String()
			for _, fragment := range []string{
				`id="account-page-header"`, `aria-label="面包屑导航"`, `aria-label="个人中心导航"`,
				`href="/dashboard"`, `href="/account/profile"`, `href="/submit"`,
				`aria-current="page"`, ">" + page.title + "</h1>",
			} {
				if !strings.Contains(html, fragment) {
					t.Errorf("missing %q", fragment)
				}
			}
			if count := strings.Count(html, `id="account-page-header"`); count != 1 {
				t.Errorf("got %d page headers, want 1", count)
			}
			if count := strings.Count(html, `aria-current="page"`); count != 2 {
				t.Errorf("got %d current page markers, want 2 (breadcrumb + tab)", count)
			}
		})
	}
}
