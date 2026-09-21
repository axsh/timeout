package tests

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/axsh/timeout"
)

func timeoutxBin(t *testing.T) string {
	t.Helper()
	root := repoRoot(t)
	name := "timeoutx"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	bin := filepath.Join(root, "bin", name)
	cmd := exec.Command("go", "build", "-o", bin, "./cmd/timeoutx")
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build timeoutx: %v\n%s", err, out)
	}
	return bin
}

func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Dir(wd)
}

func TestModuleImport_RunSucceeds(t *testing.T) {
	res := timeout.Run(context.Background(), timeout.New(), func(timeout.Execution) error {
		return nil
	})
	if res.Status != timeout.StatusSucceeded {
		t.Fatalf("status=%s", res.Status)
	}
}

func TestCLI_HardTimeoutExit124(t *testing.T) {
	bin := timeoutxBin(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, bin, "run", "--hard", "300ms", "--kill-after", "200ms", "--", "ping", "-n", "20", "127.0.0.1")
	} else {
		cmd = exec.CommandContext(ctx, bin, "run", "--hard", "200ms", "--kill-after", "100ms", "--", "sleep", "10")
	}
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected timeout, output=%s", out)
	}
	ee, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("err=%v out=%s", err, out)
	}
	if ee.ExitCode() != 124 {
		t.Fatalf("exit=%d out=%s", ee.ExitCode(), out)
	}
}

func TestCLI_ResultJSON(t *testing.T) {
	bin := timeoutxBin(t)
	dir := t.TempDir()
	resultPath := filepath.Join(dir, "result.json")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, bin, "run", "--hard", "300ms", "--kill-after", "200ms", "--result", resultPath, "--", "ping", "-n", "20", "127.0.0.1")
	} else {
		cmd = exec.CommandContext(ctx, bin, "run", "--hard", "200ms", "--kill-after", "100ms", "--result", resultPath, "--", "sleep", "10")
	}
	_ = cmd.Run()
	data, err := os.ReadFile(resultPath)
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	if !strings.Contains(s, `"status": "timeout"`) || !strings.Contains(s, `"kind": "hard"`) {
		t.Fatalf("unexpected result json: %s", data)
	}
}

func TestCLI_ShellInitOutput(t *testing.T) {
	bin := timeoutxBin(t)
	out, err := exec.Command(bin, "shell-init").CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	for _, fn := range []string{"timeout_heartbeat", "timeout_status", "timeout_progress", "timeout_unit"} {
		if !strings.Contains(s, fn) {
			t.Fatalf("shell-init missing %s:\n%s", fn, s)
		}
	}
}

func TestCLI_ShellHeartbeat(t *testing.T) {
	if runtime.GOOS == "windows" {
		// ExtraFiles FD inheritance is Unix-specific; shell-init coverage is in TestCLI_ShellInitOutput.
		TestCLI_ShellInitOutput(t)
		return
	}
	bin := timeoutxBin(t)
	script := "#!/usr/bin/env bash\n" +
		"eval \"$(" + bin + " shell-init)\"\n" +
		"timeout_heartbeat\n" +
		"timeout_status \"hello\"\n" +
		"timeout_progress --current 1 --total 2 --stage s \"m\"\n" +
		"sleep 0.05\n"
	dir := t.TempDir()
	path := filepath.Join(dir, "job.sh")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "run", "--idle", "5s", "--", "bash", path)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("err=%v out=%s", err, out)
	}
}

func TestCLI_ProgressFile(t *testing.T) {
	bin := timeoutxBin(t)
	dir := t.TempDir()
	file := filepath.Join(dir, "out.bin")

	// Helper process written in Go so Windows and Unix share one path.
	helper := filepath.Join(dir, "writer.go")
	src := `package main
import ("os"; "time")
func main() {
  time.Sleep(100*time.Millisecond)
  os.WriteFile(os.Args[1], []byte("x"), 0644)
  time.Sleep(300*time.Millisecond)
  f, _ := os.OpenFile(os.Args[1], os.O_APPEND|os.O_WRONLY, 0644)
  f.Write([]byte("yy"))
  f.Close()
  time.Sleep(100*time.Millisecond)
}`
	if err := os.WriteFile(helper, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	helperBin := filepath.Join(dir, "writer")
	if runtime.GOOS == "windows" {
		helperBin += ".exe"
	}
	build := exec.Command("go", "build", "-o", helperBin, helper)
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build helper: %v\n%s", err, out)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "run", "--stall", "2s", "--progress-file", file, "--", helperBin, file)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("err=%v out=%s", err, out)
	}
}

func TestGoAPI_StallParity(t *testing.T) {
	clk := timeout.NewManualClock(time.Unix(0, 0))
	started := make(chan struct{})
	ch := make(chan timeout.Result, 1)
	go func() {
		ch <- timeout.Run(context.Background(),
			timeout.New(timeout.Stall(time.Second), timeout.WithClock(clk)),
			func(exec timeout.Execution) error {
				exec.Heartbeat()
				close(started)
				<-exec.Context().Done()
				return nil
			},
		)
	}()
	<-started
	clk.Advance(time.Second)
	res := <-ch
	if res.Kind != timeout.KindStall {
		t.Fatalf("kind=%s", res.Kind)
	}
}

func TestCLI_CommandNotFound(t *testing.T) {
	bin := timeoutxBin(t)
	cmd := exec.Command(bin, "run", "--hard", "5s", "--", "timeoutx-definitely-missing-command-xyz")
	err := cmd.Run()
	if err == nil {
		t.Fatal("expected failure")
	}
	ee, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("err=%v", err)
	}
	code := ee.ExitCode()
	if code != 127 && code != 126 && code != 1 {
		// Platform differences: some return 1 from failed StatusFailed path.
		t.Fatalf("exit=%d want 127/126", code)
	}
}
