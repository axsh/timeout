# 000-Timeoutx-Core-Policy-Engine

> **Source Specification**: `prompts/phases/000-foundation/branches/main/ideas/000-Timeoutx-Execution-Policy-Engine.md`
> **Continuation**: Part 2 は `prompts/phases/000-foundation/branches/main/plans/001-Timeoutx-CLI-Shell-Release.md`

## Goal Description

公開 Go module `github.com/axsh/timeout` のルートに Execution Policy Engine を実装する。
Hard / Idle / Stall / Unit の独立判定、`New` + `Run(ctx, Config, Func)` API、差し替え可能な monotonic clock、Race-safe な Signal / Snapshot を完成させ、単体テストで受け入れ条件 VS1–VS6 / VS11–VS12 を満たす。

## User Review Required

None. CLI / Shell / Probe / Release は Part 2 で扱う。

## Requirement Traceability

| Requirement (from Spec) | Implementation Point (Section/File) |
| :--- | :--- |
| R2 Signal Idle/Stall 更新表 | Proposed Changes > progress.go / signal.go |
| R3 Hard/Idle/Stall/Unit Policy | Proposed Changes > monitor.go / policy.go |
| R3.5 優先順位 Hard>Unit>Stall>Idle、monotonic clock | Proposed Changes > monitor.go / clock.go |
| R4 Go API (Policy, Progress, Execution, Unit, Config, New, Run) | Proposed Changes > types / config / execution |
| R5 Progress 定性・定量・Stall 判定 | Proposed Changes > progress.go |
| R6 Unit 同期/手動/並行、冪等 End | Proposed Changes > unit.go |
| R7 Snapshot / Result | Proposed Changes > result.go / snapshot.go |
| R8 Observer Event（基本通知） | Proposed Changes > observer.go |
| R17 Concurrency | Proposed Changes > execution.go mutex |
| R18 デフォルト全 Policy disabled | Proposed Changes > config.go |
| R19.1 module path `github.com/axsh/timeout` | Proposed Changes > go.mod + build.sh |
| VS1–VS6, VS11–VS12 | Verification Plan + `*_test.go` |
| R9–R16, R12 Probe, R19.2 Release | **Deferred to Part 2** |

## Proposed Changes

### Module scaffold

#### [NEW] [go.mod](file://go.mod)
*   **Description**: 公開 module をルートに定義する。
*   **Technical Design**:
    ```text
    module github.com/axsh/timeout

    go 1.22
    ```
*   **Logic**:
    *   依存は標準ライブラリのみ（testify はテスト用に追加可）。

#### [MODIFY] [scripts/process/build.sh](file://scripts/process/build.sh)
*   **Description**: ルート `go.mod` をビルド・テスト対象に含める。現状は `features/*/` のみのため。
*   **Logic**:
    1. `PROJECT_ROOT/go.mod` があればルートを処理する。
    2. `go test -race -count=1` で `./...` を実行する（`tests/` パッケージは integration 側のためルート直下の unit のみ。ルートに `tests/` サブディレクトリがある場合は `go list` で除外）。
    3. `go build -o bin/timeoutx ./cmd/timeoutx` を実行する（`cmd/timeoutx` が無い段階ではスキップしてよいが、Part 1 完了時点では stub main を置く）。
    4. 既存の `features/*/` ループは残す。

#### [NEW] [cmd/timeoutx/main.go](file://cmd/timeoutx/main.go)
*   **Description**: Part 1 では最小 stub（`timeoutx version` または usage 表示）。本実装は Part 2。
*   **Logic**: `package main` / `func main()` で usage を stderr に出し exit 0。ビルド検証用。

### Core types (test files first — TDD)

#### [NEW] [policy_test.go](file://policy_test.go) / [policy.go](file://policy.go)
*   **Description**: Policy 検証と Option。
*   **Technical Design**:
    ```go
    package timeout

    type Policy struct {
        Hard      time.Duration
        Idle      time.Duration
        Stall     time.Duration
        Unit      time.Duration
        KillAfter time.Duration
    }

    type Option func(*Config) error

    func Hard(d time.Duration) Option
    func Idle(d time.Duration) Option
    func Stall(d time.Duration) Option
    func UnitLimit(d time.Duration) Option
    func KillAfter(d time.Duration) Option
    ```
*   **Logic**:
    *   Duration `0` = 無効。負値は `New` 時に error を返す（または `Must` せず `New` が panic せず、無効 Config + 内部エラーとして `Run` が `internal_error` を返す方針）。**採用**: `New` は負値で panic せず、`Config.err` に保持し、`Run` 先頭で `Result{Status: internal_error, Err: ...}` を返す。
    *   `KillAfter` のデフォルトは CLI 側で 10s（ライブラリ Policy では 0=未指定）。

#### [NEW] [clock.go](file://clock.go)
*   **Technical Design**:
    ```go
    type Clock interface {
        Now() time.Time
    }

    type realClock struct{}
    func (realClock) Now() time.Time { return time.Now() }

    // test only / WithClock option
    type manualClock struct {
        mu  sync.Mutex
        now time.Time
    }
    func (c *manualClock) Now() time.Time
    func (c *manualClock) Advance(d time.Duration)
    ```
*   **Logic**: Policy 判定はすべて `Clock.Now()` の差分（monotonic 相当として同一 Clock 上の経過）を使う。wall と別に `time.Since` を使わない。

#### [NEW] [config.go](file://config.go)
*   **Technical Design**:
    ```go
    type Config struct {
        policy   Policy
        clock    Clock
        observer Observer
        err      error // from New validation
    }

    func New(options ...Option) Config
    func WithClock(c Clock) Option
    func WithObserver(o Observer) Option

    func Run(ctx context.Context, cfg Config, fn Func) Result
    func (c Config) Run(ctx context.Context, fn Func) Result
    ```
*   **Logic**:
    *   `New` はゼロ値 Policy（全 disabled）から Options 適用。
    *   `Run` は毎回新しい `execution` を生成。`cfg` は不変として扱う（コピーしてから実行）。

#### [NEW] [progress.go](file://progress.go) / [progress_test.go](file://progress_test.go)
*   **Technical Design**:
    ```go
    type Progress struct {
        Stage   string
        Current int64
        Total   int64
        Message string
        Details any
    }

    // internal stall decision
    type progressSeries struct {
        stage   string
        total   int64
        current int64
        hasQuant bool
    }

    func classifyProgress(prev *progressSeries, p Progress) (idleUpdate, stallUpdate bool, next progressSeries)
    ```
*   **Logic**（仕様 R5.5 を再記述）:
    *   `Total <= 0`: 定性 Progress → idle=yes, stall=yes。系列をクリアまたは stage のみ保持。
    *   `Total > 0`: 定量。制約 `Current >= 0`。`Current > Total` は許容。
    *   同一系列 = (`Stage`, `Total`) が等しい。
    *   最初の定量 / Current 増加 / Stage 変更 / Total 変更 → stall 更新。
    *   同値 Current / 減少 Current → idle のみ。
    *   `ratio = float64(Current)/float64(Total)`, `percent = ratio*100`。

#### [NEW] [unit.go](file://unit.go) / [unit_test.go](file://unit_test.go)
*   **Technical Design**:
    ```go
    type Unit interface {
        ID() uint64
        Name() string
        End()
    }

    type unitState struct {
        id        uint64
        name      string
        startedAt time.Time
        ended     bool
    }
    ```
*   **Logic**:
    *   `BeginUnit` は atomic ID 採番、active map に登録、Unit 開始 Event。
    *   `End` は冪等。ended なら no-op。
    *   `Unit(name, fn)` は Begin → fn(ctx) → End（defer）。
    *   Unit Timeout: `now - startedAt >= policy.Unit`（Unit>0）の active unit があれば成立。複数ある場合は最初に検出したものを記録。

#### [NEW] [result.go](file://result.go) / [snapshot types]
*   **Technical Design**:
    ```go
    type ResultStatus string
    const (
        StatusSucceeded      ResultStatus = "succeeded"
        StatusFailed         ResultStatus = "failed"
        StatusTimeout        ResultStatus = "timeout"
        StatusCanceled       ResultStatus = "canceled"
        StatusInternalError  ResultStatus = "internal_error"
    )

    type TimeoutKind string
    const (
        KindNone  TimeoutKind = "none"
        KindHard  TimeoutKind = "hard"
        KindIdle  TimeoutKind = "idle"
        KindStall TimeoutKind = "stall"
        KindUnit  TimeoutKind = "unit"
        KindProbe TimeoutKind = "probe"
    )

    type State string // running, finished, ...

    type ProgressSnapshot struct {
        Stage     string
        Current   int64
        Total     int64
        Ratio     float64
        Percent   float64
        Message   string
        Details   any
        UpdatedAt time.Time
    }

    type UnitSnapshot struct {
        ID        uint64
        Name      string
        StartedAt time.Time
        Elapsed   time.Duration
        Limit     time.Duration
        Active    bool
    }

    type Snapshot struct {
        State          State
        StartedAt      time.Time
        Elapsed        time.Duration
        LastActivityAt time.Time
        IdleFor        time.Duration
        LastProgressAt time.Time
        StallFor       time.Duration
        Status         string
        Progress       ProgressSnapshot
        Units          []UnitSnapshot
    }

    type Result struct {
        Status     ResultStatus
        Kind       TimeoutKind
        Err        error
        Snapshot   Snapshot
        StartedAt  time.Time
        FinishedAt time.Time
        Elapsed    time.Duration
        // UnitTimeout detail when KindUnit
        TimeoutUnit *UnitSnapshot
    }
    ```

#### [NEW] [observer.go](file://observer.go)
*   **Technical Design**:
    ```go
    type EventType string
    // started, heartbeat, status, progress, meaningful_progress,
    // unit_started, unit_ended, timeout, termination_requested, finished

    type Event struct {
        Type      EventType
        Timestamp time.Time
        Snapshot  Snapshot
    }

    type Observer interface {
        OnEvent(Event)
    }
    ```
*   **Logic**: Observer 呼び出しは別 goroutine または短時間制限。監視ループを block しない（バッファ channel + drop-on-full をデフォルト）。

#### [NEW] [execution.go](file://execution.go) / [monitor.go](file://monitor.go)
*   **Technical Design**:
    ```go
    type Func func(Execution) error

    type Execution interface {
        Context() context.Context
        Heartbeat()
        Status(message string)
        Progress(progress Progress)
        ProgressTo(current, total int64)
        BeginUnit(name string) Unit
        Unit(name string, fn func(context.Context) error) error
        Snapshot() Snapshot
    }

    type execution struct {
        mu sync.Mutex
        ctx context.Context
        cancel context.CancelFunc
        clock Clock
        policy Policy
        startedAt time.Time
        lastActivityAt time.Time
        lastProgressAt time.Time
        statusMsg string
        progress ProgressSnapshot
        series progressSeries
        units map[uint64]*unitState
        nextUnitID uint64
        timedOut bool
        kind TimeoutKind
        done chan struct{}
        observer Observer
    }
    ```
*   **Logic — Run**:
    1. Config.err があれば internal_error で返す。
    2. `ctx, cancel := context.WithCancel(parent)`。
    3. `startedAt = clock.Now()`。`lastActivityAt` / `lastProgressAt` は開始時に startedAt（Stall/Idle は開始時点から計測）。
    4. monitor goroutine: 短い ticker（例 10ms、または next-deadline までの timer）で `evaluate()`。
    5. `fn(exec)` を呼び出し。戻り後に monitor 停止。
    6. Timeout と正常終了が競合したら、先に `commitTimeout` または `commitFinish` した方を採用（mutex + once）。
    7. 親 ctx cancel → `canceled`（timeout ではない）。
    8. fn が error → `failed`（timeout 済みなら timeout 優先）。

*   **Logic — evaluate() 優先順位**:
    1. Hard: `policy.Hard > 0 && now.Sub(startedAt) >= Hard` → KindHard
    2. Unit: いずれかの active unit で `now.Sub(startedAt) >= Unit` → KindUnit
    3. Stall: `policy.Stall > 0 && now.Sub(lastProgressAt) >= Stall` → KindStall
    4. Idle: `policy.Idle > 0 && now.Sub(lastActivityAt) >= Idle` → KindIdle
    5. 成立したら `timedOut=true`, cancel context, 通知。以後 Signal は no-op（または記録のみで結果を覆さない）。

*   **Logic — Signal**:
    *   Heartbeat: lastActivityAt = now（Idle yes, Stall no）
    *   Status: statusMsg = message, lastActivityAt = now
    *   Progress: classify → 更新 ProgressSnapshot、必要なら lastProgressAt
    *   ProgressTo(c,t): Progress{Current:c, Total:t}

## Step-by-Step Implementation Guide

1. **Scaffold**: `go.mod`, stub `cmd/timeoutx/main.go`, update `build.sh` for root module.
2. **TDD clock + policy options**: Write `policy_test.go` / `clock` tests (Advance), implement.
3. **TDD progress classification**: Table-driven cases for R5.5 matrix; implement `classifyProgress`.
4. **TDD unit End idempotency + Unit timeout detection** with manual clock.
5. **TDD Run Hard/Idle/Stall/Unit** independently (VS1); Heartbeat does not clear Stall (VS2); same Progress no Stall clear (VS3); Progress increase clears both (VS4); Status clears Idle only (VS5); Result fields (VS6).
6. **TDD race**: concurrent Heartbeat/Progress/Snapshot under `-race` (VS11).
7. **Observer** optional no-op path.
8. **Run Verification Plan** for Part 1.

## Verification Plan

### Automated Verification

1. **Build & Unit Tests**:
    ```bash
    ./scripts/process/build.sh
    ```

2. **Integration Tests** (Part 1 時点では CLI 未完成のため、ルート単体で十分な場合は integration は最小):
    ```bash
    ./scripts/process/integration_test.sh --specify "TestModuleImport"
    ```
    *   Part 1 で `tests/module_import_test.go` を追加し、別 module 風に `github.com/axsh/timeout` を import して `New`/`Run` を呼べることを確認（replace ディレクティブでローカル参照）。

3. **E2E Tests**:
    *   Part 1 はライブラリ中心。CLI E2E は Part 2。
    *   #### [NEW] [tests/module_import_test.go](file://tests/module_import_test.go)
        *   **テストケース**: `TestModuleImport_RunSucceeds`
        *   **検証ポイント**: `timeout.Run` が succeeded を返す

### Test Design Self-Review (§11.4)

1. **網羅性**: Hard/Idle/Stall/Unit、Signal 行列、優先順位、clock、race、Result 診断を単体で覆う。
2. **証拠**: Kind と Snapshot の時刻フィールドを assert。
3. **迂回排除**: Heartbeat/同値 Progress が Stall を回避できないことを明示ケース化。
4. **依存**: progress 分類 → monitor evaluate → Run API の順。

### 総合判定プロセス

Part 1 完了後、testing-rules §12 に従い判定を記録してから Part 2 へ進む。

## Documentation

#### [MODIFY] [README.md](file://README.md) / [docs/library.md](file://docs/library.md)
*   **更新内容**: API が実装と乖離しないよう、必要なら例を微修正（大きな変更は不要）。

## 継続計画について

Part 2: `001-Timeoutx-CLI-Shell-Release.md`（CLI、Protocol、Shell、Process、Probe、Release、残りの受け入れ条件）。
