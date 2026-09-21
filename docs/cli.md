# Use timeoutx from scripts

`timeoutx` applies the same policies as the Go library. Install a release binary or build it yourself. See the [README](../README.md) for download and build steps.

## Run a command

```bash
timeoutx run \
  --hard 24h \
  --idle 30s \
  --stall 5m \
  --unit 1m \
  --kill-after 10s \
  -- ./job.sh
```

Durations use Go's format: `500ms`, `30s`, `5m`, `24h`. A duration of `0` disables that policy. If every time policy is disabled, `timeoutx` may warn and still runs the command.

The child process stdout and stderr pass through. `timeoutx` diagnostics go to stderr.

On timeout, Unix builds signal the child process group with `SIGTERM`, wait for `--kill-after` (default `10s`), then send `SIGKILL`. The default exit code for a policy timeout is `124`, matching GNU `timeout`. The kind (`hard`, `idle`, `stall`, `unit`) is printed on stderr and stored in the result JSON, not in the exit code.

| Exit code | Meaning |
|---:|---|
| 0..123 | The child command's own status |
| 124 | A timeoutx policy fired |
| 125 | timeoutx internal error |
| 126 | The command could not be executed |
| 127 | The command was not found |

Save a JSON result:

```bash
timeoutx run --stall 2m --result result.json -- ./worker.sh
```

## Shell helpers

Shell functions do not implement the policy engine. They send signals to `timeoutx` over an inherited file descriptor.

```bash
#!/usr/bin/env bash
eval "$(timeoutx shell-init)"

timeout_heartbeat
timeout_status "waiting for the external API"

total=10
for i in $(seq 1 "$total"); do
    timeout_unit "import:$i" ./import "$i"

    timeout_progress \
      --current "$i" \
      --total "$total" \
      --stage import-users \
      "imported item ${i}"
done
```

| Function | Effect |
|---|---|
| `timeout_heartbeat` | Responsive, but not progress |
| `timeout_status "..."` | Update the current state. Resets idle only |
| `timeout_progress "..."` | Qualitative progress. Resets idle and stall |
| `timeout_progress --current N --total M --stage NAME "..."` | Quantitative progress |
| `timeout_unit NAME COMMAND...` | Run one command as a named unit |
| `timeout_unit_begin` / `timeout_unit_end` | Manual units |

Repeating the same `--current` value does not reset stall.

## Watch a command you cannot modify

Treat any stdout or stderr write as a heartbeat (not as progress):

```bash
timeoutx run --idle 30s --heartbeat-on-output -- rsync ...
```

Treat output as progress only when you opt in:

```bash
timeoutx run --stall 30s --progress-on-output -- ./worker.sh
```

Treat file size or modification time changes as progress:

```bash
timeoutx run --stall 1m --progress-file ./output.tar -- ./backup.sh
```

A missing file is not progress.

For the Go API, see [Use timeout as a Go library](library.md).
