package handlers

import (
	"ai-later-nav/internal/config"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const maxImageUploadBytes = 5 << 20 // 5MB

var allowedImageExts = map[string]bool{
	".png":  true,
	".jpg":  true,
	".jpeg": true,
	".gif":  true,
	".webp": true,
	".ico":  true,
}

// AdminUploadImage accepts a multipart image and stores it in the upload dir
// so it is served from /static/img/, matching the existing logo path format.
func (h *AdminHandler) AdminUploadImage(c *gin.Context) {
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "未选择文件"})
		return
	}
	defer file.Close()

	if header.Size > maxImageUploadBytes {
		c.JSON(http.StatusBadRequest, gin.H{"error": "图片大小不能超过 5MB"})
		return
	}

	ext := strings.ToLower(filepath.Ext(header.Filename))
	if !allowedImageExts[ext] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "仅支持 png/jpg/jpeg/gif/webp/ico 格式"})
		return
	}

	// Sniff the real content so a renamed non-image cannot pass as .png.
	head := make([]byte, 512)
	n, err := io.ReadFull(file, head)
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		c.JSON(http.StatusBadRequest, gin.H{"error": "读取文件失败"})
		return
	}
	if !strings.HasPrefix(http.DetectContentType(head[:n]), "image/") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "文件内容不是有效图片"})
		return
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "读取文件失败"})
		return
	}

	uploadDir := config.AppConfig.UploadDir
	if uploadDir == "" {
		uploadDir = "uploads/img"
	}
	if err := os.MkdirAll(uploadDir, 0o755); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建上传目录失败"})
		return
	}

	name := fmt.Sprintf("nav-%s-%d%s", imageBaseSlug(header.Filename), time.Now().UnixNano(), ext)
	dst, err := os.Create(filepath.Join(uploadDir, name))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存文件失败"})
		return
	}
	defer dst.Close()

	if _, err := io.Copy(dst, file); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存文件失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"path": "/static/img/" + name})
}

// imageBaseSlug keeps the readable part of the original name so uploaded
// files follow the existing nav-<name>-<timestamp>.<ext> naming convention.
func imageBaseSlug(filename string) string {
	base := strings.TrimSuffix(filepath.Base(filename), filepath.Ext(filename))
	base = strings.ToLower(base)

	var b strings.Builder
	for _, r := range base {
		switch {
		case r >= 'a' && r <= 'z' || r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_' || r == ' ' || r == '.':
			b.WriteByte('-')
		}
	}
	slug := b.String()
	for strings.Contains(slug, "--") {
		slug = strings.ReplaceAll(slug, "--", "-")
	}
	slug = strings.Trim(slug, "-")
	if len(slug) > 40 {
		slug = slug[:40]
	}
	if slug == "" {
		slug = "logo"
	}
	return slug
}
