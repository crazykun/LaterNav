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

func profileCSRF(c *gin.Context) string {
	token, err := c.Cookie("token")
	if err != nil || token == "" {
		return ""
	}
	mac := hmac.New(sha256.New, []byte(config.AppConfig.JWT.Secret))
	mac.Write([]byte("user-profile:" + token))
	return hex.EncodeToString(mac.Sum(nil))
}

// GetProfile and UpdateProfile must be registered behind AuthMiddleware.
func validUserCSRF(c *gin.Context) bool {
	expected := profileCSRF(c)
	return expected != "" && hmac.Equal([]byte(expected), []byte(c.PostForm("csrf_token")))
}
func GetProfile(c *gin.Context) {
	if c.GetInt64("user_id") <= 0 {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	user, err := services.NewUserService().GetByID(c.GetInt64("user_id"))
	if err != nil {
		log.Printf("get profile: %v", err)
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}
	if user == nil {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, gin.H{"username": user.Username, "nickname": user.Nickname, "csrf_token": profileCSRF(c)})
}

func UpdateProfile(c *gin.Context) {
	if c.GetInt64("user_id") <= 0 {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	expected := profileCSRF(c)
	if expected == "" || !hmac.Equal([]byte(expected), []byte(c.PostForm("csrf_token"))) {
		c.AbortWithStatus(http.StatusForbidden)
		return
	}
	err := services.NewUserService().UpdateProfile(c.GetInt64("user_id"), c.PostForm("username"), c.PostForm("nickname"))
	switch {
	case err == nil:
		c.Status(http.StatusNoContent)
	case errors.Is(err, services.ErrUsernameReauthRequired):
		c.String(http.StatusForbidden, "修改登录名需要重新验证身份，暂不可用")
	case errors.Is(err, services.ErrUsernameExists):
		c.String(http.StatusConflict, "登录名已存在")
	case errors.Is(err, services.ErrInvalidUsername):
		c.String(http.StatusBadRequest, "登录名无效（须为1至100个字符）")
	case errors.Is(err, services.ErrInvalidNickname):
		c.String(http.StatusBadRequest, "昵称无效（最多100个字符）")
	case errors.Is(err, services.ErrUserNotFound):
		c.AbortWithStatus(http.StatusUnauthorized)
	default:
		log.Printf("update profile: %v", err)
		c.AbortWithStatus(http.StatusInternalServerError)
	}
}
