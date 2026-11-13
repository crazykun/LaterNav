package handlers

import (
	"ai-later-nav/internal/models"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
)

type leaderboardReader interface {
	Current() (models.Leaderboard, error)
}

type LeaderboardPageHandler struct {
	page   *PageHandler
	reader leaderboardReader
}

func NewLeaderboardPageHandler(page *PageHandler, reader leaderboardReader) *LeaderboardPageHandler {
	return &LeaderboardPageHandler{page: page, reader: reader}
}

func (h *LeaderboardPageHandler) LeaderboardPage(c *gin.Context) {
	board, err := h.reader.Current()
	if err != nil {
		log.Printf("load leaderboard: %v", err)
		c.String(http.StatusInternalServerError, "加载排行榜失败")
		return
	}
	if err := h.page.renderPage(c, "leaderboard.html", gin.H{"board": board}); err != nil {
		log.Printf("render leaderboard: %v", err)
		c.String(http.StatusInternalServerError, "排行榜渲染失败")
	}
}
