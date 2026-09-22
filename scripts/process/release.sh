#!/bin/bash
set -euo pipefail

# ============================================================
# release.sh — Build multi-platform timeoutx binaries and
# publish a GitHub Release whose tag equals the Go module version.
#
# The repository VERSION file is the single source of truth.
# Tag name is "v" + VERSION (example: 0.3.0 -> v0.3.0).
# That tag is what `go get github.com/axsh/timeout@v0.3.0` resolves.
#
# Usage:
#   ./scripts/process/release.sh [OPTIONS]
#
# Options:
#   --dry-run      Build artifacts and checksums only (no tag/push/gh)
#   --skip-push    Create local tag only (no push / no gh release)
#   --help         Show this help
#
# Environment:
#   TIMEOUTX_VERSION_FILE   Optional path to VERSION file (default: repo VERSION)
#
# Exit Codes:
#   0 = Success
#   1 = Validation or build failure
# ============================================================

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m'
BOLD='\033[1m'

info()    { echo -e "${BLUE}[INFO]${NC} $*"; }
success() { echo -e "${GREEN}[PASS]${NC} $*"; }
warn()    { echo -e "${YELLOW}[WARN]${NC} $*"; }
fail()    { echo -e "${RED}[FAIL]${NC} $*"; }
step()    { echo -e "${CYAN}${BOLD}===> $*${NC}"; }

DRY_RUN=false
SKIP_PUSH=false

show_help() {
    cat << 'EOF'
Usage: ./scripts/process/release.sh [OPTIONS]

Build timeoutx for Linux/macOS/Windows (amd64+arm64), write SHA256SUMS,
and create a GitHub Release. The Git tag is v{VERSION} and is also the
Go module version for: go get github.com/axsh/timeout@v{VERSION}

Options:
  --dry-run      Build + checksum only (no tag, push, or gh release)
  --skip-push    Create local git tag only (no push / no gh release)
  --help         Show this help

Environment:
  TIMEOUTX_VERSION_FILE   Override path to the VERSION file

Examples:
  ./scripts/process/release.sh --dry-run
  ./scripts/process/release.sh
EOF
}

while [[ $# -gt 0 ]]; do
    case "$1" in
        --dry-run)
            DRY_RUN=true
            shift
            ;;
        --skip-push)
            SKIP_PUSH=true
            shift
            ;;
        --help|-h)
            show_help
            exit 0
            ;;
        *)
            fail "Unknown option: $1"
            show_help
            exit 1
            ;;
    esac
done

require_cmd() {
    if ! command -v "$1" >/dev/null 2>&1; then
        fail "required command not found: $1"
        exit 1
    fi
}

write_checksums() {
    local dir="$1"
    (
        cd "$dir"
        if command -v sha256sum >/dev/null 2>&1; then
            sha256sum timeoutx_* > SHA256SUMS
        elif command -v shasum >/dev/null 2>&1; then
            shasum -a 256 timeoutx_* > SHA256SUMS
        else
            fail "neither sha256sum nor shasum found"
            exit 1
        fi
    )
}

VERSION_FILE="${TIMEOUTX_VERSION_FILE:-$PROJECT_ROOT/VERSION}"
if [[ ! -f "$VERSION_FILE" ]]; then
    fail "VERSION file not found: $VERSION_FILE"
    exit 1
fi

VER="$(tr -d ' \t\r\n' < "$VERSION_FILE")"
if [[ -z "$VER" ]]; then
    fail "VERSION is empty"
    exit 1
fi

TAG="v${VER}"
info "VERSION=${VER} tag=${TAG} (Go module @${TAG})"

case "$VER" in
    *-dev*|*-DEV*)
        fail "refusing to release VERSION with -dev suffix: ${VER}"
        fail "set VERSION to a release number (e.g. 0.3.0) first"
        exit 1
        ;;
esac

require_cmd go
require_cmd git

if [[ "$DRY_RUN" != "true" ]]; then
    if [[ -n "$(git -C "$PROJECT_ROOT" status --porcelain)" ]]; then
        fail "working tree is dirty; commit or stash before release"
        exit 1
    fi
    if git -C "$PROJECT_ROOT" rev-parse "$TAG" >/dev/null 2>&1; then
        fail "local tag already exists: $TAG"
        exit 1
    fi
    if [[ "$SKIP_PUSH" != "true" ]]; then
        require_cmd gh
        if gh release view "$TAG" >/dev/null 2>&1; then
            fail "GitHub release already exists: $TAG"
            exit 1
        fi
    fi
fi

DIST="$PROJECT_ROOT/dist"
step "Building release artifacts into dist/"
rm -rf "$DIST"
mkdir -p "$DIST"

COMMIT="$(git -C "$PROJECT_ROOT" rev-parse --short HEAD)"
LDFLAGS="-s -w -X main.commit=${COMMIT}"
HOST_GOOS="$(go env GOOS)"
HOST_GOARCH="$(go env GOARCH)"
HOST_BIN=""

TARGETS=(
    "linux amd64"
    "linux arm64"
    "darwin amd64"
    "darwin arm64"
    "windows amd64"
    "windows arm64"
)

for pair in "${TARGETS[@]}"; do
    # shellcheck disable=SC2086
    set -- $pair
    goos="$1"
    goarch="$2"
    ext=""
    if [[ "$goos" == "windows" ]]; then
        ext=".exe"
    fi
    out="timeoutx_${goos}_${goarch}${ext}"
    info "Building ${out}"
    (
        cd "$PROJECT_ROOT"
        CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" \
            go build -trimpath -ldflags "$LDFLAGS" -o "$DIST/$out" ./cmd/timeoutx
    )
    if [[ "$goos" == "$HOST_GOOS" && "$goarch" == "$HOST_GOARCH" ]]; then
        HOST_BIN="$DIST/$out"
    fi
done

if [[ -n "$HOST_BIN" ]]; then
    step "Verifying embedded version on host binary"
    got="$("$HOST_BIN" version)"
    expect="timeoutx ${VER} (commit ${COMMIT})"
    if [[ "$got" != "$expect" ]]; then
        fail "version mismatch: got='$got' want='$expect'"
        exit 1
    fi
    success "$got"
else
    warn "No host-matching binary to execute; skipping live version check"
fi

step "Writing SHA256SUMS"
write_checksums "$DIST"
success "Artifacts ready in dist/"
ls -la "$DIST"

if [[ "$DRY_RUN" == "true" ]]; then
    success "dry-run complete; would create tag ${TAG} (= Go module version for go get ...@${TAG})"
    exit 0
fi

step "Creating local tag ${TAG}"
git -C "$PROJECT_ROOT" tag "$TAG"

if [[ "$SKIP_PUSH" == "true" ]]; then
    success "local tag ${TAG} created; skip-push set (no remote / no gh release)"
    exit 0
fi

step "Pushing tag ${TAG}"
git -C "$PROJECT_ROOT" push origin "$TAG"

step "Creating GitHub Release ${TAG}"
gh release create "$TAG" "$DIST"/* --generate-notes

success "Released ${TAG} (module go get github.com/axsh/timeout@${TAG})"
