package blog

import "testing"

func TestSlug(t *testing.T) {
	if Slug("Hello World") != "hello-world" {
		t.Fatal("bad slug")
	}
}

func TestTagged(t *testing.T) {
	p := Post{Tags: []string{"go", "spec"}}
	if !Tagged(p, "go") || Tagged(p, "rust") {
		t.Fatal("tag match wrong")
	}
}
