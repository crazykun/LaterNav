package handlers

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"ai-later-nav/internal/config"
	"ai-later-nav/internal/services"
	"github.com/gin-gonic/gin"
)

const maxAvatarBytes = 2 << 20

// validateAvatar accepts only decoded PNG/JPEG/GIF images whose file extension agrees
// with the decoded format. Unsupported formats can be added with an explicit decoder.
func validateAvatar(r io.Reader, filename string) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, maxAvatarBytes+1))
	if err != nil || len(data) == 0 || len(data) > maxAvatarBytes {
		return nil, errors.New("图片大小不能超过 2MB")
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || cfg.Width < 1 || cfg.Height < 1 || cfg.Width > 2048 || cfg.Height > 2048 || int64(cfg.Width)*int64(cfg.Height) > 4_000_000 {
		return nil, errors.New("图片格式或尺寸无效（最大 2048×2048、400 万像素）")
	}
	ext := strings.ToLower(filepath.Ext(filename))
	if !(format == "png" && ext == ".png" || format == "jpeg" && (ext == ".jpg" || ext == ".jpeg") || format == "gif" && ext == ".gif") {
		return nil, errors.New("仅支持 PNG、JPEG、GIF 图片")
	}
	// Decode the entire image, not only its header, before persisting it.
	if _, _, err := image.Decode(bytes.NewReader(data)); err != nil {
		return nil, errors.New("图片内容不完整或已损坏")
	}
	return data, nil
}

// managedAvatarPath only allows deletion of files created by the avatar uploader.
func managedAvatarPath(dir, url string) string {
	const prefix = "/static/img/avatar-"
	if !strings.HasPrefix(url, prefix) {
		return ""
	}
	name := strings.TrimPrefix(url, "/static/img/")
	base := strings.TrimSuffix(name, filepath.Ext(name))
	if len(base) != len("avatar-")+32 {
		return ""
	}
	if _, err := hex.DecodeString(strings.TrimPrefix(base, "avatar-")); err != nil {
		return ""
	}
	switch filepath.Ext(name) {
	case ".png", ".jpg", ".jpeg", ".gif":
		return filepath.Join(dir, name)
	}
	return ""
}

// UploadAvatar must be behind AuthMiddleware; images are served by NewStaticHandler.
func UploadAvatar(c *gin.Context) {
	if c.GetInt64("user_id") <= 0 {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	// PostForm inside CSRF validation parses multipart data, so cap the body first.
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxAvatarBytes+(1<<20))
	if !validUserCSRF(c) {
		c.AbortWithStatus(http.StatusForbidden)
		return
	}
	file, header, err := c.Request.FormFile("avatar")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请选择不超过 2MB 的图片"})
		return
	}
	defer file.Close()
	if header.Size > maxAvatarBytes {
		c.JSON(http.StatusBadRequest, gin.H{"error": "图片大小不能超过 2MB"})
		return
	}
	data, err := validateAvatar(file, header.Filename)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	dir := config.AppConfig.UploadDir
	if dir == "" {
		dir = "uploads/img"
	}
	if err = os.MkdirAll(dir, 0o755); err != nil {
		log.Printf("avatar directory: %v", err)
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}
	random := make([]byte, 16)
	if _, err = rand.Read(random); err != nil {
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}
	name := "avatar-" + hex.EncodeToString(random) + strings.ToLower(filepath.Ext(header.Filename))
	path := filepath.Join(dir, name)
	out, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		log.Printf("avatar create: %v", err)
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}
	_, writeErr := out.Write(data)
	closeErr := out.Close()
	if writeErr != nil || closeErr != nil {
		os.Remove(path)
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}
	url := "/static/img/" + name
	previous, err := services.NewUserService().ReplaceAvatar(c.GetInt64("user_id"), url)
	if err != nil {
		os.Remove(path)
		log.Printf("avatar update: %v", err)
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}
	if previousPath := managedAvatarPath(dir, previous); previousPath != "" {
		if err := os.Remove(previousPath); err != nil && !os.IsNotExist(err) {
			log.Printf("avatar cleanup: %v", err)
		}
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, gin.H{"avatar_url": url})
}
