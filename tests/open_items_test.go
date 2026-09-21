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
)

func TestEvents_ConflictFlagsExit125(t *testing.T) {
	bin := timeoutxBin(t)
	cmd := exec.Command(bin, "run", "--events", "-", "--events-fd", "3", "--", "echo", "x")
	err := cmd.Run()
	ee, ok := err.(*exec.ExitError)
	if !ok || ee.ExitCode() != 125 {
		t.Fatalf("err=%v", err)
	}
}

func TestEvents_NotMixedWithChildStdout(t *testing.T) {
	bin := timeoutxBin(t)
	dir := t.TempDir()
	eventsPath := filepath.Join(dir, "events.ndjson")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, bin, "run", "--hard", "5s", "--events", eventsPath, "--", "cmd", "/c", "echo HELLO")
	} else {
		cmd = exec.CommandContext(ctx, bin, "run", "--hard", "5s", "--events", eventsPath, "--", "printf", "HELLO\n")
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("err=%v out=%s", err, out)
	}
	if !strings.Contains(string(out), "HELLO") {
		t.Fatalf("child stdout missing HELLO: %s", out)
	}
	// CombinedOutput mixes stderr; ensure events file exists and does not claim to be child-only check on stdout path.
	data, err := os.ReadFile(eventsPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "HELLO") {
		t.Fatalf("events mixed with child output: %s", data)
	}
}

func TestTimeoutExit_Custom143(t *testing.T) {
	bin := timeoutxBin(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, bin, "run", "--hard", "200ms", "--kill-after", "100ms", "--timeout-exit", "143", "--", "ping", "-n", "20", "127.0.0.1")
	} else {
		cmd = exec.CommandContext(ctx, bin, "run", "--hard", "200ms", "--kill-after", "100ms", "--timeout-exit", "143", "--", "sleep", "10")
	}
	err := cmd.Run()
	ee, ok := err.(*exec.ExitError)
	if !ok || ee.ExitCode() != 143 {
		t.Fatalf("exit err=%v", err)
	}
}

func TestTimeoutExit_Child124Passthrough(t *testing.T) {
	bin := timeoutxBin(t)
	dir := t.TempDir()
	resultPath := filepath.Join(dir, "r.json")
	helper := filepath.Join(dir, "exit124.go")
	src := "package main\nimport \"os\"\nfunc main(){ os.Exit(124) }\n"
	if err := os.WriteFile(helper, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	helperBin := filepath.Join(dir, "exit124")
	if runtime.GOOS == "windows" {
		helperBin += ".exe"
	}
	if out, err := exec.Command("go", "build", "-o", helperBin, helper).CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	cmd := exec.Command(bin, "run", "--hard", "1h", "--result", resultPath, "--", helperBin)
	err := cmd.Run()
	ee, ok := err.(*exec.ExitError)
	if !ok || ee.ExitCode() != 124 {
		t.Fatalf("want child 124, err=%v", err)
	}
	data, _ := os.ReadFile(resultPath)
	s := string(data)
	if !strings.Contains(s, `"schemaVersion": 1`) {
		t.Fatalf("missing schemaVersion: %s", s)
	}
	if strings.Contains(s, `"status": "timeout"`) {
		t.Fatalf("should not be timeout: %s", s)
	}
}

func TestResultSchema_SchemaVersion1(t *testing.T) {
	bin := timeoutxBin(t)
	dir := t.TempDir()
	resultPath := filepath.Join(dir, "r.json")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, bin, "run", "--hard", "200ms", "--kill-after", "100ms", "--result", resultPath, "--", "ping", "-n", "20", "127.0.0.1")
	} else {
		cmd = exec.CommandContext(ctx, bin, "run", "--hard", "200ms", "--kill-after", "100ms", "--result", resultPath, "--", "sleep", "10")
	}
	_ = cmd.Run()
	data, err := os.ReadFile(resultPath)
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	if !strings.Contains(s, `"schemaVersion": 1`) || !strings.Contains(s, `"status": "timeout"`) {
		t.Fatalf("%s", s)
	}
	schema := filepath.Join(repoRoot(t), "docs", "schemas", "result.schema.json")
	if _, err := os.Stat(schema); err != nil {
		t.Fatal(err)
	}
}

func TestProbe_HeartbeatDoesNotPreventStall(t *testing.T) {
	bin := timeoutxBin(t)
	dir := t.TempDir()
	probe := filepath.Join(dir, "probe.go")
	src := "package main\nimport \"fmt\"\nfunc main(){ fmt.Println(`{\"v\":1,\"type\":\"heartbeat\"}`) }\n"
	if err := os.WriteFile(probe, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	probeBin := filepath.Join(dir, "probe")
	if runtime.GOOS == "windows" {
		probeBin += ".exe"
	}
	if out, err := exec.Command("go", "build", "-o", probeBin, probe).CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, bin, "run", "--stall", "800ms", "--probe-every", "100ms", "--probe", probeBin, "--kill-after", "200ms", "--", "ping", "-n", "30", "127.0.0.1")
	} else {
		cmd = exec.CommandContext(ctx, bin, "run", "--stall", "800ms", "--probe-every", "100ms", "--probe", probeBin, "--kill-after", "200ms", "--", "sleep", "10")
	}
	err := cmd.Run()
	ee, ok := err.(*exec.ExitError)
	if !ok || ee.ExitCode() != 124 {
		t.Fatalf("want stall timeout 124, err=%v", err)
	}
}

func TestProbe_FailureDoesNotFailJob(t *testing.T) {
	bin := timeoutxBin(t)
	dir := t.TempDir()
	probe := filepath.Join(dir, "bad.go")
	src := "package main\nimport \"os\"\nfunc main(){ os.Exit(1) }\n"
	if err := os.WriteFile(probe, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	probeBin := filepath.Join(dir, "bad")
	if runtime.GOOS == "windows" {
		probeBin += ".exe"
	}
	if out, err := exec.Command("go", "build", "-o", probeBin, probe).CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command(bin, "run", "--hard", "5s", "--probe-every", "200ms", "--probe", probeBin, "--", "cmd", "/c", "exit", "0")
	} else {
		cmd = exec.Command(bin, "run", "--hard", "5s", "--probe-every", "200ms", "--probe", probeBin, "--", "true")
	}
	if err := cmd.Run(); err != nil {
		t.Fatalf("job should succeed despite probe failure: %v", err)
	}
}

func TestDetailsMarshal_OmitsOnFailure(t *testing.T) {
	res := runWithChanDetails(t)
	if res != "ok" {
		t.Fatal(res)
	}
}
