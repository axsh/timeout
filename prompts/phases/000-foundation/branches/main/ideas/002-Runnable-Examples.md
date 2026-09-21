# 002 Runnable Examples for Go and Bash

> **Parent**: `prompts/phases/000-foundation/branches/main/ideas/000-Timeoutx-Execution-Policy-Engine.md`
>
> `docs/library.md` と `docs/cli.md` の説明を、リポジトリ内でそのまま実行できる例へつなぐ。

## 背景 (Background)

利用者向け説明は `README.md`、`docs/library.md`、`docs/cli.md` にある。
これらは短いコード断片であり、コピーして依存解決するまで動かない。

初心者が「設定を関数より前に置く Go API」と「Shell から Signal を送る CLI」を、手元で実行して確認できるようにする。
例はドキュメント本文に埋め込まず、`examples/` に置き、`docs/` から参照する。

## 要件 (Requirements)

### 必須要件

#### R1. ディレクトリ構成

次のツリーをリポジトリルートに置く。例はテーマごとに 1 ディレクトリとする。

```text
examples/
├── README.md
├── go/
│   ├── basic-run/
│   ├── stall-heartbeat/
│   └── units/
└── bash/
    ├── shell-progress/
    ├── heartbeat-on-output/
    └── progress-file/
```

- `examples/go/` と `examples/bash/` 以外の言語ディレクトリは作らない。
- 各例ディレクトリは自己完結する。他の例のファイルに依存しない。
- 例の説明文・コメントは英語とする。

#### R2. Go 例

各 Go 例は独立した module とする。ルート `go.mod`（`github.com/axsh/timeout`）のパッケージ一覧には含めない。

```text
module example

go 1.22

require github.com/axsh/timeout v0.0.0

replace github.com/axsh/timeout => ../../..
```

`replace` の相対パスは、その例ディレクトリからリポジトリルートを指す。

##### R2.1 `examples/go/basic-run`

設定を関数より前に置き、成功する実行を示す。

1. `timeout.New` に `Hard(30*time.Second)`、`Idle(10*time.Second)`、`Stall(10*time.Second)` を渡す。
2. `timeout.Run(ctx, cfg, fn)` で 3 ステップ分の定量 Progress（`Current` を 1, 2, 3、`Total` は 3、`Stage` は `import-users`）を送る。
3. 終了時に `result.Status` を標準出力へ書く。期待は `succeeded`。

##### R2.2 `examples/go/stall-heartbeat`

Heartbeat だけでは Stall を解除できないことを示す。

1. `Stall(1*time.Second)` のみ有効にする。
2. 関数内で `Heartbeat()` を短い間隔で送り続ける。
3. `Execution.Context()` が cancel されたら戻る。
4. `result.Status` が `timeout`、`result.Kind` が `stall` であることを標準出力へ書く。

##### R2.3 `examples/go/units`

Unit ごとの Hard Timeout を示す。

1. `UnitLimit(500*time.Millisecond)` のみ有効にする。
2. `exec.Unit("download", ...)` の中で Context が cancel されるまで待つ。
3. `result.Kind` が `unit` であり、`result.TimeoutUnit.Name` が `download` であることを標準出力へ書く。

各 Go 例の実行方法:

```bash
cd examples/go/basic-run
go run .
```

#### R3. Bash 例

各 Bash 例は `run.sh`（実行エントリ）を持つ。`timeoutx` は次の順で探す。

1. 環境変数 `TIMEOUTX` が指す実行ファイル
2. リポジトリの `bin/timeoutx`（Windows では `bin/timeoutx.exe`）

見つからなければ、メッセージを stderr に出して exit 1 する。

##### R3.1 `examples/bash/shell-progress`

1. `eval "$("$TIMEOUTX" shell-init)"` する。
2. 3 回ループし、各回で `timeout_unit` と定量 `timeout_progress`（`--current` / `--total 3` / `--stage import-users`）を送る。
3. `timeoutx run --idle 30s --stall 30s --` でこのスクリプトを起動する。
4. 正常終了（exit 0）する。

##### R3.2 `examples/bash/heartbeat-on-output`

1. 子コマンドが短い間隔で stdout に 1 行書く（合計数秒以内に終了）。
2. `timeoutx run --idle 2s --heartbeat-on-output --` でそのコマンドを起動する。
3. 出力が Heartbeat として扱われ、Idle timeout せず exit 0 で終わる。

##### R3.3 `examples/bash/progress-file`

1. 作業ディレクトリに `output.bin` を作り、途中でサイズを増やす子コマンドを起動する。
2. `timeoutx run --stall 5s --progress-file ./output.bin --` で起動する。
3. ファイル更新が Progress になり、Stall timeout せず exit 0 で終わる。

Bash 例は `bash run.sh` で起動できること。Windows の Git Bash でも、`bin/timeoutx.exe` が見つかれば動くこと。

#### R4. docs からの参照

本文の長いサンプルを例ファイルの代替にしない。各ガイドから、対応する例ディレクトリへ相対リンクする。

| ドキュメント | 参照先 |
|---|---|
| `docs/library.md` | `examples/go/basic-run`、`examples/go/stall-heartbeat`、`examples/go/units` |
| `docs/cli.md` | `examples/bash/shell-progress`、`examples/bash/heartbeat-on-output`、`examples/bash/progress-file` |
| `README.md` | `examples/README.md`（Documentation 節から 1 リンク） |
| `examples/README.md` | 上記 6 例への索引と実行コマンド |

リンクはリポジトリルートからの相対パスを Markdown リンクで書く。例:

```markdown
[basic-run](../examples/go/basic-run)
```

`docs/library.md` から見た相対パスは `../examples/go/basic-run` である。

#### R5. ビルド対象外

- `scripts/process/build.sh` のルート `go test ./...` に、例 module を含めない（独立 `go.mod` により自然に除外される）。
- 例はライブラリの公開 API を import するだけとし、`internal` 相当の未公開識別子に依存しない。

### 任意要件

- 各例ディレクトリに 10 行以内の `README.md`（実行コマンドのみ）
- `examples/go` をまとめて `go run` する補助スクリプト

## 実現方針 (Implementation Approach)

```text
docs/library.md  --link-->  examples/go/<name>/main.go
docs/cli.md      --link-->  examples/bash/<name>/run.sh
README.md        --link-->  examples/README.md
```

- Go 例の `main` は標準出力に最終 `status` と、必要なとき `kind` を 1 行で書く。テストはこの行を assert する。
- Bash 例の成否はプロセス exit code で判定する。
- Stall / Unit の例は実時間 1 秒前後で終わる長さにし、CI で待ちすぎない。

設計上の決定:

- 例はルート module に混ぜない。`replace` でローカルの `github.com/axsh/timeout` を参照する。
- ドキュメントの短い断片は残してよい。動く完全例の正本は `examples/` とする。
- Bash 例は Policy Engine を再実装しない。`timeoutx` と `shell-init` だけを使う。

## 検証シナリオ (Verification Scenarios)

### VS1. basic-run が成功する

1. リポジトリルートで `go build -o bin/timeoutx ./cmd/timeoutx` は不要（この例はライブラリのみ）。
2. `examples/go/basic-run` で `go run .` を実行する。
3. exit 0 であり、標準出力に `succeeded` が含まれる。

### VS2. Heartbeat では Stall を回避できない

1. `examples/go/stall-heartbeat` で `go run .` を実行する。
2. 約 1 秒で終了する。
3. 標準出力に `timeout` と `stall` が含まれる。

### VS3. Unit timeout

1. `examples/go/units` で `go run .` を実行する。
2. 標準出力に `unit` と `download` が含まれる。

### VS4. Shell progress

1. `scripts/process/build.sh` 済み、または同等に `bin/timeoutx` がある。
2. `examples/bash/shell-progress` で `bash run.sh` を実行する。
3. exit 0 である。

### VS5. heartbeat-on-output

1. `examples/bash/heartbeat-on-output` で `bash run.sh` を実行する。
2. Idle 2s より長く出力が続く子でも、出力のたびに Idle が更新され、exit 0 である。

### VS6. progress-file

1. `examples/bash/progress-file` で `bash run.sh` を実行する。
2. ファイルが成長している間は Stall せず、exit 0 である。

### VS7. docs から辿れる

1. `docs/library.md` に 3 つの Go 例へのリンクがある。
2. `docs/cli.md` に 3 つの Bash 例へのリンクがある。
3. `README.md` の Documentation から `examples/README.md` へリンクがある。

## テスト項目 (Testing for the Requirements)

### ビルド・全体検証

1. ビルド＋単体テスト（例 module がルートテストを壊さないこと）:

```bash
scripts/process/build.sh
```

2. 例の実行確認:

```bash
scripts/process/integration_test.sh --specify "TestExample"
```

統合テストは `tests/` から各例を `go run` / `bash run.sh` し、VS1–VS6 の出力と exit code を assert する。
`bin/timeoutx` が無い場合、テストは `go build -o` で作ってから Bash 例を実行する（`t.Skip` は使わない）。

### 要件と検証の対応

| 要件 | シナリオ | 自動化 |
|---|---|---|
| R2.1 basic-run | VS1 | `--specify "TestExampleGoBasic"` |
| R2.2 stall-heartbeat | VS2 | `--specify "TestExampleGoStall"` |
| R2.3 units | VS3 | `--specify "TestExampleGoUnit"` |
| R3.1 shell-progress | VS4 | `--specify "TestExampleBashShell"` |
| R3.2 heartbeat-on-output | VS5 | `--specify "TestExampleBashHeartbeat"` |
| R3.3 progress-file | VS6 | `--specify "TestExampleBashFile"` |
| R4 docs リンク | VS7 | ファイル内容の存在確認を `TestExampleDocsLinks` で行う |
| R5 ビルド対象外 | VS1 前 | `scripts/process/build.sh` |

### テスト設計セルフレビュー

1. **網羅性**: 6 例それぞれを実行し、期待する status/kind または exit 0 を確認すれば「動く例」と言える。
2. **証拠**: コンパイル成功だけでなく、標準出力の `stall` / `unit` を assert する。
3. **迂回排除**: Bash 例は実際に `timeoutx run` を起動する。ドキュメントのコードブロック文字列一致だけでは成功にしない。
4. **依存**: ライブラリ単体テストが先に通っている前提で、例は公開 API だけを呼ぶ。
