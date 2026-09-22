# 003 Timeoutx Version Embedding and GitHub Release Script

> **Parent**: `prompts/phases/000-foundation/branches/main/ideas/000-Timeoutx-Execution-Policy-Engine.md`
>
> 編集可能なバージョン番号を正本化し、ライブラリ・CLI・Git tag / GitHub Release で共有する。
> `timeoutx` への埋め込み表示と、その番号で tag を打つ Release スクリプトを追加する。

## 背景 (Background)

現状のバージョン表示は `cmd/timeoutx/main.go` 内の定数 `const version = "0.3.0-dev"` にハードコードされている。
ビルド ID（commit SHA）は埋め込まれておらず、`timeoutx version` はバージョン文字列だけを出す。
公開 Go module `github.com/axsh/timeout` 側にも、実行時に参照できるバージョン文字列がない。

GitHub Releases 向けのクロスコンパイルは `.github/workflows/release.yml` が tag `v*` で動作するが、
次の点が不足している。

1. リポジトリ内に「現在のバージョン」をテキストで編集できる単一の正本がない
2. ビルド成果物にバージョンと commit ID が埋め込まれない（`ldflags` 未使用）
3. ローカルから同じ規則で Release を切るスクリプトがない（CI のみ）
4. ライブラリ利用者が参照する版と、CLI / GitHub Release / `go get` 用タグが別管理になりうる

利用者が「どの版のバイナリ／ライブラリか」を確認でき、メンテナが同じ番号で tag・Release・module 版を切れる状態にする。

### Go module との関係（可能である理由）

Go の module バージョンは `go.mod` に自モジュール版を書かず、**Git タグ `vX.Y.Z`** が公開バージョンになる。

例: `go get github.com/axsh/timeout@v0.3.0` は、リポジトリの tag `v0.3.0` を指す。

したがって次の一本化が可能である。

```text
VERSION（例: 0.3.0）
   │
   ├─► timeout.Version（ライブラリ API・go:embed）
   ├─► timeoutx version 表示（CLI）
   └─► git tag / GitHub Release 名 v0.3.0（= go get 可能な module 版）
```

`VERSION` を正本にし、`release.sh` が `v{VERSION}` を打つ設計は、ライブラリ版とリリースタグの両方にそのまま使える。

---

## 要件 (Requirements)

### 必須要件

#### R1. バージョン正本ファイル

リポジトリルートに編集可能なテキストファイル `VERSION` を置く。

- 内容は **1 行のみ**。前後の空白は無視してよいが、改行以外の余分な文字は含めない
- 形式は SemVer 風の文字列とする（例: `0.3.0`、開発中は `0.4.0-dev` も可）
- 先頭の `v` は付けない（正本は `0.3.0`、Git tag / module 版は `v0.3.0`）
- このファイルがプロジェクトの「現在のバージョン」の唯一の正本である
- 次のすべてが同じ `VERSION` を参照する:
  1. ライブラリ API（R1.1）
  2. CLI `timeoutx version`（R3）
  3. `release.sh` が打つ Git tag および GitHub Release 名（R4.2）
- `cmd/timeoutx/main.go` 内のハードコード定数は廃止し、ライブラリ側の値を使う

初期値は現状の意図に合わせて `0.3.0-dev` とする（実装時に現状定数と揃える）。

##### R1.1 ライブラリ API

ルート package `timeout`（`github.com/axsh/timeout`）から、埋め込み済みのバージョン文字列を公開する。

```go
// 利用者からの参照例
fmt.Println(timeout.Version)
```

- `VERSION` はルート package と同じディレクトリにあるため、`//go:embed VERSION` で取り込む（利用者が `go get` したときも ldflags 不要で値が入る）
- 公開識別子は `Version`（string）。前後空白・末尾改行は trim してから使う
- protocol の `protocol.Version`（エンベロープ形式の整数）とは別物であり、名前空間も `protocol` のまま変更しない
- ライブラリ利用者向けに `docs/library.md` へ一行以上の言及を入れる

補足: Go module としての解決バージョン（`go.mod` の `require github.com/axsh/timeout v0.3.0`）は Git tag 由来であり、`timeout.Version` と一致させるのは `release.sh` が `VERSION` から `v{VERSION}` を打つことで保証する。

#### R2. バイナリへの埋め込み（CLI build ID）

`timeoutx` ビルド時に次を満たす。

| 値 | 意味 | 取得元 |
|---|---|---|
| version | リリース／開発バージョン | `timeout.Version`（= `VERSION` の embed） |
| commit（build ID） | ビルド時点の Git commit | `git rev-parse --short HEAD`（7 文字以上の短縮 SHA）を `-ldflags` で CLI に注入 |

- version はライブラリ embed を正とし、CLI 用に二重管理しない
- commit だけは CLI の `main` パッケージへ `-ldflags "-X main.commit=..."` で注入する（ライブラリ利用時の ldflags は期待できないため、commit の公開は CLI 必須・ライブラリは任意）
- `scripts/process/build.sh` で `bin/timeoutx` を作るときも commit を埋め込む
- `.github/workflows/release.yml` を残す場合も同様
- ldflags 未指定の素の `go build` / `go install` では commit は `unknown` とし、パニックしない。version は embed により `VERSION` の内容になる

dirty 作業ツリーの扱いは任意（任意要件 R7）とする。必須では commit に dirty 接尾辞を付けなくてよい。

#### R3. `timeoutx version` の表示

既存サブコマンド `version` / `--version` / `-V` の出力を、バージョンと build ID の両方を含む形に更新する。

推奨フォーマット（1 行）:

```text
timeoutx 0.3.0-dev (commit abcdef1)
```

ここでのバージョン部分は `timeout.Version` と同一である。

制約:

- 標準出力へ出す（stderr ではない）
- exit code は 0
- `help` の説明文は「print version」のままでよいが、実際の出力には commit が含まれることをドキュメント（`docs/cli.md` または `README.md`）に 1 箇所以上記載する

#### R4. GitHub Release スクリプト

`scripts/process/release.sh` を新設する。`VERSION` の番号を使ってマルチプラットフォームの `timeoutx` バイナリをビルドし、GitHub Release を作成する。

##### R4.1 前提・入力

- 依存コマンド: `go`, `git`, `gh`, `sha256sum`（または同等）
- リポジトリルート、またはそこへ解決できる場所から実行する
- GitHub 認証は `gh` の既存ログイン（または `GH_TOKEN`）に任せる
- 対象リポジトリは `origin` が指す GitHub リポジトリ（現状 `axsh/timeout`）

##### R4.2 バージョンと tag（ライブラリ module 版と同一）

1. `VERSION` を読む（trim 後が空ならエラー終了）
2. tag 名は `v` + `VERSION`（例: `VERSION=0.3.0` → tag `v0.3.0`）
3. この tag が **Go module の公開バージョン** でもある（`go get github.com/axsh/timeout@v0.3.0` が同じ commit を指す）
4. `VERSION` が `-dev` を含む場合は **リリースを拒否** して非 0 で終了する（誤公開防止。module に `-dev` タグを出さない）
5. 作業ツリーが dirty の場合は拒否する（未コミット変更での公開を防ぐ）
6. 既に同名 tag / Release が存在する場合は拒否する（上書きしない）
7. リリース直前に、埋め込み後の `timeout.Version`（またはビルドした `timeoutx version`）が `VERSION` と一致することを確認してから tag を打つ（不一致なら中止）

##### R4.3 成果物

既存の R19.2 / README と揃える。

| OS | Arch | ファイル名 |
|---|---|---|
| Linux | amd64 | `timeoutx_linux_amd64` |
| Linux | arm64 | `timeoutx_linux_arm64` |
| macOS | amd64 | `timeoutx_darwin_amd64` |
| macOS | arm64 | `timeoutx_darwin_arm64` |
| Windows | amd64 | `timeoutx_windows_amd64.exe` |
| Windows | arm64 | `timeoutx_windows_arm64.exe` |

- 各バイナリに R2 の version / commit を埋め込む
- `SHA256SUMS` を同じ成果物セットに含める
- 一時成果物ディレクトリ（例: `dist/`）に出力し、Release アップロード後に残してよい（gitignore 済みであること）

##### R4.4 Release 作成手順（スクリプトの振る舞い）

標準フロー:

1. `VERSION` 検証（R4.2）
2. 現在の `HEAD` に annotated または lightweight tag `v{VERSION}` を付ける（未 push のローカル tag）
3. 6 プラットフォームをクロスコンパイル（`CGO_ENABLED=0`, `-trimpath`, サイズ削減 ldflags は既存 CI に合わせてよい）
4. `SHA256SUMS` 生成
5. `git push origin v{VERSION}`（リモートへ tag を送る）
6. `gh release create v{VERSION}` でバイナリ群と `SHA256SUMS` を添付する

オプション:

- `--dry-run`: tag 作成・push・`gh release create` を行わず、ビルドと checksum まで行う
- `--skip-push`: tag はローカルのみ、push と `gh` をスキップ（ビルド検証用）

既存の `.github/workflows/release.yml` との関係:

- スクリプトが tag push すると CI も動く可能性があるため、**二重アップロードを避ける**こと
- 方針（実装でどちらか一方を選ぶ。仕様として必須）:
  - **A案（推奨）**: `release.sh` がビルド＋`gh release create` まで行い、CI の `release.yml` は同じ埋め込み規則に更新したうえで「tag だけでは upload せず、スクリプト利用を正」にする、または CI を削除／無効化する
  - **B案**: `release.sh` は tag 作成と push のみ行い、成果物生成は既存 CI に任せる。その場合でも CI は `VERSION` と commit を ldflags で埋め込むこと
- 採用した案を実装計画と README の Release 節に明記する

本仕様の既定は **A案** とする（ユーザーが求めた「スクリプトでバイナリを出す」に直接対応するため）。

#### R5. ドキュメント更新

- `README.md` の Install / Build 周辺に、`VERSION` が正本であること、`timeoutx version` で version + commit が見られること、`go get ...@vX.Y.Z` のタグが同じ `VERSION` 由来であることを短く追記する
- Release 手順として `scripts/process/release.sh` の使い方を数行で記載する（「VERSION を直す → スクリプトが `v{VERSION}` tag を打つ → module と GitHub Release が揃う」）
- `docs/cli.md` に `version` サブコマンドの出力例を 1 つ載せる
- `docs/library.md` に `timeout.Version` の参照例を 1 つ載せる

### 任意要件

#### R6. 長い commit（フル SHA）

環境変数やフラグでフル SHA を埋め込むオプションは任意。必須は短縮 SHA でよい。

#### R7. dirty 接尾辞

`git` が dirty なときのローカル `build.sh` で commit を `abcdef1-dirty` にするのは任意。
`release.sh` は dirty を拒否すれば足りる。

#### R8. ライブラリからの Commit 公開

`timeout.Commit` を公開するのは任意。必須は CLI の build ID 表示でよい。
ライブラリへ commit を載せる場合はリリース時 codegen か同様の手段が必要（通常の `go get` では ldflags が効かない）。

---

## 実現方針 (Implementation Approach)

### コンポーネント概要

```text
VERSION                                 # 正本（人が編集）
   │
   ├─► //go:embed → timeout.Version     # ライブラリ API
   │                    │
   │                    └─► timeoutx version 表示の version 部分
   │
   ├─► scripts/process/build.sh         # commit のみ -ldflags → bin/timeoutx
   ├─► scripts/process/release.sh       # tag v{VERSION} + バイナリ + gh release
   │         (= go get @v{VERSION} の module 版)
   └─► .github/workflows/release.yml    # A案では無効化 or 役割分担を明記

cmd/timeoutx/main.go
  version ← timeout.Version
  commit  ← ldflags（無指定時 "unknown"）
```

### 設計上の決定

1. **正本はルート `VERSION` 1 ファイル**。Go 定数や CI 環境変数を正本にしない
2. **Git tag は常に `v` + VERSION**。これが GitHub Release 名かつ Go module 版になる。`VERSION` 自体には `v` を書かない
3. **開発用サフィックス `-dev` の Release / tag は禁止**。正式公開前に `VERSION` を `x.y.z` に直してから `release.sh` を走らせる
4. **ライブラリ版文字列は `go:embed`**。消費者が `go get` する経路では ldflags が使えないため
5. **CLI の commit（build ID）だけ ldflags**。version はライブラリと共有
6. **Result JSON や protocol エンベロープに product version は足さない**（本仕様の範囲外。`protocol.Version` は別物）
7. **Release の正経路は `scripts/process/release.sh`（A案）**。CI との二重公開を避ける

### 主要な変更ファイル（予定）

| パス | 変更 |
|---|---|
| `VERSION` | 新規。初期値 `0.3.0-dev` |
| `version.go`（ルート package） | `//go:embed VERSION` と公開 `Version` |
| `cmd/timeoutx/main.go` | `timeout.Version` + `commit` 表示。ハードコード削除 |
| `scripts/process/build.sh` | commit の ldflags 埋め込み |
| `scripts/process/release.sh` | 新規（tag = module 版） |
| `.github/workflows/release.yml` | A案に合わせて無効化または埋め込みのみに整理 |
| `README.md` / `docs/cli.md` / `docs/library.md` | 表示例・`timeout.Version`・Release / tag 手順 |
| `.gitignore` | `dist/` を追加（未追加なら） |
| `tests/` | version / release の検証 |

### CLI commit 埋め込みの想定

```bash
COMMIT=$(git rev-parse --short HEAD)
LDFLAGS="-s -w -X main.commit=${COMMIT}"
go build -trimpath -ldflags "$LDFLAGS" -o bin/timeoutx ./cmd/timeoutx
# version 文字列は timeout.Version（embed）から取得
```

---

## 検証シナリオ (Verification Scenarios)

### VS1. VERSION を編集すると表示が変わる

1. `VERSION` を一時的に `9.9.9-test` にする（テスト後に戻す）
2. `scripts/process/build.sh` 相当で `bin/timeoutx` をビルドする
3. `bin/timeoutx version`（または `bin/timeoutx.exe version`）を実行する
4. 標準出力に `9.9.9-test` と、現在の短縮 commit が含まれる
5. `VERSION` を元に戻す

### VS2. ldflags 無しビルドでも落ちない

1. `go build -o /tmp/timeoutx-plain ./cmd/timeoutx` のように ldflags 無しでビルドする
2. `version` を実行する
3. exit 0 であり、フォールバック version と `unknown`（または同等）が表示される

### VS3. `-dev` では release できない

1. `VERSION` が `0.3.0-dev` の状態で `scripts/process/release.sh --dry-run` または本実行を試みる
2. スクリプトはエラーメッセージを出して非 0 で終了する
3. tag も Release も作られない

### VS4. 正式 VERSION での dry-run

1. クリーンな作業ツリーで `VERSION` を `0.3.0`（または未使用のテスト用番号）にする
2. `scripts/process/release.sh --dry-run` を実行する
3. 6 プラットフォーム分のバイナリと `SHA256SUMS` が `dist/`（または同等）にできる
4. いずれかのバイナリで `version` を実行し、埋め込み VERSION と commit が一致する（Windows バイナリは実行環境が合わなければファイル存在＋Linux/darwin 成果物で確認）
5. GitHub Release とリモート tag は作られない

### VS5. ドキュメント

1. `README.md` に `VERSION` と `scripts/process/release.sh` への言及がある
2. `docs/cli.md` に `timeoutx version` の出力例がある
3. `docs/library.md` に `timeout.Version` の言及がある

### VS6. ライブラリ Version と VERSION ファイルが一致する

1. ルート package のテスト、または小さなプログラムで `timeout.Version` を読む
2. リポジトリの `VERSION`（trim 後）と文字列が一致する
3. `VERSION` を一時変更して再テストすると、embed 側も追従する（テスト後に戻す）

### VS7. リリースタグが module 版になる

1. `VERSION=0.3.0` のとき、`release.sh`（またはその dry-run のタグ名計算）が `v0.3.0` を出す
2. ドキュメントまたはスクリプトコメントに、この tag が `go get github.com/axsh/timeout@v0.3.0` 用であることが書かれている
3. 実リモートへの tag push は本仕様の自動テストでは行わない（名前規則の検証に留める）

---

## テスト項目 (Testing for the Requirements)

### ビルド・全体検証

1. ビルド＋単体テスト:

```bash
scripts/process/build.sh
```

2. CLI version 表示の統合テスト:

```bash
scripts/process/integration_test.sh --specify "TestVersion"
```

3. Release スクリプトの dry-run / ガード（統合またはスクリプト単体テスト）:

```bash
scripts/process/integration_test.sh --specify "TestRelease"
```

本リポジトリの `integration_test.sh` は `--categories` を持たないため、`--specify` で絞り込む。

### 要件と検証の対応

| 要件 | シナリオ | 自動化 |
|---|---|---|
| R1 VERSION 正本 | VS1, VS6 | `TestLibraryVersionMatchesFile` |
| R1.1 ライブラリ API | VS6 | 単体: `timeout.Version` と `VERSION` の一致 |
| R2 commit ldflags | VS1, VS2 | `TestVersionEmbed` / `TestVersionCommitFallback` |
| R3 version 表示 | VS1 | 出力が `timeout.Version` + commit を含む |
| R4.2 tag = module 版 | VS3, VS7 | `TestReleaseTagName` / `TestReleaseRejectsDev` |
| R4.3 成果物名 | VS4 | dry-run 後のファイル一覧 assert |
| R4.4 dry-run | VS4 | `TestReleaseDryRun`（実 GitHub は叩かない） |
| R5 ドキュメント | VS5 | `TestVersionDocs`（cli / library / README） |

### テスト設計セルフレビュー

1. **網羅性**: ライブラリ一致・CLI 表示・commit フォールバック・`-dev` 拒否・tag 名規則・dry-run 成果物を分けて確認する
2. **証拠**: `timeout.Version` と `VERSION` ファイル、および `timeoutx version` の stdout を assert する
3. **迂回排除**: Release の本実行（`gh release create` / 実 tag push）は自動テストでリモートを汚さない。dry-run とガード条件で検証する
4. **依存**: `gh` 認証が無い環境でも VS3/VS4/VS7 が通ること（dry-run / 早期バリデーション）

---

## 完了条件

- [ ] ルートに `VERSION` があり、`timeout.Version` および CLI 表示に反映される
- [ ] `timeoutx version` が version と commit（build ID）を表示する
- [ ] `scripts/process/build.sh` が commit 埋め込み付きで `bin/timeoutx` を作る
- [ ] `scripts/process/release.sh` が `VERSION` 由来の tag `v{VERSION}` で GitHub Release 用成果物を出せる（dry-run で検証可能）
- [ ] その tag が Go module の公開バージョン（`go get ...@v{VERSION}`）と同じ規則である
- [ ] `-dev` VERSION ではリリース／tag できない
- [ ] README / CLI / library ドキュメントが更新されている
- [ ] 既存 CI Release との二重公開が起きないよう整理されている
