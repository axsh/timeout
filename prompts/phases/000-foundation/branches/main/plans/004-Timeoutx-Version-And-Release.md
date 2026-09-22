# 004-Timeoutx-Version-And-Release

> **Source Specification**: `prompts/phases/000-foundation/branches/main/ideas/003-Timeoutx-Version-And-Release.md`

## Goal Description

ルート `VERSION` を正本とし、`timeout.Version`（go:embed）・`timeoutx version`（+ commit ldflags）・`scripts/process/release.sh` の Git tag / GitHub Release（= Go module 版 `v{VERSION}`）を一本化する。既存 CI `release.yml` は二重公開を避けるため削除する（A案）。

## User Review Required

None. 仕様の A案（`release.sh` が正、CI Release 削除）を採用する。

## Requirement Traceability

| Requirement (from Spec) | Implementation Point (Section/File) |
| :--- | :--- |
| R1 VERSION 正本 | Proposed Changes > `VERSION` |
| R1.1 `timeout.Version` go:embed | `version.go` + `version_test.go` |
| R2 commit ldflags | `cmd/timeoutx/main.go` + `build.sh` + `release.sh` |
| R3 `timeoutx version` 表示 | `cmd/timeoutx/main.go` |
| R4.1–R4.4 release.sh | `scripts/process/release.sh` |
| R4 A案（CI 二重公開防止） | `.github/workflows/release.yml` 削除 |
| R5 ドキュメント | `README.md`, `docs/cli.md`, `docs/library.md` |
| R6–R8 任意 | **Deferred**（フル SHA / dirty 接尾辞 / `timeout.Commit` は作らない） |
| VS1–VS7 | Verification Plan + `tests/version_release_test.go` |
| dist/ gitignore | `.gitignore` |

## Proposed Changes

### Library version

#### [NEW] [VERSION](file://VERSION)

*   **Logic**: 1 行のみ。初期内容:

```text
0.3.0-dev
```

末尾改行ありでよい（embed 後に trim）。

#### [NEW] [version.go](file://version.go)

*   **Technical Design**:

```go
package timeout

import (
	_ "embed"
	"strings"
)

//go:embed VERSION
var versionRaw string

// Version is the library/product version from the VERSION file (trimmed).
// It matches the Go module tag when released as v{Version}.
var Version = strings.TrimSpace(versionRaw)
```

*   **Logic**:
    1. `VERSION` を `//go:embed` で取り込む（ルート package と同ディレクトリ）。
    2. `strings.TrimSpace` した結果を公開変数 `Version` に入れる。
    3. `protocol.Version`（整数）は触らない。

#### [NEW] [version_test.go](file://version_test.go)

*   **Logic**（単体・TDD 先書き）:
    1. `TestVersionMatchesVERSIONFile`: `os.ReadFile("VERSION")` を trim し、`Version` と一致することを assert。
    2. `TestVersionNonEmpty`: `Version != ""`。

### CLI

#### [MODIFY] [cmd/timeoutx/main.go](file://cmd/timeoutx/main.go)

*   **Technical Design**:

```go
var commit = "unknown"

// remove: const version = "0.3.0-dev"
```

*   **Logic**:
    1. `const version` を削除する。
    2. `var commit = "unknown"` を追加（`-ldflags "-X main.commit=..."` で上書き）。
    3. `version` / `--version` / `-V` 分岐:

```go
case "version", "--version", "-V":
    fmt.Printf("timeoutx %s (commit %s)\n", timeout.Version, commit)
```

### Build / Release scripts

#### [MODIFY] [scripts/process/build.sh](file://scripts/process/build.sh)

*   **Logic**（`build_root_module` 内の `go build`）:
    1. `COMMIT=$(git -C "$PROJECT_ROOT" rev-parse --short HEAD 2>/dev/null || echo unknown)`
    2. `go build -trimpath -ldflags "-s -w -X main.commit=${COMMIT}" -o "$PROJECT_ROOT/bin/timeoutx" ./cmd/timeoutx`
    3. Windows でも同じ（出力名は既存どおり `bin/timeoutx`；`.exe` は go が付与する場合あり。既存挙動を維持）。

#### [NEW] [scripts/process/release.sh](file://scripts/process/release.sh)

*   **Description**: VERSION から tag `v{VERSION}` を決め、クロスコンパイル＋`SHA256SUMS`＋（本実行時）tag push と `gh release create` を行う。この tag が `go get github.com/axsh/timeout@v{VERSION}` の module 版。
*   **Usage**:

```text
./scripts/process/release.sh [--dry-run] [--skip-push] [--help]
```

*   **Environment**:
    *   `TIMEOUTX_VERSION_FILE`（任意）: 読むバージョンファイル経路。未設定時は `$PROJECT_ROOT/VERSION`。
*   **Logic**:
    1. `set -euo pipefail`。依存: `go`, `git`。本実行時は追加で `gh`。checksum は `sha256sum` があればそれ、無ければ `shasum -a 256`。
    2. `VER=$(tr -d ' \t\r\n' < "$VERSION_FILE")`。空ならエラー。
    3. `TAG="v${VER}"`。
    4. `VER` に `-dev` が含まれる（`case` / `grep`）なら常にエラー終了（`--dry-run` でも拒否）。メッセージに「remove -dev from VERSION」相当を出す。
    5. `--dry-run` のとき: dirty チェック・tag・push・gh をスキップし、ビルド＋checksum のみ。
    6. 本実行（dry-run でない）とき:
       - `git status --porcelain` が空でなければ拒否。
       - ローカルに同名 tag があれば拒否。
       - `gh release view "$TAG"` が成功する（既存）なら拒否（失敗＝未作成なら続行）。
    7. 成果物ディレクトリ: `$PROJECT_ROOT/dist` を作り直す（`rm -rf dist && mkdir -p dist`）。
    8. `COMMIT=$(git rev-parse --short HEAD)`。`LDFLAGS="-s -w -X main.commit=${COMMIT}"`。
    9. 次の 6 組をクロスコンパイル（`CGO_ENABLED=0`）:

| GOOS | GOARCH | 出力名 |
|---|---|---|
| linux | amd64 | `timeoutx_linux_amd64` |
| linux | arm64 | `timeoutx_linux_arm64` |
| darwin | amd64 | `timeoutx_darwin_amd64` |
| darwin | arm64 | `timeoutx_darwin_arm64` |
| windows | amd64 | `timeoutx_windows_amd64.exe` |
| windows | arm64 | `timeoutx_windows_arm64.exe` |

    10. ホストで実行可能なバイナリ（`GOOS`/`GOARCH` が現在の runtime と一致するもの）で `"$bin" version` を実行し、出力に `"timeoutx ${VER} (commit ${COMMIT})"` が含まれることを確認。不一致なら中止。
    11. `dist` 内で checksum ファイル `SHA256SUMS` を生成。
    12. `--dry-run` ならここで成功終了（「would create tag $TAG / Go module @${TAG}」を表示）。
    13. `--skip-push` ならローカル `git tag "$TAG"` のみ（既にあればエラー）、push/gh はしない。
    14. 本実行: `git tag "$TAG"` → `git push origin "$TAG"` → `gh release create "$TAG" dist/* --generate-notes`（または files を列挙）。
    15. `--help` で用法を表示。コメントに「tag equals Go module version for go get」と書く。

#### [DELETE] [.github/workflows/release.yml](file://.github/workflows/release.yml)

*   **Logic**: A案。`release.sh` が唯一の Release 経路。ファイルを削除し、README に手順を書く。

#### [MODIFY] [.gitignore](file://.gitignore)

*   **Logic**: `dist/` を追加（`bin/` の近く）。

### Documentation

#### [MODIFY] [README.md](file://README.md)

*   **更新内容**:
    1. Install 節に、version 正本が `VERSION` であること、`go get ...@vX.Y.Z` のタグが `VERSION` 由来であること。
    2. Build 節に `timeoutx version` の例: `timeoutx 0.3.0-dev (commit abcdef1)`。
    3. Release 節（新規または Install 近く）: `VERSION` を `x.y.z` に直す → `./scripts/process/release.sh` → tag `vX.Y.Z` = module 版 + GitHub Release。`--dry-run` に言及。CI workflow は使わない旨を一文。

#### [MODIFY] [docs/cli.md](file://docs/cli.md)

*   **更新内容**: `## Version` 節を追加:

```markdown
## Version

```bash
timeoutx version
# timeoutx 0.3.0-dev (commit abcdef1)
```

The version string comes from the repository `VERSION` file (same as `timeout.Version`).
The commit is embedded at build time.
```

#### [MODIFY] [docs/library.md](file://docs/library.md)

*   **更新内容**: Import 節の直後に:

```markdown
## Version

```go
fmt.Println(timeout.Version) // e.g. "0.3.0-dev"
```

This matches the repository `VERSION` file. Released module tags are `v` + that value
(for example `go get github.com/axsh/timeout@v0.3.0`).
```

### Integration tests

#### [NEW] [tests/version_release_test.go](file://tests/version_release_test.go)

*   **テストケース**:
    1. `TestVersion_LibraryMatchesFile`: `timeout.Version` とルート `VERSION`（ReadFile+trim）が一致。
    2. `TestVersion_CLIShowsVersionAndCommit`: `timeoutxBin` 相当でビルドするとき `-ldflags -X main.commit=testhash1` を付け、`version` の stdout が `timeoutx {timeout.Version} (commit testhash1)` に一致（または含む）。
    3. `TestVersion_CLICommitFallbackUnknown`: ldflags 無しでビルドし、出力に `(commit unknown)` と `timeout.Version` が含まれる。
    4. `TestRelease_RejectsDev`: `TIMEOUTX_VERSION_FILE` を temp の `0.3.0-dev` にして `bash scripts/process/release.sh --dry-run` を実行。exit != 0。`dist` に成果物が無い（または作られない）。
    5. `TestRelease_DryRunArtifacts`: temp VERSION ファイルに `0.3.0`（または `9.9.9`）を書き、`TIMEOUTX_VERSION_FILE` を指定して `--dry-run`。exit 0。次のファイルが `dist/` に存在: 6 バイナリ + `SHA256SUMS`。ホスト一致バイナリで `version` を実行し VERSION と一致。
    6. `TestRelease_TagNameIsModuleVersion`: スクリプトが `--dry-run` 時に stdout/stderr へ `v0.3.0`（temp の値に応じた tag）と `go get` / `module` 言及を出すことを assert（release.sh が tag 名を表示する）。
    7. `TestVersion_DocsMention`: `README.md`, `docs/cli.md`, `docs/library.md` を読み、それぞれ `VERSION` / `timeoutx version` / `timeout.Version` の言及があること。

*   **注意**: `timeoutxBin` は ldflags 無しビルド。version テスト用に `buildTimeoutx(t, ldflags string)` ヘルパーを同ファイルまたは cli_test から共有可能な形で置く（同パッケージ `tests` なので `version_release_test.go` 内にローカルヘルパーで可）。

## Step-by-Step Implementation Guide

1. **[x] Unit test first (library)**:
    *   Add `version_test.go` expecting `Version` to match `VERSION` file（ファイル未作成でもテストは書く。次でグリーン）。
2. **[x] Add VERSION + version.go**:
    *   Create `VERSION` with `0.3.0-dev` and `version.go` as above. Run unit path via build later.
3. **[x] CLI version + commit**:
    *   Update `cmd/timeoutx/main.go`.
4. **[x] build.sh ldflags**:
    *   Embed `main.commit` on build.
5. **[x] release.sh**:
    *   Implement full script with `--dry-run`, `--skip-push`, `TIMEOUTX_VERSION_FILE`, `-dev` reject, artifact matrix, version self-check, checksum, tag=module note.
6. **[x] Remove CI release.yml + gitignore dist/**:
    *   Delete workflow; add `dist/` to `.gitignore`.
7. **[x] Docs**:
    *   Update `README.md`, `docs/cli.md`, `docs/library.md`.
8. **[x] Integration tests**:
    *   Add `tests/version_release_test.go` for VS1–VS7 coverage.
9. **[x] Verify**:
    *   Run Verification Plan commands. Fix until green.
10. **[x] Commit / far-knowledge / push** per execute workflow.

## Verification Plan

### Automated Verification

1. **Build & Unit Tests**:

```bash
./scripts/process/build.sh
```

*   Expect: unit tests including `TestVersionMatchesVERSIONFile` pass; `bin/timeoutx` built with commit.

2. **Integration Tests (version + release)**:

```bash
./scripts/process/integration_test.sh --specify "TestVersion|TestRelease"
```

*   **Log Verification**:
    *   `TestVersion_CLIShowsVersionAndCommit` pass.
    *   `TestRelease_RejectsDev` non-zero exit for `-dev`.
    *   `TestRelease_DryRunArtifacts` creates 6 binaries + `SHA256SUMS`.
    *   No real `gh release create` / remote tag push.

3. **E2E Tests**:

#### [NEW] [tests/version_release_test.go](file://tests/version_release_test.go)

*   CLI サブコマンドと外部スクリプト（`release.sh`, `git`）を呼ぶため統合テスト必須。上記テストケースが E2E 相当。
*   VSCode GUI E2E は対象外（CLI/ライブラリのみ）。

### テスト設計セルフレビュー (§11.4)

1. **網羅性**: ライブラリ一致・CLI embed/fallback・`-dev` 拒否・dry-run 成果物・tag 名=module・docs を個別ケースで確認。
2. **証拠**: stdout / ファイル存在 / exit code を assert。ドキュメントは必須キーワードの存在確認。
3. **迂回排除**: dry-run で実 GitHub を叩かない。本実行パスは自動テスト対象外。
4. **依存**: `TIMEOUTX_VERSION_FILE` でリポジトリの dirty/`-dev` 状態に依存せずガードと dry-run を検証。

### 総合判定 (§12)

全コマンド成功後、仕様完了条件チェックリストを照合し、未達があれば修正してから push する。

## Documentation

#### [MODIFY] [README.md](file://README.md)
*   **更新内容**: VERSION 正本、`timeoutx version`、`release.sh`、module tag の説明。

#### [MODIFY] [docs/cli.md](file://docs/cli.md)
*   **更新内容**: Version 節。

#### [MODIFY] [docs/library.md](file://docs/library.md)
*   **更新内容**: `timeout.Version` 節。
