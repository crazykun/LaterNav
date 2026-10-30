package test

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"ai-later-nav/internal/models"
	"ai-later-nav/internal/web"
)

func TestAdminSitesTemplateRendersFilterAndFeaturedToggle(t *testing.T) {
	tmpl := web.BuildSharedTemplates(os.DirFS("../templates"))

	var out bytes.Buffer
	err := tmpl.ExecuteTemplate(&out, "admin-sites.html", map[string]any{
		"username":       "admin",
		"adminSection":   "sites",
		"query":          "chatgpt",
		"featuredFilter": "1",
		"toggleQuery":    "?q=chatgpt&featured=1",
		"sites": []models.SiteDisplay{
			{Site: models.Site{ID: 1, Name: "ChatGPT", Featured: true}, DisplayTags: []models.DisplayTag{{Name: "AI", Class: "bg-blue-50"}}},
			{Site: models.Site{ID: 2, Name: "Other", Featured: false}},
		},
	})
	if err != nil {
		t.Fatalf("ExecuteTemplate failed: %v", err)
	}

	html := out.String()
	if !strings.Contains(html, `value="chatgpt"`) || !strings.Contains(html, `name="featured"`) {
		t.Fatalf("admin-sites.html did not render filter form")
	}
	if !strings.Contains(html, `value="1" selected`) {
		t.Fatalf("admin-sites.html did not keep featured filter selected")
	}
	if !strings.Contains(html, `href="/admin/sites/toggle-featured/1?q=chatgpt&amp;featured=1"`) {
		t.Fatalf("admin-sites.html did not preserve list query in toggle link")
	}
	if !strings.Contains(html, "★ 已推荐") || !strings.Contains(html, "☆ 未推荐") {
		t.Fatalf("admin-sites.html did not render featured badges")
	}
	if !strings.Contains(html, "点击取消推荐") || !strings.Contains(html, "点击设为推荐") {
		t.Fatalf("admin-sites.html did not render toggle hints")
	}
}

func TestAdminSitesTemplateWorksWithoutFilterData(t *testing.T) {
	tmpl := web.BuildSharedTemplates(os.DirFS("../templates"))

	var out bytes.Buffer
	err := tmpl.ExecuteTemplate(&out, "admin-sites.html", map[string]any{
		"username":     "admin",
		"adminSection": "sites",
		"sites":        []models.SiteDisplay{},
	})
	if err != nil {
		t.Fatalf("ExecuteTemplate failed without filter data: %v", err)
	}
	if strings.Contains(out.String(), "已推荐") {
		t.Fatalf("unexpected featured badge without sites")
	}
}
