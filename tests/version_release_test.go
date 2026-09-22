package tests

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/axsh/timeout"
)

func buildTimeoutxWithFlags(t *testing.T, ldflags string) string {
	t.Helper()
	root := repoRoot(t)
	name := "timeoutx-version-test"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	bin := filepath.Join(t.TempDir(), name)
	args := []string{"build", "-o", bin}
	if ldflags != "" {
		args = append(args, "-ldflags", ldflags)
	}
	args = append(args, "./cmd/timeoutx")
	cmd := exec.Command("go", args...)
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build timeoutx: %v\n%s", err, out)
	}
	return bin
}

func TestVersion_LibraryMatchesFile(t *testing.T) {
	root := repoRoot(t)
	raw, err := os.ReadFile(filepath.Join(root, "VERSION"))
	if err != nil {
		t.Fatalf("read VERSION: %v", err)
	}
	want := strings.TrimSpace(string(raw))
	if timeout.Version != want {
		t.Fatalf("timeout.Version=%q want %q", timeout.Version, want)
	}
}

func TestVersion_CLIShowsVersionAndCommit(t *testing.T) {
	bin := buildTimeoutxWithFlags(t, "-X main.commit=testhash1")
	out, err := exec.Command(bin, "version").CombinedOutput()
	if err != nil {
		t.Fatalf("version: %v\n%s", err, out)
	}
	got := strings.TrimSpace(string(out))
	want := "timeoutx " + timeout.Version + " (commit testhash1)"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestVersion_CLICommitFallbackUnknown(t *testing.T) {
	bin := buildTimeoutxWithFlags(t, "")
	out, err := exec.Command(bin, "version").CombinedOutput()
	if err != nil {
		t.Fatalf("version: %v\n%s", err, out)
	}
	got := strings.TrimSpace(string(out))
	want := "timeoutx " + timeout.Version + " (commit unknown)"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestRelease_RejectsDev(t *testing.T) {
	root := repoRoot(t)
	dir := t.TempDir()
	verFile := filepath.Join(dir, "VERSION")
	if err := os.WriteFile(verFile, []byte("0.3.0-dev\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("bash", filepath.Join(root, "scripts/process/release.sh"), "--dry-run")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "TIMEOUTX_VERSION_FILE="+verFile)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected failure for -dev VERSION, output=%s", out)
	}
	if !strings.Contains(string(out), "-dev") {
		t.Fatalf("expected -dev mention in output: %s", out)
	}
}

func TestRelease_DryRunArtifacts(t *testing.T) {
	root := repoRoot(t)
	const ver = "9.9.9"

	// Temporary rewrite of repo VERSION so go:embed matches the release check.
	// --dry-run skips the dirty-tree guard.
	repoVersionPath := filepath.Join(root, "VERSION")
	original, err := os.ReadFile(repoVersionPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.WriteFile(repoVersionPath, original, 0o644)
	})
	if err := os.WriteFile(repoVersionPath, []byte(ver+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command("bash", filepath.Join(root, "scripts/process/release.sh"), "--dry-run")
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("dry-run failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "v"+ver) {
		t.Fatalf("expected tag v%s in output: %s", ver, out)
	}
	if !strings.Contains(strings.ToLower(string(out)), "go get") && !strings.Contains(string(out), "module") {
		t.Fatalf("expected module/go get mention: %s", out)
	}

	dist := filepath.Join(root, "dist")
	wantNames := []string{
		"timeoutx_linux_amd64",
		"timeoutx_linux_arm64",
		"timeoutx_darwin_amd64",
		"timeoutx_darwin_arm64",
		"timeoutx_windows_amd64.exe",
		"timeoutx_windows_arm64.exe",
		"SHA256SUMS",
	}
	for _, name := range wantNames {
		path := filepath.Join(dist, name)
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("missing artifact %s: %v\n%s", name, err, out)
		}
	}

	hostName := "timeoutx_" + runtime.GOOS + "_" + runtime.GOARCH
	if runtime.GOOS == "windows" {
		hostName += ".exe"
	}
	hostBin := filepath.Join(dist, hostName)
	vout, err := exec.Command(hostBin, "version").CombinedOutput()
	if err != nil {
		t.Fatalf("host binary version: %v\n%s", err, vout)
	}
	if !strings.Contains(string(vout), ver) {
		t.Fatalf("host version output %q missing %q", vout, ver)
	}
}

func TestRelease_TagNameIsModuleVersion(t *testing.T) {
	root := repoRoot(t)
	raw, err := os.ReadFile(filepath.Join(root, "scripts/process/release.sh"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	if !strings.Contains(text, "go get github.com/axsh/timeout@") {
		t.Fatal("release.sh should document go get module tag")
	}
	if !strings.Contains(text, `TAG="v${VER}"`) {
		t.Fatal(`release.sh should set TAG="v${VER}"`)
	}
}

func TestVersion_DocsMention(t *testing.T) {
	root := repoRoot(t)
	checks := []struct {
		path string
		need []string
	}{
		{filepath.Join(root, "README.md"), []string{"VERSION", "scripts/process/release.sh"}},
		{filepath.Join(root, "docs/cli.md"), []string{"timeoutx version"}},
		{filepath.Join(root, "docs/library.md"), []string{"timeout.Version"}},
	}
	for _, c := range checks {
		raw, err := os.ReadFile(c.path)
		if err != nil {
			t.Fatalf("read %s: %v", c.path, err)
		}
		text := string(raw)
		for _, n := range c.need {
			if !strings.Contains(text, n) {
				t.Fatalf("%s missing %q", c.path, n)
			}
		}
	}
}
