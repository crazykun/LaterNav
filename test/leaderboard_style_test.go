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

func renderLeaderboard(t *testing.T, board models.Leaderboard) string {
	t.Helper()
	tmpl := web.BuildPageTemplates(os.DirFS("../templates"))["leaderboard.html"]
	if tmpl == nil {
		t.Fatal("leaderboard template not registered")
	}
	var out bytes.Buffer
	if err := tmpl.ExecuteTemplate(&out, "leaderboard.html", map[string]any{"board": board, "SiteName": "LaterNav"}); err != nil {
		t.Fatal(err)
	}
	return out.String()
}

func TestLeaderboardStyledRowsKeepEvidenceAndSource(t *testing.T) {
	board := models.Leaderboard{
		Models: []models.RankedModel{
			{Rank: 1, Name: "<Model A>", Creator: "Maker A", Score: 97.45, EvidenceStatus: "estimated", URL: "https://benchlm.ai/model/a"},
			{Rank: 2, Name: "Model B", Creator: "Maker B", Score: 93, EvidenceStatus: "supported", URL: "https://benchlm.ai/model/b"},
		},
		GeneratedAt: time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC),
	}
	html := renderLeaderboard(t, board)
	for _, want := range []string{
		"aria-label=\"面包屑导航\"", "aria-current=\"page\"", "overflow-x-auto", "<caption", "scope=\"col\"",
		"排名", "模型", "厂商", "综合分", "证据", "97.45", "93.00", "估算", "有证据支持",
		"BenchLM.ai", "评分方法", "CC BY-NC 4.0", "datetime=\"2026-11-01T12:00:00Z\"",
		"href=\"https://benchlm.ai/model/a\"", "rel=\"noopener noreferrer\"", "min-w-[640px]", "rounded-2xl",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("rendered leaderboard missing %q", want)
		}
	}
	if strings.Contains(html, "<Model A>") || !strings.Contains(html, "&lt;Model A&gt;") {
		t.Error("model name must be HTML escaped")
	}
}

func TestLeaderboardEmptyStateRetainsSourceAndSyncInstruction(t *testing.T) {
	html := renderLeaderboard(t, models.Leaderboard{})
	for _, want := range []string{"role=\"status\"", "榜单数据暂未就绪", "定时任务", "BenchLM.ai", "CC BY-NC 4.0"} {
		if !strings.Contains(html, want) {
			t.Errorf("empty leaderboard missing %q", want)
		}
	}
}
