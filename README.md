# timeout

Detect long-running jobs that are alive but no longer making progress.

`timeout` is an execution policy engine. GNU `timeout` only asks whether a wall-clock limit has elapsed. This project also separates:

- the whole run is too long (**hard**)
- the process stopped responding (**idle**)
- the process is still responding, but the work is not moving forward (**stall**)
- a single named step is too long (**unit**)

The Go library, the `timeoutx` CLI, and shell helpers share one policy engine.

```text
Heartbeat  = still alive
Status     = current state, not progress
Progress   = the work actually moved forward
```

## Install

Go module:

```bash
go get github.com/axsh/timeout
```

CLI from source:

```bash
go install github.com/axsh/timeout/cmd/timeoutx@latest
```

Prebuilt `timeoutx` binaries are published on [GitHub Releases](https://github.com/axsh/timeout/releases) for:

| OS | Architectures |
|---|---|
| Linux | amd64, arm64 |
| macOS | amd64, arm64 |
| Windows | amd64, arm64 |

Each release includes a `SHA256SUMS` file. On Windows the file name ends with `.exe`.

## Build

Requirements: Go 1.22 or newer.

```bash
go build -o timeoutx ./cmd/timeoutx
```

Cross-compile examples:

```bash
GOOS=linux   GOARCH=amd64 go build -o timeoutx_linux_amd64       ./cmd/timeoutx
GOOS=darwin  GOARCH=arm64 go build -o timeoutx_darwin_arm64      ./cmd/timeoutx
GOOS=windows GOARCH=amd64 go build -o timeoutx_windows_amd64.exe ./cmd/timeoutx
```

Tests:

```bash
go test ./...
```

## Documentation

Start here if you are new to the project:

- [Use timeout as a Go library](docs/library.md)
- [Use timeoutx from shell scripts](docs/cli.md)

## License

See the repository license file.
