package timeout

import (
	"os"
	"strings"
	"testing"
)

func TestVersionMatchesVERSIONFile(t *testing.T) {
	raw, err := os.ReadFile("VERSION")
	if err != nil {
		t.Fatalf("read VERSION: %v", err)
	}
	want := strings.TrimSpace(string(raw))
	if Version != want {
		t.Fatalf("Version=%q want %q", Version, want)
	}
}

func TestVersionNonEmpty(t *testing.T) {
	if Version == "" {
		t.Fatal("Version is empty")
	}
}
