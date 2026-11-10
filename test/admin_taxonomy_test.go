package test

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"ai-later-nav/internal/models"
	"ai-later-nav/internal/web"
)

func TestAdminCategoryAndTagTemplatesRender(t *testing.T) {
	tmpl := web.BuildSharedTemplates(os.DirFS("../templates"))
	common := func() map[string]any {
		return map[string]any{
			"username": "admin", "csrfToken": "csrf", "query": "AI", "basePath": "/admin/categories",
			"total": int64(2), "page": 1, "pageSize": 20, "totalPages": 1,
		}
	}
	data := common()
	data["categories"] = []models.CategoryUsage{
		{Category: models.Category{ID: 1, Name: "AI对话", SortOrder: 10}, SiteCount: 5},
		{Category: models.Category{ID: 2, Name: "空分类", SortOrder: 20}},
	}
	var out bytes.Buffer
	if err := tmpl.ExecuteTemplate(&out, "admin-categories.html", data); err != nil {
		t.Fatal(err)
	}
	html := out.String()
	for _, want := range []string{"/admin/categories/add", "/admin/categories/1/rename", "/admin/categories/1/move", "使用中，先迁移站点", "/admin/categories/2/delete", `name="csrf_token" value="csrf"`} {
		if !strings.Contains(html, want) {
			t.Errorf("category page missing %q: %s", want, html)
		}
	}
	if strings.Contains(html, "/admin/categories/1/delete") {
		t.Error("in-use category should not render delete action")
	}

	data = common()
	data["basePath"] = "/admin/tags"
	data["tags"] = []models.TagUsage{{Tag: models.Tag{ID: 1, Name: "AI"}, SiteCount: 5}}
	data["allTags"] = []models.Tag{{ID: 1, Name: "AI"}, {ID: 2, Name: "工具"}, {ID: 3, Name: "开源"}}
	out.Reset()
	if err := tmpl.ExecuteTemplate(&out, "admin-tags.html", data); err != nil {
		t.Fatal(err)
	}
	html = out.String()
	for _, want := range []string{"/admin/tags/add", "/admin/tags/1/rename", "/admin/tags/1/merge", "/admin/tags/1/delete", `<option value="2">工具</option>`, `<option value="3">开源</option>`} {
		if !strings.Contains(html, want) {
			t.Errorf("tag page missing %q: %s", want, html)
		}
	}
	if strings.Contains(html, `>AI</option>`) {
		t.Error("source tag must not appear in its own merge targets")
	}
}
