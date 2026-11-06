package handlers

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestProfileRejectsMissingCSRF(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/profile", func(c *gin.Context) { c.Set("user_id", int64(1)); UpdateProfile(c) })
	form := url.Values{"username": {"alice"}, "nickname": {"Alice"}}
	req := httptest.NewRequest(http.MethodPost, "/profile", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("got %d, want 403", w.Code)
	}
}
