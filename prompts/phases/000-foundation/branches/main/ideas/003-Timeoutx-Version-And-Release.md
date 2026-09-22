# 003 Timeoutx Version Embedding and GitHub Release Script

> **Parent**: `prompts/phases/000-foundation/branches/main/ideas/000-Timeoutx-Execution-Policy-Engine.md`
>
> 編集可能なバージョン番号を正本化し、`timeoutx` バイナリへの埋め込み表示と、
> その番号を使う GitHub Release 用スクリプトを追加する。

## 背景 (Background)

現状のバージョン表示は `cmd/timeoutx/main.go` 内の定数 `const version = "0.3.0-dev"` にハードコードされている。
ビルド ID（commit SHA）は埋め込まれておらず、`timeoutx version` はバージョン文字列だけを出す。

GitHub Releases 向けのクロスコンパイルは `.github/workflows/release.yml` が tag `v*` で動作するが、
次の点が不足している。

1. リポジトリ内に「現在のバージョン」をテキストで編集できる単一の正本がない
2. ビルド成果物にバージョンと commit ID が埋め込まれない（`ldflags` 未使用）
3. ローカルから同じ規則で Release を切るスクリプトがない（CI のみ）

利用者が「どの版のバイナリか」を確認でき、メンテナが同じ番号で tag / Release を切れる状態にする。

---

## 要件 (Requirements)

### 必須要件

#### R1. バージョン正本ファイル

リポジトリルートに編集可能なテキストファイル `VERSION` を置く。

- 内容は **1 行のみ**。前後の空白は無視してよいが、改行以外の余分な文字は含めない
- 形式は SemVer 風の文字列とする（例: `0.3.0`、開発中は `0.4.0-dev` も可）
- 先頭の `v` は付けない（正本は `0.3.0`、Git tag は `v0.3.0`）
- このファイルがプロジェクトの「現在のバージョン」の唯一の正本である
- `cmd/timeoutx/main.go` 内のハードコード定数は、ビルド時未指定時のフォールバックに限り残してよいが、正本は `VERSION` とする

初期値は現状の意図に合わせて `0.3.0-dev` とする（実装時に現状定数と揃える）。

#### R2. バイナリへの埋め込み

`timeoutx` ビルド時に次の 2 値を埋め込む。

| 変数 | 意味 | 取得元 |
|---|---|---|
| version | リリース／開発バージョン | `VERSION` の内容 |
| commit（build ID） | ビルド時点の Git commit | `git rev-parse --short HEAD`（7 文字以上の短縮 SHA） |

- 埋め込み方法は Go の `-ldflags "-X ..."` とする
- `scripts/process/build.sh` で `bin/timeoutx` を作るときも、上記を埋め込む
- `.github/workflows/release.yml` のビルドでも同様に埋め込む
- ldflags 未指定の素の `go build` / `go install` では:
  - version: フォールバック文字列（例: `dev` または現行の `0.3.0-dev`）
  - commit: `unknown`
  とし、パニックしない

dirty 作業ツリーの扱いは任意（任意要件 R7）とする。必須では commit に dirty 接尾辞を付けなくてよい。

#### R3. `timeoutx version` の表示

既存サブコマンド `version` / `--version` / `-V` の出力を、バージョンと build ID の両方を含む形に更新する。

推奨フォーマット（1 行）:

```text
timeoutx 0.3.0-dev (commit abcdef1)
```

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

##### R4.2 バージョンと tag

1. `VERSION` を読む（trim 後が空ならエラー終了）
2. tag 名は `v` + `VERSION`（例: `VERSION=0.3.0` → tag `v0.3.0`）
3. `VERSION` が `-dev` を含む場合は **リリースを拒否** して非 0 で終了する（誤公開防止）
4. 作業ツリーが dirty の場合は拒否する（未コミット変更での公開を防ぐ）
5. 既に同名 tag / Release が存在する場合は拒否する（上書きしない）

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

- `README.md` の Install / Build 周辺に、`VERSION` が正本であることと、`timeoutx version` で version + commit が見られることを短く追記する
- Release 手順として `scripts/process/release.sh` の使い方を 数行で記載する
- `docs/cli.md` に `version` サブコマンドの出力例を 1 つ載せる

### 任意要件

#### R6. 長い commit（フル SHA）

環境変数やフラグでフル SHA を埋め込むオプションは任意。必須は短縮 SHA でよい。

#### R7. dirty 接尾辞

`git` が dirty なときのローカル `build.sh` で commit を `abcdef1-dirty` にするのは任意。
`release.sh` は dirty を拒否すれば足りる。

#### R8. `go generate` / embed

`VERSION` を `go:embed` で取り込む方式は任意。必須は ldflags による注入でよい。

---

## 実現方針 (Implementation Approach)

### コンポーネント概要

```text
VERSION                          # 正本（人が編集）
        │
        ├─► scripts/process/build.sh     -ldflags → bin/timeoutx
        ├─► scripts/process/release.sh   -ldflags → dist/timeoutx_* + gh release
        └─► .github/workflows/release.yml（A案では無効化 or スクリプトと役割分担を明記）

cmd/timeoutx/main.go
  var version = "..."   # ldflags で上書き
  var commit  = "unknown"
  version コマンドが両方を表示
```

### 設計上の決定

1. **正本はルート `VERSION` 1 ファイル**。Go 定数や CI 環境変数を正本にしない
2. **Git tag は常に `v` + VERSION**。`VERSION` 自体には `v` を書かない
3. **開発用サフィックス `-dev` の Release は禁止**。正式公開前に `VERSION` を `x.y.z` に直してから `release.sh` を走らせる
4. **表示は CLI のみ**。Result JSON や protocol に version フィールドを足さない（本仕様の範囲外）
5. **Release の正経路は `scripts/process/release.sh`（A案）**。CI との二重公開を避ける

### 主要な変更ファイル（予定）

| パス | 変更 |
|---|---|
| `VERSION` | 新規。初期値 `0.3.0-dev` |
| `cmd/timeoutx/main.go` | `version`/`commit` 変数化、表示更新 |
| `scripts/process/build.sh` | ldflags 埋め込み |
| `scripts/process/release.sh` | 新規 |
| `.github/workflows/release.yml` | A案に合わせて無効化または埋め込みのみに整理 |
| `README.md` / `docs/cli.md` | 表示例と Release 手順 |
| `.gitignore` | `dist/` を追加（未追加なら） |
| `tests/cli_test.go` 等 | version 出力の検証 |

### ldflags の想定

```bash
VERSION=$(tr -d ' \t\r\n' < VERSION)
COMMIT=$(git rev-parse --short HEAD)
LDFLAGS="-s -w -X main.version=${VERSION} -X main.commit=${COMMIT}"
go build -trimpath -ldflags "$LDFLAGS" -o bin/timeoutx ./cmd/timeoutx
```

（パッケージパスが `main` 以外に分かれた場合は `-X` のパスを合わせる。）

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
| R1 VERSION 正本 | VS1 | ビルドが `VERSION` を読むこと、`TestVersionEmbed` |
| R2 ldflags 埋め込み | VS1, VS2 | `TestVersionEmbed` / `TestVersionFallback` |
| R3 version 表示 | VS1 | 出力正規表現で version + commit を assert |
| R4.2 `-dev` 拒否 | VS3 | `TestReleaseRejectsDev` |
| R4.3 成果物名 | VS4 | dry-run 後のファイル一覧 assert |
| R4.4 dry-run | VS4 | `TestReleaseDryRun`（実 GitHub は叩かない） |
| R5 ドキュメント | VS5 | `TestVersionDocs` またはファイル存在・文字列確認 |

### テスト設計セルフレビュー

1. **網羅性**: 埋め込み成功・未指定フォールバック・`-dev` 拒否・dry-run 成果物を分けて確認する
2. **証拠**: `timeoutx version` の stdout を正規表現で assert する。ドキュメント文字列一致だけにしない
3. **迂回排除**: Release の本実行（`gh release create`）は CI 上で実リモートを汚さない。dry-run とガード条件で検証する
4. **依存**: `gh` 認証が無い環境でも VS3/VS4 が通ること（dry-run / 早期バリデーション）

---

## 完了条件

- [ ] ルートに `VERSION` があり、編集がビルド成果物の表示に反映される
- [ ] `timeoutx version` が version と commit（build ID）を表示する
- [ ] `scripts/process/build.sh` が埋め込み付きで `bin/timeoutx` を作る
- [ ] `scripts/process/release.sh` が `VERSION` 由来の tag 名で GitHub Release 用成果物を出せる（dry-run で検証可能）
- [ ] `-dev` VERSION ではリリースできない
- [ ] README / CLI ドキュメントが更新されている
- [ ] 既存 CI Release との二重公開が起きないよう整理されている
