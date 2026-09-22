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

The product version lives in the repository root `VERSION` file (no leading `v`).
Released Go module tags and GitHub Release names are `v` + that value
(for example `VERSION=0.3.0` → `go get github.com/axsh/timeout@v0.3.0`).

Go module:

```bash
go get github.com/axsh/timeout@v0.3.0
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

Preferred (embeds the current git commit into the CLI):

```bash
./scripts/process/build.sh
./bin/timeoutx version
# timeoutx 0.3.0-dev (commit abcdef1)
```

The version string comes from `VERSION` / `timeout.Version`. The commit is set at link time.

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
./scripts/process/build.sh
```

## Release

1. Set `VERSION` to a release number without `-dev` (example: `0.3.0`).
2. Commit the change on a clean working tree.
3. Run:

```bash
./scripts/process/release.sh --dry-run   # optional: build artifacts only
./scripts/process/release.sh            # tag v0.3.0, push, GitHub Release
```

The script builds the six platform binaries, writes `SHA256SUMS`, creates tag `v{VERSION}`
(also the Go module version), and uploads the assets with `gh`.
Do not rely on a separate CI release workflow; `scripts/process/release.sh` is the release path.

## Documentation

Start here if you are new to the project:

- [Use timeout as a Go library](docs/library.md)
- [Use timeoutx from shell scripts](docs/cli.md)
- [Runnable examples](examples/README.md)
- [Result JSON Schema](docs/schemas/result.schema.json)
- [Event NDJSON Schema](docs/schemas/event.schema.json)

## License

See the repository license file.
