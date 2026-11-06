package services

import (
	"errors"
	"strings"
	"testing"
)

func TestUsernameChangeRequiresReauthentication(t *testing.T) {
	if err := CheckProfileUsername("old", "new"); !errors.Is(err, ErrUsernameReauthRequired) {
		t.Fatalf("expected reauthentication error, got %v", err)
	}
	if err := CheckProfileUsername("old", "old"); err != nil {
		t.Fatalf("same username rejected: %v", err)
	}
}

func TestValidateProfile(t *testing.T) {
	for _, tc := range []struct {
		username, nickname string
		want               error
	}{
		{"alice", "昵称", nil}, {"", "昵称", ErrInvalidUsername},
		{strings.Repeat("a", 101), "", ErrInvalidUsername}, {"alice\nname", "", ErrInvalidUsername},
		{"alice", strings.Repeat("中", 101), ErrInvalidNickname}, {"alice", "a\nb", ErrInvalidNickname},
	} {
		if err := ValidateProfile(tc.username, tc.nickname); !errors.Is(err, tc.want) {
			t.Errorf("ValidateProfile(%q, %q) = %v, want %v", tc.username, tc.nickname, err, tc.want)
		}
	}
}
