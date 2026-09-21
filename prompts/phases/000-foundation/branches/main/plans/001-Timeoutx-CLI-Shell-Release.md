# 001-Timeoutx-CLI-Shell-Release

> **Source Specification**: `prompts/phases/000-foundation/branches/main/ideas/000-Timeoutx-Execution-Policy-Engine.md`
> **Depends on**: `prompts/phases/000-foundation/branches/main/plans/000-Timeoutx-Core-Policy-Engine.md`

## Goal Description

`timeoutx` CLI、Control Protocol、Shell adapter、process group TERM→KILL、外形監視 Probe、GitHub Releases 用クロスコンパイル、および CLI/Protocol 統合テストを実装し、仕様の受け入れ条件 VS7–VS10 / VS13–VS16 を満たす。

## User Review Required

None.
Windows process tree 終了は仕様の未確定事項のため、v0.3 ではベストエフォート（子プロセスへの直接シグナル）とし、Unix の process group を必須とする。

## Requirement Traceability

| Requirement (from Spec) | Implementation Point |
| :--- | :--- |
| R9 CLI options / I/O | `cmd/timeoutx` |
| R10 Shell API | `shell/bash`, `shell/sh`, `shell-init` |
| R11 Control Protocol NDJSON | `protocol/v1.go` |
| R12 heartbeat/progress-on-output, file/command probe | `probe.go`, CLI flags |
| R13 TERM→KILL process group | `process/runner.go`, `terminate_unix.go` |
| R14 Exit codes 124–127 | CLI result mapping |
| R15 Result JSON | `--result` atomic write |
| R16 Progress display | `--show-progress` / `--progress-format` |
| R19.2 Release binaries | `.github/workflows/release.yml` |
| VS7–VS10, VS13–VS16 | `tests/*_test.go` |

## Proposed Changes

### Protocol

#### [NEW] [protocol/v1.go](file://protocol/v1.go) / [protocol/v1_test.go](file://protocol/v1_test.go)
*   **Technical Design**:
    ```go
    package protocol

    const Version = 1
    const MaxLineBytes = 1 << 20 // 1MiB provisional (未確定事項の仮決め)

    type Envelope struct {
        V       int             `json:"v"`
        Type    string          `json:"type"`
        Message string          `json:"message,omitempty"`
        Stage   string          `json:"stage,omitempty"`
        Current *int64          `json:"current,omitempty"`
        Total   *int64          `json:"total,omitempty"`
        Details json.RawMessage `json:"details,omitempty"`
        ID      uint64          `json:"id,omitempty"`
        Name    string          `json:"name,omitempty"`
    }

    func DecodeLine(line []byte) (Envelope, error)
    func EncodeHeartbeat() []byte
    // ... EncodeStatus, EncodeProgress, EncodeUnitBegin, EncodeUnitEnd
    ```
*   **Logic**:
    *   不正 JSON / 行長超過 / 未知必須 version → 診断として記録。デフォルトは warn and ignore。
    *   Reader は一行ずつ読み、callback で Engine に Signal を渡す。

### Process runner

#### [NEW] [process/runner.go](file://process/runner.go)
*   **Technical Design**:
    ```go
    type Runner struct {
        KillAfter time.Duration // default 10s
    }

    type RunRequest struct {
        Ctx     context.Context
        Config  timeout.Config
        Command string
        Args    []string
        Env     []string
        Dir     string
        // flags: HeartbeatOnOutput, ProgressOnOutput, ProgressFile, Probes...
        ResultPath string
    }

    func (r *Runner) Run(req RunRequest) (timeout.Result, int /*exitCode*/)
    ```
*   **Logic**:
    1. `timeout.Run` 内で子プロセスを起動するか、Runner が Engine を駆動する。
    2. 子に Control FD を継承し `TIMEOUTX_FD` / `TIMEOUTX_PROTOCOL=1` をセット。
    3. stdout/stderr 透過。オプションで出力を Heartbeat/Progress に変換。
    4. Timeout 時: Snapshot 確定 → process group に SIGTERM → KillAfter 待ち → SIGKILL。
    5. Exit: 子の code を透過。Policy timeout → 124。内部エラー → 125。exec 失敗 → 126/127。

#### [NEW] [process/terminate_unix.go](file://process/terminate_unix.go) (`//go:build unix`)
*   **Logic**: `Setpgid: true` で起動。`syscall.Kill(-pid, SIGTERM/SIGKILL)`。

#### [NEW] [process/terminate_windows.go](file://process/terminate_windows.go) (`//go:build windows`)
*   **Logic**: 子プロセスへ直接 kill（ベストエフォート）。process tree 完全対応は未確定のまま。

### Probe

#### [NEW] [probe.go](file://probe.go) / [probe_test.go](file://probe_test.go)
*   **Logic**:
    *   File probe: interval ごとに size / mtime を見て変化したら Progress（または設定された signal）。
    *   ファイル未存在 = Progress なし。
    *   Command probe: interval で外部コマンド実行。exit 0 かつ stdout 変化等は実装時に簡易プロトコル（exit 0 = heartbeat、stdout に `PROGRESS current total` 行があれば Progress）とする（未確定事項の仮決め）。
    *   Probe 失敗は Result 上で監視対象失敗と区別（診断フィールド）。

### CLI

#### [MODIFY] [cmd/timeoutx/main.go](file://cmd/timeoutx/main.go) + subcommands
*   **Commands**:
    *   `timeoutx run [flags] -- command...`
    *   `timeoutx shell-init`
    *   `timeoutx version`
*   **Flags**: `--hard`, `--idle`, `--stall`, `--unit`, `--kill-after` (default 10s), `--result`, `--show-progress`, `--progress-format`, `-c/--config`, `--heartbeat-on-output`, `--progress-on-output`, `--progress-file`, `--probe`, `--probe-every`
*   **Logic**:
    *   Duration パースは `time.ParseDuration`。
    *   全 Policy disabled 時は stderr に警告、実行は許可。
    *   YAML config (`gopkg.in/yaml.v3`): hard/idle/stall/unit/probes。

### Shell

#### [NEW] [shell/bash](file://shell/bash) / [shell/sh](file://shell/sh)
*   **Logic**: `shell-init` が関数定義を stdout に出す。
    *   `timeout_heartbeat` → NDJSON heartbeat を `$TIMEOUTX_FD` へ
    *   `timeout_status`, `timeout_progress`, `timeout_unit`, `timeout_unit_begin/end`
*   Shell は監視ロジックを持たない。

### Release

#### [NEW] [.github/workflows/release.yml](file://.github/workflows/release.yml)
*   **Logic**: tag `v*` で linux/darwin/windows × amd64/arm64 を cross-compile、`SHA256SUMS` 生成、GitHub Release に upload。

### Integration / E2E tests

#### [NEW] [tests/go.mod](file://tests/go.mod)
```text
module github.com/axsh/timeout/tests

go 1.22

require github.com/axsh/timeout v0.0.0
replace github.com/axsh/timeout => ../
```

#### [NEW] tests (examples)
*   `tests/cli_run_test.go`: Hard timeout で exit 124、Result kind
*   `tests/cli_shell_test.go`: shell-init + progress via FD
*   `tests/cli_terminate_test.go`: process group TERM→KILL（unix）
*   `tests/cli_exitcode_test.go`: 124/126/127
*   `tests/probe_file_test.go`: progress-file
*   `tests/goapi_cli_parity_test.go`: 同じ Signal 列で Kind 一致

## Step-by-Step Implementation Guide

1. Implement `protocol` with unit tests.
2. Implement `process` runner + unix terminate; windows stub.
3. Wire CLI `run` / `shell-init` / `version`.
4. Add shell script libraries.
5. Implement probes + CLI flags.
6. Add `--result` atomic JSON writer.
7. Write integration tests under `tests/`.
8. Add GitHub release workflow.
9. Run full Verification Plan + §12 総合判定.
10. `git push` after record-far-knowledge.

## Verification Plan

### Automated Verification

1. **Build & Unit Tests**:
    ```bash
    ./scripts/process/build.sh
    ```

2. **Integration Tests** (selective):
    ```bash
    ./scripts/process/integration_test.sh --specify "TestCLI|TestShell|TestProtocol|TestTerminate|TestExitCode|TestProbe|TestParity|TestModuleImport"
    ```

3. **E2E Tests**:
    #### [NEW] [tests/cli_run_test.go](file://tests/cli_run_test.go) 他上記
    *   手動コマンド確認は代替にしない。すべて `tests/` の Go テストとする。

### Test Design Self-Review (§11.4)

1. Protocol decode → Engine signal → CLI exit/result → process kill のボトムアップ。
2. Exit 124 と kind=stall の両方を assert し偽成功を防ぐ。
3. Shell は実際に `timeoutx` バイナリを起動して FD 通信する。

### 総合判定プロセス

全テスト成功後、testing-rules §12 チェックリストで判定を記録する。

## Documentation

#### [MODIFY] [README.md](file://README.md), [docs/cli.md](file://docs/cli.md), [docs/library.md](file://docs/library.md)
*   **更新内容**: 実装済みフラグ・Release URL・導入手順が事実と一致するよう更新。
