package handlers

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestOAuthStartGeneratesStateAndRedirect(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &OAuthHandler{ClientID: "client", ClientSecret: "secret", RedirectURL: "http://localhost:8080/oauth/callback", AuthorizeURL: "https://linux.sb/forum_oauth_authorize"}
	r := gin.New()
	r.GET("/oauth/login", h.Start)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/oauth/login", nil))
	if w.Code != http.StatusFound {
		t.Fatalf("status = %d", w.Code)
	}
	u, err := url.Parse(w.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	if q.Get("state") == "" || q.Get("client_id") != "client" || q.Get("scope") != "openid profile email" || q.Get("redirect_uri") != h.RedirectURL {
		t.Fatalf("invalid redirect: %s", u)
	}
	cookies := w.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].HttpOnly || cookies[0].Value != q.Get("state") {
		t.Fatalf("invalid state cookie: %#v", cookies)
	}
}

func TestOAuthFetchProfileUsesAuthorizationCodeAndBearerToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			id, secret, ok := r.BasicAuth()
			if !ok || id != "client" || secret != "secret" || r.FormValue("code") != "code" || r.FormValue("redirect_uri") != "http://localhost:8080/oauth/callback" {
				t.Error("invalid token request")
			}
			w.Write([]byte(`{"access_token":"access"}`))
		case "/userinfo":
			if r.Header.Get("Authorization") != "Bearer access" {
				t.Error("missing bearer token")
			}
			w.Write([]byte(`{"sub":"stable-id","preferred_username":"alice","email":"virtual@example.org"}`))
		}
	}))
	defer server.Close()
	h := &OAuthHandler{ClientID: "client", ClientSecret: "secret", RedirectURL: "http://localhost:8080/oauth/callback", TokenURL: server.URL + "/token", UserInfoURL: server.URL + "/userinfo"}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	p, err := h.fetchProfile(c, "code")
	if err != nil || p.Sub != "stable-id" || p.Username != "alice" {
		t.Fatalf("profile = %#v, err = %v", p, err)
	}
}

func TestOAuthCallbackRejectsInvalidState(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &OAuthHandler{ClientID: "client", ClientSecret: "secret", RedirectURL: "http://localhost:8080/oauth/callback"}
	r := gin.New()
	r.GET("/oauth/callback", h.Callback)
	req := httptest.NewRequest(http.MethodGet, "/oauth/callback?code=code&state=bad", nil)
	req.AddCookie(&http.Cookie{Name: "oauth_state", Value: "different"})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}
