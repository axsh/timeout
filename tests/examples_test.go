package tests

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func goRunExample(t *testing.T, rel string) string {
	t.Helper()
	dir := filepath.Join(repoRoot(t), rel)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "run", ".")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s: %v\n%s", rel, err, out)
	}
	return string(out)
}

func bashExample(t *testing.T, rel string) {
	t.Helper()
	if _, err := exec.LookPath("bash"); err != nil {
		t.Fatalf("bash is required to run %s: %v", rel, err)
	}
	bin := timeoutxBin(t)
	dir := filepath.Join(repoRoot(t), rel)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bash", "run.sh")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "TIMEOUTX="+bin)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s: %v\n%s", rel, err, out)
	}
}

func TestExampleGoBasic(t *testing.T) {
	out := goRunExample(t, filepath.Join("examples", "go", "basic-run"))
	if !strings.Contains(out, "succeeded") {
		t.Fatalf("stdout=%s", out)
	}
}

func TestExampleGoStall(t *testing.T) {
	out := goRunExample(t, filepath.Join("examples", "go", "stall-heartbeat"))
	if !strings.Contains(out, "timeout") || !strings.Contains(out, "stall") {
		t.Fatalf("stdout=%s", out)
	}
}

func TestExampleGoUnit(t *testing.T) {
	out := goRunExample(t, filepath.Join("examples", "go", "units"))
	if !strings.Contains(out, "unit") || !strings.Contains(out, "download") {
		t.Fatalf("stdout=%s", out)
	}
}

func TestExampleBashShell(t *testing.T) {
	bashExample(t, filepath.Join("examples", "bash", "shell-progress"))
}

func TestExampleBashHeartbeat(t *testing.T) {
	bashExample(t, filepath.Join("examples", "bash", "heartbeat-on-output"))
}

func TestExampleBashFile(t *testing.T) {
	bashExample(t, filepath.Join("examples", "bash", "progress-file"))
}

func TestExampleDocsLinks(t *testing.T) {
	root := repoRoot(t)
	checks := []struct {
		file string
		subs []string
	}{
		{"docs/library.md", []string{"../examples/go/basic-run", "../examples/go/stall-heartbeat", "../examples/go/units"}},
		{"docs/cli.md", []string{"../examples/bash/shell-progress", "../examples/bash/heartbeat-on-output", "../examples/bash/progress-file"}},
		{"README.md", []string{"examples/README.md"}},
	}
	for _, c := range checks {
		b, err := os.ReadFile(filepath.Join(root, c.file))
		if err != nil {
			t.Fatal(err)
		}
		s := string(b)
		for _, sub := range c.subs {
			if !strings.Contains(s, sub) {
				t.Fatalf("%s missing %s", c.file, sub)
			}
		}
	}
}
