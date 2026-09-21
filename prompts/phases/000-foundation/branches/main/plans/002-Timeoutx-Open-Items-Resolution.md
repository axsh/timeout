# 002-Timeoutx-Open-Items-Resolution

> **Source Specification**: `prompts/phases/000-foundation/branches/main/ideas/001-Timeoutx-Open-Items-Resolution.md`

## Goal Description

親仕様で未確定だった 8 項目（Command Probe、Event 出力、行サイズ上限、Observer drop、Windows Job Object、Result Schema、Details marshal 失敗、`--timeout-exit`）を、仕様 001 の確定内容どおりに実装し、単体・統合テストで VS1–VS11 を満たす。

## User Review Required

None.

任意要件のうち次は本計画で **先送り** する（必須 8 項目の完了を優先）:

| 任意要件 | 先送り理由 |
|---|---|
| YAML `-c/--config` 一括定義 | CLI フラグ `--probe` / `--probe-every` で R1 を満たせる |
| `--max-line-bytes` CLI | 環境変数 `TIMEOUTX_MAX_LINE_BYTES` で R3 を満たせる |
| `--result-schema` stdout 出力 | Schema ファイル公開（R6）で十分 |

## Requirement Traceability

| Requirement (from Spec) | Implementation Point (Section/File) |
| :--- | :--- |
| R1 Command Probe Protocol | Proposed Changes > `probe.go`, `process/runner.go`, CLI flags |
| R1.2 NDJSON v1 on probe stdout | Proposed Changes > `probe.go` + `protocol` reuse |
| R1.3 fail vs warn | Proposed Changes > `probe.go` OnFailure |
| R1.4 overlap skip | Proposed Changes > `probe.go` running flag |
| R2 `--events` / `--events-fd` | Proposed Changes > `process/events.go`, CLI |
| R2.3 Event NDJSON format | Proposed Changes > `process/events.go` |
| R3 MaxLineBytes 1MiB + env | Proposed Changes > `protocol/v1.go` |
| R4 Observer queue / DropNewest\|Oldest | Proposed Changes > `observer.go`, `policy.go` |
| R5 Windows Job Object | Proposed Changes > `process/terminate_windows.go`, `job_windows.go` |
| R6 result.schema.json + schemaVersion | Proposed Changes > `docs/schemas/`, `process/runner.go` writeResultJSON |
| R7 Details marshal omit | Proposed Changes > `jsonutil.go` / writeResultJSON |
| R8 `--timeout-exit` / `--signal-exit` | Proposed Changes > `process/runner.go` mapExitCode, CLI |
| VS1–VS11 | Verification Plan > `tests/*_test.go` |
| YAML `-c` 等の任意 | **Deferred**（上記） |

## Proposed Changes

### Protocol (R3) — tests first

#### [NEW] [protocol/limits_test.go](file://protocol/limits_test.go)
*   **テストケース**:
    *   `TestMaxLineBytesDefault`: デフォルトが `1048576`
    *   `TestMaxLineBytesFromEnv`: `TIMEOUTX_MAX_LINE_BYTES=8192` で 8192
    *   `TestMaxLineBytesEnvTooSmall`: `4095` でエラー
    *   `TestMaxLineBytesEnvInvalid`: 非数でエラー
    *   `TestDecodeLineExceedsLimit`: 上限超過行がエラー

#### [MODIFY] [protocol/v1.go](file://protocol/v1.go)
*   **Description**: 行上限を実行時解決する。
*   **Technical Design**:
    ```go
    const DefaultMaxLineBytes = 1 << 20 // 1048576
    const MinMaxLineBytes = 4096

    // MaxLineBytes は互換のため残し、EffectiveMaxLineBytes() の初期値と一致させる。
    var MaxLineBytes = DefaultMaxLineBytes

    func EffectiveMaxLineBytes() (int, error)
    // TIMEOUTX_MAX_LINE_BYTES を読む。未設定なら DefaultMaxLineBytes。
    // 不正・下限未満は error。

    func DecodeLineWithLimit(line []byte, limit int) (Envelope, error)
    ```
*   **Logic**:
    *   `DecodeLine` は `EffectiveMaxLineBytes()` を使う（テストでは `t.Setenv`）。
    *   `ReadLoop` の Scanner buffer も同じ limit を使う。
    *   CLI / Runner 起動時に `EffectiveMaxLineBytes()` が error なら exit 125。

### Observer (R4) — tests first

#### [NEW] [observer_test.go](file://observer_test.go)
*   **テストケース**:
    *   `TestObserverDropNewest`: queue=1、遅い Observer、多数 emit → `EventsDropped()>0`、monitor 相当の emit がブロックしない
    *   `TestObserverDropOldest`: queue=1、DropOldest で最新が届く
    *   `TestObserverQueueDefault64`: size<=0 で 64

#### [MODIFY] [observer.go](file://observer.go)
*   **Technical Design**:
    ```go
    type DropPolicy int
    const (
        DropNewest DropPolicy = iota // default
        DropOldest
    )

    type asyncObserver struct {
        inner      Observer
        ch         chan Event
        dropPolicy DropPolicy
        dropped    atomic.Uint64
    }

    func (a *asyncObserver) EventsDropped() uint64
    func (a *asyncObserver) emit(ev Event) {
        select {
        case a.ch <- ev:
        default:
            if a.dropPolicy == DropOldest {
                select {
                case <-a.ch: // discard oldest
                default:
                }
                select {
                case a.ch <- ev:
                default:
                    a.dropped.Add(1)
                }
            } else {
                a.dropped.Add(1) // DropNewest
            }
        }
    }
    ```
*   **Logic**:
    *   `newAsyncObserver(inner, size, policy)`。size<=0 → 64。
    *   監視ループは従来どおり `emit` のみ呼び、block しない。

#### [MODIFY] [policy.go](file://policy.go) / [config.go](file://config.go)
*   **Technical Design**:
    ```go
    func WithObserver(o Observer) Option
    func WithObserverQueue(size int) Option
    func WithObserverDropPolicy(p DropPolicy) Option
    ```
*   **Logic**: Config に `observerQueue int`, `observerDrop DropPolicy` を保持。`Run` で `newAsyncObserver` に渡す。

#### [MODIFY] [result.go](file://result.go)
*   **Technical Design**:
    ```go
    type Result struct {
        // ... existing ...
        EventsDropped uint64
    }
    ```
*   **Logic**: `finish` 時に `observer.EventsDropped()` を Result へコピー。

### Details marshal (R7) — tests first

#### [NEW] [jsonutil.go](file://jsonutil.go) / [jsonutil_test.go](file://jsonutil_test.go)
*   **Technical Design**:
    ```go
    // MarshalDetailsJSON returns JSON for details, or (nil, false) on failure.
    func MarshalDetailsJSON(details any) (json.RawMessage, bool)
    ```
*   **Logic**:
    *   `details == nil` → `(nil, true)`（キー省略でよい）。
    *   `json.Marshal` 成功 → raw, true。
    *   失敗 → nil, false（呼び出し側がキー省略 + 警告）。
*   **テスト**: `chan int` / `func()` で false、map で true。

### Command Probe (R1) — tests first

#### [NEW] [probe_test.go](file://probe_test.go) / [probe.go](file://probe.go)
*   **Technical Design**:
    ```go
    package timeout // or package process — 採用: package timeout に Probe 設定、実行ループは process から呼ぶ

    type ProbeOnFailure string
    const (
        ProbeOnFailureIgnore    ProbeOnFailure = "ignore"
        ProbeOnFailureWarn      ProbeOnFailure = "warn"      // default
        ProbeOnFailureFailProbe ProbeOnFailure = "fail-probe"
    )

    type CommandProbe struct {
        Command    string
        Args       []string
        Interval   time.Duration // default 5s; <=0 invalid at start
        OnFailure  ProbeOnFailure
        EmptySignal string // "" | "heartbeat" | "progress"  // --probe-signal
        Verbose    bool
    }

    type ProbeDiagnostic struct {
        LastError     string
        SkippedOverlap int
        Failures      int
    }

    // StartCommandProbe runs until ctx done. Applies envelopes via apply func.
    func StartCommandProbe(ctx context.Context, p CommandProbe, apply func(protocol.Envelope), onFatal func(error), diag *ProbeDiagnostic)
    ```
*   **Logic**:
    1. ticker ごとに `exec.Command(p.Command, p.Args...)` を直接起動（shell なし）。
    2. 前回が未完了なら skip + `SkippedOverlap++`。
    3. Wait 後、exit≠0 → `Failures++`、`LastError` 更新。`warn` なら stderr、`fail-probe` なら `onFatal`。
    4. exit=0: stdout を `protocol.ReadLoop` 相当で行処理。許可 type は heartbeat/status/progress のみ。
    5. stdout 空かつ EmptySignal 設定時はその Signal を 1 回。
    6. stderr は Verbose または TTY のときプレフィックス `timeoutx: probe: ` で転送。

#### [MODIFY] [process/runner.go](file://process/runner.go)
*   **RunRequest 追加フィールド**:
    ```go
    Probes            []timeout.CommandProbe // or local struct
    EventsPath        string // "" | "-" | path
    EventsFD          int    // -1 = unset
    ProgressFormatJSON bool  // existing
    TimeoutExit       int    // default 124
    SignalExit        bool
    ProbeVerbose      bool
    ```
*   **Logic**: Execution 開始後に各 CommandProbe を `StartCommandProbe`。`onFatal` は Execution を cancel し `kind=probe` 相当の failed を Result に載せるため、専用 error を返すか Engine に `FailProbe(err)` を追加する。

#### FailProbe 経路（Engine）

#### [MODIFY] [execution.go](file://execution.go)
*   **Technical Design**:
    ```go
    // optional internal: requestProbeFailure(err) sets status failed, kind probe, cancels
    ```
*   **Logic**: Timeout と同様 once。`KindProbe` を Result に設定。CLI mapExitCode では Probe 失敗を 125 または非 0（仕様: status=failed, kind=probe → 内部的には failed。exit は 125 とする）。

### Events (R2)

#### [NEW] [process/events.go](file://process/events.go) / [process/events_test.go](file://process/events_test.go)
*   **Technical Design**:
    ```go
    type EventWriter struct { w io.Writer }

    func OpenEventWriter(path string, fd int) (*EventWriter, error)
    // fd >= 0 → os.NewFile(uintptr(fd), "events")
    // path == "-" → os.Stderr
    // path != "" → os.OpenFile append/create
    // path=="" && fd<0 → nil writer (disabled)

    func (e *EventWriter) WriteEvent(typ string, fields map[string]any) error
    // {"v":1,"type":typ,"ts":RFC3339, ...fields}\n
    // details は MarshalDetailsJSON; false なら省略
    ```
*   **Logic**:
    *   Runner が `WithObserver` で EventWriter を接続し、Engine Event を NDJSON に変換。
    *   EventType → snake_case type 名（既存定数の文字列をそのまま使うか、短い alias: `progress` / `timeout` / `finished`）。仕様例に合わせ、少なくとも `progress`, `timeout`, `finished` を出す。他は `string(EventType)` で可。
    *   子 stdout パイプとは別 writer。混線しない。

#### [MODIFY] [cmd/timeoutx/main.go](file://cmd/timeoutx/main.go)
*   **Flags 追加**:
    *   `--events PATH`
    *   `--events-fd N`
    *   `--probe CMD`（単純: 単一トークン。引数付きは後でスペース分割 `fields` または `sh -c` 禁止で `ProbeCommand` + 残りを `--probe-arg` で。**採用**: `--probe` はコマンド文字列 1 つ、追加引数は `--probe-arg` 繰り返し、または空白分割しないでパスのみ。統合テストは単一バイナリ helper を渡す）
    *   `--probe-every D`（default 5s）
    *   `--probe-signal heartbeat|progress`
    *   `--probe-verbose`
    *   `--timeout-exit CODE`（default 124）
    *   `--signal-exit`
*   **Logic**:
    *   `--events` と `--events-fd` 同時 → stderr メッセージ + return 125。
    *   `EffectiveMaxLineBytes()` 失敗 → 125。

### Exit mapping (R8)

#### [MODIFY] [process/runner.go](file://process/runner.go) `mapExitCode`
*   **Logic**:
    ```text
    if result.Status == timeout:
      if SignalExit && killedBySIGKILL:
        return 137
      return TimeoutExit  // default 124
    if result.Status == failed && result.Kind == probe:
      return 125
    // child exit (including 124) passthrough when not timeout
    ```
*   **Result JSON**: `schemaVersion: 1`, `exitCode`, `eventsDropped`（>0 のとき）, `probes` 診断配列。

#### [MODIFY] writeResultJSON
*   **Logic**:
    ```go
    type resultJSON struct {
        SchemaVersion int     `json:"schemaVersion"`
        Status        string  `json:"status"`
        Kind          string  `json:"kind"`
        ExitCode      int     `json:"exitCode"`
        EventsDropped uint64  `json:"eventsDropped,omitempty"`
        // ... existing ...
        Progress *progressJSON `json:"progress,omitempty"` // Details via MarshalDetailsJSON
    }
    ```

### Windows Job Object (R5)

#### [NEW] [process/job_windows.go](file://process/job_windows.go) (`//go:build windows`)
*   **Technical Design**:
    ```go
    type jobObject struct { handle syscall.Handle }

    func createJob() (*jobObject, error)
    func (j *jobObject) assign(pid uint32) error
    func (j *jobObject) terminate(exitCode uint32) error
    func (j *jobObject) close()
    ```
*   **Logic** (syscall / golang.org/x/sys/windows):
    1. `CreateJobObject`
    2. 子 Start 後 `AssignProcessToJobObject`
    3. Timeout: 猶予後 `TerminateJobObject`
    4. 失敗時 stderr 警告 + 既存 `proc.Kill()` フォールバック

#### [MODIFY] [process/terminate_windows.go](file://process/terminate_windows.go)
*   **Logic**: Runner が保持する `*jobObject` があれば `terminate`、なければ PID Kill。

#### [MODIFY] [process/runner.go](file://process/runner.go)
*   **Logic**: Windows ビルドで Job 作成・Assign を Start 直後に行う（build tag 付き関数 `attachJob(cmd) (closer, error)`）。

### Schemas (R6)

#### [NEW] [docs/schemas/result.schema.json](file://docs/schemas/result.schema.json)
*   **Logic**: JSON Schema 2020-12。required: `schemaVersion`, `status`, `kind`。`schemaVersion` const 1。

#### [NEW] [docs/schemas/event.schema.json](file://docs/schemas/event.schema.json)
*   **Logic**: required: `v`, `type`, `ts`。

#### [MODIFY] [README.md](file://README.md) / [docs/cli.md](file://docs/cli.md)
*   **更新内容**: 新フラグ、schema パス、Command Probe 例、`--timeout-exit` を追記。

## Step-by-Step Implementation Guide

1. **Protocol limits (TDD)**: `protocol/limits_test.go` を書き失敗させ、`EffectiveMaxLineBytes` と Decode 上限を実装する。
2. **jsonutil (TDD)**: Details marshal helper を実装する。
3. **Observer (TDD)**: DropNewest/DropOldest、queue、EventsDropped、Config Options を実装する。Result に `EventsDropped` を載せる。
4. **writeResultJSON**: `schemaVersion`, `exitCode`, Details omit、atomic rename は維持。
5. **EventWriter**: OpenEventWriter + Observer 接続。混線しないことを単体で確認。
6. **Command Probe (TDD)**: `probe.go` ループ、overlap skip、NDJSON 適用、on_failure。
7. **Runner 配線**: Probes、Events、TimeoutExit、SignalExit、Probe 失敗経路。
8. **CLI flags**: main.go に全フラグと相互排他チェック。
9. **Windows Job Object**: `job_windows.go` + terminate 経路。Unix は変更なし。
10. **Schemas + docs**: `docs/schemas/*.json`、README/cli.md 更新。
11. **Integration tests**: 下記 Verification Plan のテストを追加し、`build.sh` → `integration_test.sh --specify ...` を実行する。
12. **総合判定**（testing-rules §12）を記録して完了とする。

## Verification Plan

### Automated Verification

1. **Build & Unit Tests**:
    ```bash
    ./scripts/process/build.sh
    ```

2. **Integration Tests**（本リポジトリの `integration_test.sh` は `--specify` のみ。`--categories` は無し）:
    ```bash
    ./scripts/process/integration_test.sh --specify "TestProbe|TestEvents|TestTimeoutExit|TestProtocolLine|TestResultSchema|TestObserverDrop|TestDetailsMarshal|TestWindowsJob|TestTerminate|TestCLI"
    ```
    *   **Log Verification**: Timeout 時 stderr に kind が出ること。Event ファイルに子の `HELLO` が混ざらないこと。

3. **E2E / Integration テストコード**（`tests/`）:

    #### [NEW] [tests/probe_test.go](file://tests/probe_test.go)
    *   `TestProbe_ProgressPreventsStall` (VS1)
    *   `TestProbe_HeartbeatDoesNotPreventStall` (VS2)
    *   `TestProbe_FailureDoesNotFailJob` (VS3)

    #### [NEW] [tests/events_test.go](file://tests/events_test.go)
    *   `TestEvents_NotMixedWithChildStdout` (VS4)
    *   `TestEvents_ConflictFlagsExit125` (VS5)

    #### [NEW] [tests/protocol_line_test.go](file://tests/protocol_line_test.go)
    *   `TestProtocolLine_OversizeIgnored` (VS6) — helper が巨大行を Control FD へ書く

    #### [NEW] [tests/observer_drop_test.go](file://tests/observer_drop_test.go) または unit のみ
    *   統合が不要な場合は unit `TestObserverDropNewest` で足りる旨をここに明記。統合は任意。

    #### [NEW] [tests/result_schema_test.go](file://tests/result_schema_test.go)
    *   `TestResultSchema_SchemaVersion1` (VS9)
    *   schema ファイル存在確認

    #### [NEW] [tests/details_test.go](file://tests/details_test.go)
    *   `TestDetailsMarshal_OmitsOnFailure` (VS10) — ライブラリ JSON helper

    #### [NEW] [tests/timeout_exit_test.go](file://tests/timeout_exit_test.go)
    *   `TestTimeoutExit_Custom143` (VS11-2)
    *   `TestTimeoutExit_Child124Passthrough` (VS11-1)

    #### [NEW] [tests/windows_job_test.go](file://tests/windows_job_test.go) (`//go:build windows`)
    *   `TestWindowsJob_KillsGrandchild` (VS8)

### Test Design Self-Review (§11.4)

1. **網羅性**: R1–R8 それぞれにテスト名が対応。任意 YAML のみ先送り。
2. **証拠**: exit code + Result `status`/`kind`/`schemaVersion`/`exitCode` を assert。
3. **迂回排除**: Event と子 stdout を別キャプチャで比較。Probe heartbeat では Stall することを明示。
4. **依存**: protocol/jsonutil/observer unit → probe unit → CLI integration の順。

### 総合判定プロセス

全テスト成功後、testing-rules §12 チェックリストで判定を記録する（スキップ有無、Windows ビルドタグ、先送り任意要件の明示）。

## Documentation

#### [MODIFY] [docs/cli.md](file://docs/cli.md)
*   **更新内容**: `--events`, `--events-fd`, `--probe*`, `--timeout-exit`, `--signal-exit`, schema リンク

#### [MODIFY] [docs/library.md](file://docs/library.md)
*   **更新内容**: `WithObserverQueue`, `WithObserverDropPolicy`, EventsDropped

#### [MODIFY] [README.md](file://README.md)
*   **更新内容**: 新機能への短いポインタと `docs/schemas/` リンク

#### [NEW] [docs/schemas/result.schema.json](file://docs/schemas/result.schema.json)
#### [NEW] [docs/schemas/event.schema.json](file://docs/schemas/event.schema.json)
