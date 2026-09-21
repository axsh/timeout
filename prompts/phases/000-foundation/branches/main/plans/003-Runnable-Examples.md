# 003-Runnable-Examples

> **Source Specification**: `prompts/phases/000-foundation/branches/main/ideas/002-Runnable-Examples.md`

## Goal Description

`examples/go` と `examples/bash` に、それぞれ独立して実行できる 3 例を置き、`docs/library.md`、`docs/cli.md`、`README.md` から参照する。統合テストが各例の終了コードと標準出力を確認する。

## User Review Required

None. 任意の「各例 README」と「一括 go run スクリプト」は、各例に短い README を付ける範囲で実施し、一括スクリプトは作らない。

## Requirement Traceability

| Requirement (from Spec) | Implementation Point (Section/File) |
| :--- | :--- |
| R1 ディレクトリ構成 | Proposed Changes > examples tree |
| R2 Go 独立 module + replace | 各 `examples/go/*/go.mod` |
| R2.1 basic-run | `examples/go/basic-run/main.go` |
| R2.2 stall-heartbeat | `examples/go/stall-heartbeat/main.go` |
| R2.3 units | `examples/go/units/main.go` |
| R3 Bash と timeoutx 探索 | 各 `examples/bash/*/run.sh` |
| R3.1 shell-progress | `examples/bash/shell-progress/run.sh` |
| R3.2 heartbeat-on-output | `examples/bash/heartbeat-on-output/run.sh` |
| R3.3 progress-file | `examples/bash/progress-file/run.sh` |
| R4 docs リンク | `docs/library.md`, `docs/cli.md`, `README.md`, `examples/README.md` |
| R5 ビルド対象外 | 独立 `go.mod`（`build.sh` は変更しない） |
| 任意: 一括スクリプト | **Deferred**（例は個別 `go run` / `bash run.sh` で足りる） |

## Proposed Changes

### Go examples

各ディレクトリに同じ `go.mod`:

```text
module example

go 1.22

require github.com/axsh/timeout v0.0.0

replace github.com/axsh/timeout => ../../..
```

#### [NEW] [examples/go/basic-run/main.go](file://examples/go/basic-run/main.go)

*   **Logic**:
    1. `cfg := timeout.New(timeout.Hard(30*time.Second), timeout.Idle(10*time.Second), timeout.Stall(10*time.Second))`
    2. `timeout.Run(ctx, cfg, fn)` で `Current` を 1, 2, 3、`Total: 3`、`Stage: "import-users"` の Progress を送る。
    3. `fmt.Println(result.Status)`。期待文字列は `succeeded`。

#### [NEW] [examples/go/stall-heartbeat/main.go](file://examples/go/stall-heartbeat/main.go)

*   **Logic**:
    1. `timeout.New(timeout.Stall(time.Second))` のみ。
    2. ループで `Heartbeat()` と短い `Sleep`。`exec.Context().Done()` で return。
    3. `fmt.Println(result.Status, result.Kind)` → `timeout stall`。

#### [NEW] [examples/go/units/main.go](file://examples/go/units/main.go)

*   **Logic**:
    1. `timeout.New(timeout.UnitLimit(500*time.Millisecond))`。
    2. `exec.Unit("download", func(ctx) { <-ctx.Done(); return nil })`。
    3. `fmt.Println(result.Kind)` と `result.TimeoutUnit.Name` → `unit` と `download`。

### Bash examples

共通のバイナリ解決:

```bash
if [ -n "${TIMEOUTX:-}" ]; then
  :
elif [ -x "$ROOT/bin/timeoutx.exe" ]; then
  TIMEOUTX="$ROOT/bin/timeoutx.exe"
elif [ -x "$ROOT/bin/timeoutx" ]; then
  TIMEOUTX="$ROOT/bin/timeoutx"
else
  echo "timeoutx not found" >&2
  exit 1
fi
```

`ROOT` は `run.sh` から 3 階層上（`examples/bash/<name>` → repo root）。

#### [NEW] [examples/bash/shell-progress/run.sh](file://examples/bash/shell-progress/run.sh)

*   **Logic**:
    1. `TIMEOUTX_FD` が未設定なら `exec "$TIMEOUTX" run --idle 30s --stall 30s -- bash "$0"` で自分を子にする（再入防止）。
    2. `eval "$("$TIMEOUTX" shell-init)"`。
    3. `i` を 1..3 で `timeout_unit "import:$i" true`（Windows Git Bash では `true` が無い場合 `cmd /c exit 0` ではなく `:` を使う）し、`timeout_progress --current "$i" --total 3 --stage import-users "imported $i"`。

#### [NEW] [examples/bash/heartbeat-on-output/run.sh](file://examples/bash/heartbeat-on-output/run.sh)

*   **Logic**:
    1. `timeoutx run --idle 2s --heartbeat-on-output -- bash -c 'i=0; while [ "$i" -lt 8 ]; do echo tick; i=$((i+1)); sleep 0.4; done'`
    2. 子は 2 秒より長く出力し続ける。exit 0。

#### [NEW] [examples/bash/progress-file/run.sh](file://examples/bash/progress-file/run.sh)

*   **Logic**:
    1. 一時ディレクトリに `output.bin` の絶対パスを作る。
    2. `timeoutx run --stall 5s --progress-file "$file" -- bash -c 'sleep 0.3; echo x > "$1"; sleep 0.5; echo yy >> "$1"; sleep 0.2' _ "$file"`
    3. exit 0。

### Docs

#### [NEW] [examples/README.md](file://examples/README.md)

*   6 例へのリンクと実行コマンド（`go run .` / `bash run.sh`）。事前に `bin/timeoutx` をビルドする旨。

#### [MODIFY] [docs/library.md](file://docs/library.md)

*   Examples 節を追加。相対リンク `../examples/go/basic-run`、`../examples/go/stall-heartbeat`、`../examples/go/units`。

#### [MODIFY] [docs/cli.md](file://docs/cli.md)

*   Examples 節を追加。`../examples/bash/shell-progress`、`../examples/bash/heartbeat-on-output`、`../examples/bash/progress-file`。

#### [MODIFY] [README.md](file://README.md)

*   Documentation に `[Runnable examples](examples/README.md)`。

### Tests

#### [NEW] [tests/examples_test.go](file://tests/examples_test.go)

*   `TestExampleGoBasic` / `TestExampleGoStall` / `TestExampleGoUnit`: 各ディレクトリで `go run .`、stdout に期待文字列。
*   `TestExampleBashShell` / `TestExampleBashHeartbeat` / `TestExampleBashFile`: `bin/timeoutx` を `go build` で用意し `bash run.sh`、exit 0。`bash` が無い場合は `t.Fatalf`（Skip 禁止）。
*   `TestExampleDocsLinks`: `docs/library.md`、`docs/cli.md`、`README.md` に上記相対リンク文字列が含まれる。

## Step-by-Step Implementation Guide

1. 6 例の `go.mod` / `main.go` / `run.sh` と `examples/README.md` を追加する。
2. `docs/library.md`、`docs/cli.md`、`README.md` にリンクを書く。
3. `tests/examples_test.go` を追加する。
4. Verification Plan を実行する。
5. testing-rules §12 の総合判定を記録する。

## Verification Plan

### Automated Verification

1. **Build & Unit Tests**:

```bash
./scripts/process/build.sh
```

2. **Integration Tests**:

```bash
./scripts/process/integration_test.sh --specify "TestExample"
```

*   **Log Verification**: Go stall 例の stdout に `timeout` と `stall`。Unit 例に `unit` と `download`。Bash 例は exit 0。

3. **E2E Tests**:

GUI E2E ヘルパーは存在しない。例の実行そのものを `tests/examples_test.go` の統合テストとする。手動実行は代替にしない。

#### [NEW] [tests/examples_test.go](file://tests/examples_test.go)

*   **テストケース**: `TestExampleGoBasic`, `TestExampleGoStall`, `TestExampleGoUnit`, `TestExampleBashShell`, `TestExampleBashHeartbeat`, `TestExampleBashFile`, `TestExampleDocsLinks`
*   **検証ポイント**: 仕様 VS1–VS7 の出力と exit code

### Test Design Self-Review (§11.4)

1. **網羅性**: 6 例の実行と docs リンク文字列を別テストにする。
2. **証拠**: コンパイル成功だけでなく status/kind 文字列を assert する。
3. **迂回排除**: Bash は `timeoutx run` 経由。ドキュメントの断片を実行したことにしない。
4. **依存**: 公開 API のみ。ルート単体テストが先に通る前提。

### 総合判定プロセス

全テスト成功後、§12 チェックリスト（スキップ無し、例 module がルートテストに混ざっていないこと）を記録する。

## Documentation

#### [MODIFY] [docs/library.md](file://docs/library.md)
*   **更新内容**: Go 3 例へのリンク

#### [MODIFY] [docs/cli.md](file://docs/cli.md)
*   **更新内容**: Bash 3 例へのリンク

#### [MODIFY] [README.md](file://README.md)
*   **更新内容**: `examples/README.md` へのリンク

#### [NEW] [examples/README.md](file://examples/README.md)
*   **更新内容**: 索引と実行コマンド
