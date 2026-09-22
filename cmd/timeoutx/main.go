package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/axsh/timeout"
	"github.com/axsh/timeout/process"
	"github.com/axsh/timeout/protocol"
)

// commit is set at link time via -ldflags "-X main.commit=...".
var commit = "unknown"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}
	switch os.Args[1] {
	case "version", "--version", "-V":
		fmt.Printf("timeoutx %s (commit %s)\n", timeout.Version, commit)
	case "shell-init":
		printShellInit()
	case "run":
		os.Exit(runCommand(os.Args[2:]))
	case "help", "--help", "-h":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "timeoutx: unknown command %q\n", os.Args[1])
		usage()
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `usage: timeoutx <command> [options]

commands:
  run          run a command under execution policies
  shell-init   print shell helper functions
  version      print version

example:
  timeoutx run --stall 2m -- ./job.sh`)
}

func runCommand(args []string) int {
	if _, err := protocol.EffectiveMaxLineBytes(); err != nil {
		fmt.Fprintf(os.Stderr, "timeoutx: %v\n", err)
		return 125
	}

	var (
		hard, idle, stall, unit, killAfter time.Duration
		resultPath, progressFile           string
		eventsPath                         string
		eventsFD                           = -1
		eventsFDSet                        bool
		eventsPathSet                      bool
		heartbeatOnOutput                  bool
		progressOnOutput                   bool
		showProgress                       bool
		progressFormat                     string
		probeCmd                           string
		probeArgs                          []string
		probeEvery                         = 5 * time.Second
		probeSignal                        string
		probeVerbose                       bool
		timeoutExit                        = 124
		signalExit                         bool
	)
	killAfter = 10 * time.Second

	i := 0
	for i < len(args) {
		a := args[i]
		if a == "--" {
			i++
			break
		}
		if !strings.HasPrefix(a, "-") {
			break
		}
		switch a {
		case "--hard":
			i++
			hard = mustDuration(args, &i, a)
		case "--idle":
			i++
			idle = mustDuration(args, &i, a)
		case "--stall":
			i++
			stall = mustDuration(args, &i, a)
		case "--unit":
			i++
			unit = mustDuration(args, &i, a)
		case "--kill-after":
			i++
			killAfter = mustDuration(args, &i, a)
		case "--result":
			i++
			resultPath = mustArg(args, &i, a)
		case "--progress-file":
			i++
			progressFile = mustArg(args, &i, a)
		case "--events":
			i++
			eventsPath = mustArg(args, &i, a)
			eventsPathSet = true
		case "--events-fd":
			i++
			eventsFD = mustInt(args, &i, a)
			eventsFDSet = true
		case "--probe":
			i++
			probeCmd = mustArg(args, &i, a)
		case "--probe-arg":
			i++
			probeArgs = append(probeArgs, mustArg(args, &i, a))
		case "--probe-every":
			i++
			probeEvery = mustDuration(args, &i, a)
		case "--probe-signal":
			i++
			probeSignal = mustArg(args, &i, a)
		case "--probe-verbose":
			probeVerbose = true
			i++
		case "--timeout-exit":
			i++
			timeoutExit = mustInt(args, &i, a)
		case "--signal-exit":
			signalExit = true
			i++
		case "--heartbeat-on-output":
			heartbeatOnOutput = true
			i++
		case "--progress-on-output":
			progressOnOutput = true
			i++
		case "--show-progress":
			showProgress = true
			i++
		case "--progress-format":
			i++
			progressFormat = mustArg(args, &i, a)
		default:
			fmt.Fprintf(os.Stderr, "timeoutx: unknown flag %q\n", a)
			return 125
		}
	}
	if eventsPathSet && eventsFDSet {
		fmt.Fprintln(os.Stderr, "timeoutx: --events and --events-fd are mutually exclusive")
		return 125
	}
	if probeEvery <= 0 {
		fmt.Fprintln(os.Stderr, "timeoutx: --probe-every must be > 0")
		return 125
	}
	if i >= len(args) {
		fmt.Fprintln(os.Stderr, "timeoutx: missing command")
		return 125
	}
	cmdName := args[i]
	cmdArgs := args[i+1:]

	opts := []timeout.Option{}
	if hard != 0 {
		opts = append(opts, timeout.Hard(hard))
	}
	if idle != 0 {
		opts = append(opts, timeout.Idle(idle))
	}
	if stall != 0 {
		opts = append(opts, timeout.Stall(stall))
	}
	if unit != 0 {
		opts = append(opts, timeout.UnitLimit(unit))
	}
	if killAfter != 0 {
		opts = append(opts, timeout.KillAfter(killAfter))
	}
	cfg := timeout.New(opts...)

	var probes []timeout.CommandProbe
	if probeCmd != "" {
		probes = append(probes, timeout.CommandProbe{
			Command:     probeCmd,
			Args:        probeArgs,
			Interval:    probeEvery,
			OnFailure:   timeout.ProbeOnFailureWarn,
			EmptySignal: probeSignal,
			Verbose:     probeVerbose,
		})
	}

	fd := -1
	if eventsFDSet {
		fd = eventsFD
	}
	_, code := (process.Runner{}).Run(process.RunRequest{
		Config:             cfg,
		Command:            cmdName,
		Args:               cmdArgs,
		KillAfter:          killAfter,
		HeartbeatOnOutput:  heartbeatOnOutput,
		ProgressOnOutput:   progressOnOutput,
		ProgressFile:       progressFile,
		ResultPath:         resultPath,
		ShowProgress:       showProgress,
		ProgressFormatJSON: progressFormat == "json",
		WarnNoPolicy:       true,
		Probes:             probes,
		EventsPath:         eventsPath,
		EventsFD:           fd,
		TimeoutExit:        timeoutExit,
		SignalExit:         signalExit,
	})
	_ = showProgress
	return code
}

func mustDuration(args []string, i *int, flag string) time.Duration {
	if *i >= len(args) {
		fmt.Fprintf(os.Stderr, "timeoutx: %s requires a duration\n", flag)
		os.Exit(125)
	}
	d, err := time.ParseDuration(args[*i])
	if err != nil {
		fmt.Fprintf(os.Stderr, "timeoutx: invalid duration for %s: %v\n", flag, err)
		os.Exit(125)
	}
	*i++
	return d
}

func mustArg(args []string, i *int, flag string) string {
	if *i >= len(args) {
		fmt.Fprintf(os.Stderr, "timeoutx: %s requires a value\n", flag)
		os.Exit(125)
	}
	v := args[*i]
	*i++
	return v
}

func mustInt(args []string, i *int, flag string) int {
	if *i >= len(args) {
		fmt.Fprintf(os.Stderr, "timeoutx: %s requires an integer\n", flag)
		os.Exit(125)
	}
	n, err := strconv.Atoi(args[*i])
	if err != nil {
		fmt.Fprintf(os.Stderr, "timeoutx: invalid int for %s: %v\n", flag, err)
		os.Exit(125)
	}
	*i++
	return n
}

func printShellInit() {
	_, _ = os.Stdout.WriteString(shellInitScript)
}

const shellInitScript = `# timeoutx shell helpers
timeout_control_send() {
  if [ -n "${TIMEOUTX_FD:-}" ] && [ "${TIMEOUTX_FD}" -ge 0 ] 2>/dev/null; then
    printf '%s\n' "$1" >&"$TIMEOUTX_FD" 2>/dev/null || true
    return 0
  fi
}

timeout_heartbeat() {
  timeout_control_send '{"v":1,"type":"heartbeat"}'
}

timeout_status() {
  local msg
  msg=$(printf '%s' "$1" | sed 's/\\/\\\\/g; s/"/\\"/g')
  timeout_control_send "{\"v\":1,\"type\":\"status\",\"message\":\"${msg}\"}"
}

timeout_progress() {
  local current="" total="" stage="" message=""
  while [ $# -gt 0 ]; do
    case "$1" in
      --current) current="$2"; shift 2 ;;
      --total) total="$2"; shift 2 ;;
      --stage) stage="$2"; shift 2 ;;
      *) message="$1"; shift ;;
    esac
  done
  local msg
  msg=$(printf '%s' "$message" | sed 's/\\/\\\\/g; s/"/\\"/g')
  local stg
  stg=$(printf '%s' "$stage" | sed 's/\\/\\\\/g; s/"/\\"/g')
  if [ -n "$current" ] && [ -n "$total" ]; then
    timeout_control_send "{\"v\":1,\"type\":\"progress\",\"stage\":\"${stg}\",\"current\":${current},\"total\":${total},\"message\":\"${msg}\"}"
  else
    timeout_control_send "{\"v\":1,\"type\":\"progress\",\"stage\":\"${stg}\",\"message\":\"${msg}\"}"
  fi
}

timeout_unit_begin() {
  local name="$1"
  local id
  id=$(date +%s%N 2>/dev/null || echo $$)
  local nm
  nm=$(printf '%s' "$name" | sed 's/\\/\\\\/g; s/"/\\"/g')
  timeout_control_send "{\"v\":1,\"type\":\"unit_begin\",\"id\":${id},\"name\":\"${nm}\"}"
  printf '%s\n' "$id"
}

timeout_unit_end() {
  local id="$1"
  timeout_control_send "{\"v\":1,\"type\":\"unit_end\",\"id\":${id}}"
}

timeout_unit() {
  local name="$1"
  shift
  local id
  id=$(timeout_unit_begin "$name")
  "$@"
  local st=$?
  timeout_unit_end "$id"
  return $st
}
`
