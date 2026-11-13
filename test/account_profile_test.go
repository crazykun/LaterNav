package test

import (
	"bytes"
	"os"
	"strings"
	"testing"
	"time"

	"ai-later-nav/internal/web"
)

func TestAccountProfileRendersSectionsAndEscapesUserData(t *testing.T) {
	tmpl := web.BuildPageTemplates(os.DirFS("../templates"))["account-profile.html"]
	if tmpl == nil {
		t.Fatal("account-profile.html not registered")
	}
	var out bytes.Buffer
	data := map[string]any{"username": "alice", "nickname": "<img src=x>", "displayName": "Alice", "csrfToken": "binding-token", "availableProviders": []any{map[string]any{"Slug": "github", "Name": "GitHub"}}, "profileCSRFToken": "profile-token", "passwordCSRFToken": "profile-token", "identities": []any{}, "canUnbind": true}
	if err := tmpl.ExecuteTemplate(&out, "account-profile.html", data); err != nil {
		t.Fatal(err)
	}
	html := out.String()
	for _, section := range []string{"修改头像", "修改昵称", "修改密码", "第三方账号", "id=\"account-bindings-list\"", "id=\"password-form\"", "profile-token", "binding-token"} {
		if !strings.Contains(html, section) {
			t.Errorf("missing %q", section)
		}
	}
	if strings.Contains(html, "<img src=x>") {
		t.Error("nickname rendered without escaping")
	}
}

func TestPublicPagesRenderSingleBackToTopAndVersionedScript(t *testing.T) {
	templates := web.BuildPageTemplates(os.DirFS("../templates"))
	for _, name := range []string{"index.html", "leaderboard.html", "dashboard.html", "account-profile.html"} {
		tmpl := templates[name]
		if tmpl == nil {
			t.Errorf("missing %s", name)
			continue
		}
		var out bytes.Buffer
		data := map[string]any{"sites": []any{}, "identities": []any{}, "board": map[string]any{"GeneratedAt": time.Time{}, "Models": []any{}}}
		if err := tmpl.ExecuteTemplate(&out, name, data); err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if n := strings.Count(out.String(), "id=\"back-to-top\""); n != 1 {
			t.Errorf("%s has %d scroll buttons, want one", name, n)
		}
		if !strings.Contains(out.String(), "/static/js/main.js?v=") {
			t.Errorf("%s missing versioned scroll script", name)
		}
	}
}
