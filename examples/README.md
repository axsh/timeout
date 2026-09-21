# Examples

Runnable samples for the Go library and the `timeoutx` CLI. Build the CLI before the Bash examples:

```bash
go build -o bin/timeoutx ./cmd/timeoutx
```

## Go

| Example | What it shows | Run |
|---|---|---|
| [basic-run](go/basic-run) | Configuration before the function, quantitative progress, success | `cd examples/go/basic-run && go run .` |
| [stall-heartbeat](go/stall-heartbeat) | Heartbeats do not clear a stall timeout | `cd examples/go/stall-heartbeat && go run .` |
| [units](go/units) | A named unit has its own time limit | `cd examples/go/units && go run .` |

## Bash

| Example | What it shows | Run |
|---|---|---|
| [shell-progress](bash/shell-progress) | `shell-init`, units, and quantitative progress | `bash examples/bash/shell-progress/run.sh` |
| [heartbeat-on-output](bash/heartbeat-on-output) | stdout counts as a heartbeat | `bash examples/bash/heartbeat-on-output/run.sh` |
| [progress-file](bash/progress-file) | file growth counts as progress | `bash examples/bash/progress-file/run.sh` |

Guides: [Go library](../docs/library.md), [CLI](../docs/cli.md).
