package test

import (
	"bytes"
	"encoding/base64"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"ai-later-nav/internal/config"
	"ai-later-nav/internal/handlers"
	"ai-later-nav/internal/web"

	"github.com/gin-gonic/gin"
)

const tinyPNGBase64 = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNkYPhfDwAChwGA60e6kgAAAABJRU5ErkJggg=="

func TestAdminSiteFormsRenderLogoUpload(t *testing.T) {
	tmpl := web.BuildSharedTemplates(os.DirFS("../templates"))

	cases := []struct {
		name string
		data map[string]any
	}{
		{"admin-add-site.html", map[string]any{"username": "admin", "adminSection": "sites"}},
		{"admin-edit-site.html", map[string]any{
			"username":     "admin",
			"adminSection": "sites",
			"site":         map[string]any{"ID": 1, "Logo": "/static/img/nav-test.png"},
		}},
	}

	for _, tc := range cases {
		var out bytes.Buffer
		if err := tmpl.ExecuteTemplate(&out, tc.name, tc.data); err != nil {
			t.Fatalf("%s ExecuteTemplate failed: %v", tc.name, err)
		}
		html := out.String()
		if !strings.Contains(html, "logoFileInput") {
			t.Fatalf("%s did not render logo upload control", tc.name)
		}
		if tc.name == "admin-edit-site.html" && !strings.Contains(html, `value="/static/img/nav-test.png"`) {
			t.Fatalf("admin-edit-site.html did not keep existing logo path")
		}
	}
}

func TestStaticHandlerOverlaysUploads(t *testing.T) {
	uploadDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(uploadDir, "uploaded.png"), []byte("uploaded-bytes"), 0o644); err != nil {
		t.Fatal(err)
	}

	embed := fstest.MapFS{
		"img/logo.png":  &fstest.MapFile{Data: []byte("embedded-bytes")},
		"css/style.css": &fstest.MapFile{Data: []byte("css")},
	}
	h := handlers.NewStaticHandler(embed, uploadDir)

	cases := []struct {
		url      string
		wantCode int
		wantBody string
	}{
		{"/static/img/uploaded.png", http.StatusOK, "uploaded-bytes"},
		{"/static/img/logo.png", http.StatusOK, "embedded-bytes"},
		{"/static/css/style.css", http.StatusOK, "css"},
		{"/static/img/missing.png", http.StatusNotFound, ""},
		{"/static/img/../../etc/passwd", http.StatusNotFound, ""},
	}

	for _, tc := range cases {
		req := httptest.NewRequest(http.MethodGet, tc.url, nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != tc.wantCode {
			t.Fatalf("GET %s: got status %d, want %d", tc.url, rec.Code, tc.wantCode)
		}
		if tc.wantBody != "" && rec.Body.String() != tc.wantBody {
			t.Fatalf("GET %s: got body %q, want %q", tc.url, rec.Body.String(), tc.wantBody)
		}
	}
}

func newUploadRouter(t *testing.T) (*gin.Engine, string) {
	gin.SetMode(gin.TestMode)
	uploadDir := t.TempDir()
	config.AppConfig = config.Config{UploadDir: uploadDir}

	r := gin.New()
	r.POST("/admin/upload", handlers.NewAdminHandler().AdminUploadImage)
	return r, uploadDir
}

func uploadRequest(t *testing.T, filename string, content []byte) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	fw, err := writer.CreateFormFile("file", filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fw.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/admin/upload", &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req
}

// uploadedPath extracts the "/static/img/..." path from a success response.
func uploadedPath(t *testing.T, body string) string {
	t.Helper()
	start := strings.Index(body, "/static/img/")
	if start < 0 {
		t.Fatalf("response has no image path: %s", body)
	}
	end := strings.IndexByte(body[start:], '"')
	if end < 0 {
		t.Fatalf("response path is unterminated: %s", body)
	}
	return body[start : start+end]
}

func TestAdminUploadImageAcceptsPNG(t *testing.T) {
	r, uploadDir := newUploadRouter(t)

	png, err := base64.StdEncoding.DecodeString(tinyPNGBase64)
	if err != nil {
		t.Fatal(err)
	}

	req := uploadRequest(t, "My Site Logo.PNG", png)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("upload failed with status %d: %s", rec.Code, rec.Body.String())
	}

	path := uploadedPath(t, rec.Body.String())
	if !strings.HasPrefix(path, "/static/img/nav-my-site-logo-") || !strings.HasSuffix(path, ".png") {
		t.Fatalf("unexpected response path: %s", path)
	}

	// The returned path must map to a real file under the upload dir.
	diskPath := filepath.Join(uploadDir, strings.TrimPrefix(path, "/static/img/"))
	data, err := os.ReadFile(diskPath)
	if err != nil {
		t.Fatalf("uploaded file not found at %s: %v", diskPath, err)
	}
	if !bytes.Equal(data, png) {
		t.Fatalf("stored file content mismatch")
	}
}

func TestAdminUploadImageSanitizesChineseFilename(t *testing.T) {
	r, _ := newUploadRouter(t)

	png, err := base64.StdEncoding.DecodeString(tinyPNGBase64)
	if err != nil {
		t.Fatal(err)
	}

	req := uploadRequest(t, "站点图标.png", png)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("upload failed with status %d: %s", rec.Code, rec.Body.String())
	}
	if path := uploadedPath(t, rec.Body.String()); !strings.HasPrefix(path, "/static/img/nav-logo-") {
		t.Fatalf("unexpected slug path for Chinese filename: %s", path)
	}
}

func TestAdminUploadImageRejectsDisguisedTextFile(t *testing.T) {
	r, _ := newUploadRouter(t)

	req := uploadRequest(t, "evil.png", []byte("<html><script>alert(1)</script></html>"))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for disguised text file, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "不是有效图片") {
		t.Fatalf("unexpected error message: %s", rec.Body.String())
	}
}

func TestAdminUploadImageRejectsBadExtension(t *testing.T) {
	r, _ := newUploadRouter(t)

	req := uploadRequest(t, "page.svg", []byte("<svg xmlns='http://www.w3.org/2000/svg'></svg>"))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for svg upload, got %d", rec.Code)
	}
}

func TestAdminUploadImageRejectsOversizedFile(t *testing.T) {
	r, _ := newUploadRouter(t)

	big := make([]byte, 6<<20)
	copy(big, []byte("\x89PNG\r\n\x1a\n"))

	req := uploadRequest(t, "big.png", big)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for oversized upload, got %d", rec.Code)
	}
}
