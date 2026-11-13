package handlers

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestValidateAvatarRejectsNonImageAndOversize(t *testing.T) {
	if _, err := validateAvatar(bytes.NewReader([]byte("not an image")), "photo.png"); err == nil {
		t.Fatal("non-image accepted")
	}
	if _, err := validateAvatar(bytes.NewReader(make([]byte, 2<<20+1)), "photo.png"); err == nil {
		t.Fatal("oversize accepted")
	}
}

func TestValidateAvatarAcceptsPNGAndRejectsMismatchedExtension(t *testing.T) {
	var b bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	if _, err := validateAvatar(bytes.NewReader(b.Bytes()), "photo.png"); err != nil {
		t.Fatalf("valid PNG: %v", err)
	}
	if _, err := validateAvatar(bytes.NewReader(b.Bytes()), "photo.jpg"); err == nil {
		t.Fatal("mismatched format accepted")
	}
	if _, err := validateAvatar(bytes.NewReader(b.Bytes()[:len(b.Bytes())-3]), "photo.png"); err == nil {
		t.Fatal("truncated image accepted")
	}
}

func TestManagedAvatarPathRejectsForeignOrMalformedNames(t *testing.T) {
	for _, url := range []string{"/static/img/nav-logo.png", "/static/img/avatar-../secret.png", "/static/img/avatar-1234.png", "https://example.com/photo.png"} {
		if got := managedAvatarPath("uploads/img", url); got != "" {
			t.Errorf("unsafe avatar URL %q -> %q", url, got)
		}
	}
	url := "/static/img/avatar-0123456789abcdef0123456789abcdef.png"
	if got := managedAvatarPath("uploads/img", url); got != "uploads/img/avatar-0123456789abcdef0123456789abcdef.png" {
		t.Fatalf("managed avatar path = %q", got)
	}
}

func TestUploadAvatarRejectsMissingCSRF(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/api/profile/avatar", func(c *gin.Context) { c.Set("user_id", int64(1)); UploadAvatar(c) })
	var b bytes.Buffer
	form := multipart.NewWriter(&b)
	part, err := form.CreateFormFile("avatar", "x.png")
	if err != nil {
		t.Fatal(err)
	}
	part.Write([]byte("image"))
	form.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/profile/avatar", &b)
	req.Header.Set("Content-Type", form.FormDataContentType())
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("got %d want 403: %s", rec.Code, strings.TrimSpace(rec.Body.String()))
	}
}
