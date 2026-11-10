package services

import (
	"reflect"
	"testing"
)

func TestNormalizeTagNames(t *testing.T) {
	tags, err := NormalizeTagNames(" AI绘画， 工具,AI绘画, ,开源 ")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"AI绘画", "工具", "开源"}
	if !reflect.DeepEqual(tags, want) {
		t.Fatalf("tags = %#v, want %#v", tags, want)
	}
	empty, err := NormalizeTagNames("")
	if err != nil || empty == nil || len(empty) != 0 {
		t.Fatalf("empty tags = %#v, %v; want non-nil empty slice", empty, err)
	}
	if _, err := NormalizeTagNames("good,tag"); err != nil {
		t.Fatalf("valid two tags rejected: %v", err)
	}
}
