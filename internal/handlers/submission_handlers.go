package handlers

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log"
	"net/http"

	"ai-later-nav/internal/config"
	"ai-later-nav/internal/services"
	"github.com/gin-gonic/gin"
)

func submissionCSRF(c *gin.Context) string {
	token, err := c.Cookie("token")
	if err != nil || token == "" {
		return ""
	}
	mac := hmac.New(sha256.New, []byte(config.AppConfig.JWT.Secret))
	mac.Write([]byte("user-submission:" + token))
	return hex.EncodeToString(mac.Sum(nil))
}

// SubmitPage 投稿页，仅登录会员可见（挂在 AuthMiddleware 之后）。
func (h *PageHandler) SubmitPage(c *gin.Context) {
	categories, _ := h.siteService.GetCategories()
	if err := h.renderPage(c, "submit.html", gin.H{
		"csrfToken":  submissionCSRF(c),
		"categories": categories,
	}); err != nil {
		c.HTML(http.StatusInternalServerError, "error.html", gin.H{
			"error": "页面渲染失败",
		})
	}
}

// CreateSubmission 处理会员投稿（HTMX），失败重渲染表单 partial 并回填，成功展示提示。
func CreateSubmission(c *gin.Context) {
	userID := c.GetInt64("user_id")
	if userID <= 0 {
		c.Header("HX-Redirect", "/login")
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	expected := submissionCSRF(c)
	if expected == "" || !hmac.Equal([]byte(expected), []byte(c.PostForm("csrf_token"))) {
		c.AbortWithStatus(http.StatusForbidden)
		return
	}

	form := gin.H{
		"Name":        c.PostForm("name"),
		"URL":         c.PostForm("url"),
		"Description": c.PostForm("description"),
		"Logo":        c.PostForm("logo"),
		"Category":    c.PostForm("category"),
		"Tags":        c.PostForm("tags"),
	}
	categories, _ := services.NewSiteService().GetCategories()

	_, err := services.NewSiteSubmissionService().Submit(userID,
		form["Name"].(string), form["URL"].(string), form["Description"].(string),
		form["Logo"].(string), form["Category"].(string), form["Tags"].(string))
	if err != nil {
		log.Printf("create submission: %v", err)
		c.HTML(http.StatusOK, "partials/submit-form.html", gin.H{
			"error":      submissionErrorMessage(err),
			"values":     form,
			"csrfToken":  expected,
			"categories": categories,
		})
		return
	}

	// 成功后返回干净表单，方便继续投稿。
	c.HTML(http.StatusOK, "partials/submit-form.html", gin.H{
		"success":    "投稿成功，已进入审核队列，可在个人中心查看进度。",
		"csrfToken":  expected,
		"categories": categories,
	})
}

func submissionErrorMessage(err error) string {
	switch {
	case errors.Is(err, services.ErrInvalidName),
		errors.Is(err, services.ErrInvalidURL),
		errors.Is(err, services.ErrInvalidDescription),
		errors.Is(err, services.ErrInvalidTags),
		errors.Is(err, services.ErrSubmissionExists),
		errors.Is(err, services.ErrAlreadySubmitted),
		errors.Is(err, services.ErrTooManyPending):
		return err.Error()
	default:
		return "提交失败，请稍后重试"
	}
}
