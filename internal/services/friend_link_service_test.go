package services

import (
	"errors"
	"testing"

	"ai-later-nav/internal/models"
)

func TestValidateNormalizesFriendLink(t *testing.T) {
	svc := NewFriendLinkService()
	link, err := svc.Validate(&models.FriendLink{
		Name:        " Example ",
		URL:         "example.com/path",
		Description: " 一个很好的站点 ",
		Logo:        "/static/img/nav-example-1.png",
	})
	if err != nil {
		t.Fatal(err)
	}
	if link.URL != "https://example.com/path" {
		t.Fatalf("URL = %q, want https://example.com/path", link.URL)
	}
	if link.Name != "Example" || link.Description != "一个很好的站点" {
		t.Fatalf("unexpected trim result: %+v", link)
	}
}

func TestValidateRejectsBadURLAndLogo(t *testing.T) {
	svc := NewFriendLinkService()
	for _, tc := range []struct{ name, url string }{
		{"empty url", "  "},
		{"ftp url", "ftp://example.com"},
		{"no host", "https://"},
	} {
		if _, err := svc.Validate(&models.FriendLink{Name: "OK", URL: tc.url}); err == nil {
			t.Errorf("%s: expected validation error", tc.name)
		}
	}
	for _, logo := range []string{"img/x.png", "data:text/html;base64,xxx", "javascript:alert(1)"} {
		if _, err := svc.Validate(&models.FriendLink{Name: "OK", URL: "https://a.com", Logo: logo}); err == nil {
			t.Errorf("logo %q should be rejected", logo)
		}
	}
}

func TestValidateAcceptsRemoteLogoAndEmptyLogo(t *testing.T) {
	svc := NewFriendLinkService()
	if _, err := svc.Validate(&models.FriendLink{Name: "A", URL: "https://a.com", Logo: "https://a.com/logo.png"}); err != nil {
		t.Fatalf("remote logo rejected: %v", err)
	}
	if _, err := svc.Validate(&models.FriendLink{Name: "B", URL: "https://b.com"}); err != nil {
		t.Fatalf("empty logo rejected: %v", err)
	}
}

func TestMoveRejectsInvalidDirection(t *testing.T) {
	svc := NewFriendLinkService()
	if err := svc.Move(1, "sideways"); !errors.Is(err, ErrFriendLinkNotFound) {
		t.Fatalf("Move invalid direction err = %v", err)
	}
}
