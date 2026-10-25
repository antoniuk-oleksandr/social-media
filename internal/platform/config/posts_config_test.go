package config

import (
	"os"
	"testing"
)

func TestLoadPostsConfigUsesDefaultAndEnvironment(t *testing.T) {
	if err := os.Unsetenv("POSTS_TABLE"); err != nil {
		t.Fatal(err)
	}
	if got := LoadPostsConfig().PostsTable; got != "social-media-local-posts" {
		t.Fatalf("default posts table = %q", got)
	}
	if err := os.Setenv("POSTS_TABLE", "custom-posts"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Unsetenv("POSTS_TABLE") })
	if got := LoadPostsConfig().PostsTable; got != "custom-posts" {
		t.Fatalf("configured posts table = %q", got)
	}
}
