# Use timeout as a Go library

Import the module from another repository:

```go
import "github.com/axsh/timeout"
```

```bash
go get github.com/axsh/timeout
```

## Version

```go
fmt.Println(timeout.Version) // e.g. "0.3.0-dev"
```

This matches the repository `VERSION` file. Released module tags are `v` + that value
(for example `go get github.com/axsh/timeout@v0.3.0`).

## Run a job

Configuration comes before the function, so a long function body does not hide the limits.

```go
result := timeout.Run(ctx,
    timeout.New(
        timeout.Hard(24*time.Hour),
        timeout.Idle(30*time.Second),
        timeout.Stall(2*time.Minute),
        timeout.UnitLimit(1*time.Minute),
    ),
    func(exec timeout.Execution) error {
        exec.Heartbeat()
        exec.Status("waiting for the external API")
        exec.Progress(timeout.Progress{
            Stage:   "import-users",
            Current: 3,
            Total:   10,
            Message: "importing users",
        })
        return nil
    },
)
```

Reuse the same configuration:

```go
cfg := timeout.New(
    timeout.Hard(24*time.Hour),
    timeout.Idle(30*time.Second),
)
r1 := timeout.Run(ctx, cfg, jobA)
r2 := cfg.Run(ctx, jobB)
```

A duration of `0` disables that policy. Pass `timeout.New()` when you want every time policy disabled. Do not pass a nil config.

`Execution.Context()` is canceled when a policy fires, the parent context is canceled, or the engine hits an internal error. Pass that context into the work you start.

## Signals

| Call | Meaning | Resets idle | Resets stall |
|---|---|---|---|
| `Heartbeat()` | The worker is still responsive | yes | no |
| `Status(message)` | The current state changed, but the work did not advance | yes | no |
| `Progress` with no total, or with an increased `Current` | Meaningful progress | yes | yes |
| Quantitative `Progress` with the same or a lower `Current` | Still responsive, but not moving forward | yes | no |

Sending heartbeats, or repeating `3/10`, does not prevent a stall timeout.

`ProgressTo(current, total)` is only a shortcut for the two numbers. Use `Progress` when you also need `Stage`, `Message`, or `Details`.

## Units

A unit is a named step with its own hard limit.

```go
err := exec.Unit("download", func(ctx context.Context) error {
    return download(ctx)
})
```

Manual units are allowed, including several at once. `End` is idempotent.

```go
unit := exec.BeginUnit("database-import")
defer unit.End()
```

## Result

`result.Status` is one of `succeeded`, `failed`, `timeout`, `canceled`, or `internal_error`.

`result.Kind` is `none`, `hard`, `idle`, `stall`, `unit`, or `probe`.

On timeout, `result.Snapshot` keeps the latest status, the latest progress, and the unit that exceeded its limit.

The library cancels the execution context. It does not kill arbitrary goroutines. Process `SIGTERM` / `SIGKILL` is handled by the CLI, or by the process runner when you use that API.

## Examples

These directories are complete programs you can run:

- [basic-run](../examples/go/basic-run) — configuration before the function, then success
- [stall-heartbeat](../examples/go/stall-heartbeat) — heartbeats do not clear stall
- [units](../examples/go/units) — per-step time limit

See also the [CLI and shell guide](cli.md) and the [project overview](../README.md).
