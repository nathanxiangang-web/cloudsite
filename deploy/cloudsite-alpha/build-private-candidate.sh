#!/usr/bin/env bash
# Builds a reproducible, product-only CloudSite release candidate package.
# Never touches a production host; only local git, Go and Docker are used.
#
# Usage: bash deploy/cloudsite-alpha/build-private-candidate.sh [OUTPUT_DIR]
# Env: SKIP_DOCKER=1  build binaries/package without the product image.
set -euo pipefail

readonly REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
readonly SCRIPT_DIR="$REPO_ROOT/deploy/cloudsite-alpha"
readonly DEFAULT_OUTPUT_DIR="$HOME/桌面/cloudsite-release-candidate"
readonly GO_PINNED_VERSION="1.27.1"
readonly IMAGE_REPO="cloudsite"

die() { printf 'error: %s\n' "$*" >&2; exit 1; }
step() { printf '==> %s\n' "$*" >&2; }

WORK_DIR=""
BUILD_TREE=""
cleanup() {
  if [[ -n "$BUILD_TREE" ]]; then
    git -C "$REPO_ROOT" worktree remove --force "$BUILD_TREE" >/dev/null 2>&1 || true
    rm -rf "$BUILD_TREE"
    git -C "$REPO_ROOT" worktree prune >/dev/null 2>&1 || true
  fi
  if [[ -n "$WORK_DIR" && -d "$WORK_DIR" ]]; then rm -rf "$WORK_DIR"; fi
}
trap cleanup EXIT

[[ -n "${1:-}" ]] && OUTPUT_DIR="$1" || OUTPUT_DIR="$DEFAULT_OUTPUT_DIR"
mkdir -p "$OUTPUT_DIR"
OUTPUT_DIR="$(cd "$OUTPUT_DIR" && pwd)"

# Candidate packages must never land inside the Git work tree (design §2.1).
case "$OUTPUT_DIR/" in
  "$REPO_ROOT"/*) die "OUTPUT_DIR must be outside the Git repository: $OUTPUT_DIR" ;;
esac

step "verifying committed source"
cd "$REPO_ROOT"
git rev-parse --git-dir >/dev/null 2>&1 || die "not a Git repository: $REPO_ROOT"
COMMIT="$(git rev-parse HEAD)" || die "cannot resolve HEAD"
SHORT_SHA="$(git rev-parse --short HEAD)"
BRANCH="$(git rev-parse --abbrev-ref HEAD)"
REMOTE_URL="$(git remote get-url origin 2>/dev/null || printf 'unknown')"
if [[ -n "$(git status --porcelain --untracked-files=no)" ]]; then
  die "working tree has tracked changes; commit or stash them first"
fi
[[ -f "$REPO_ROOT/VERSION" ]] || die "missing VERSION"
VERSION="$(tr -d '[:space:]' < "$REPO_ROOT/VERSION")"
[[ -n "$VERSION" ]] || die "VERSION is empty"

step "verifying Go ${GO_PINNED_VERSION}"
if [[ -n "${GO_BIN:-}" ]]; then
  [[ -x "$GO_BIN" ]] || die "GO_BIN is not executable: $GO_BIN"
else
  GO_BIN="$(command -v go || true)"
  if [[ -z "$GO_BIN" && -x /home/nathan/.local/share/go1.27.1/bin/go ]]; then
    GO_BIN=/home/nathan/.local/share/go1.27.1/bin/go
  fi
fi
[[ -n "$GO_BIN" ]] || die "Go toolchain not found"
GO_VERSION_STRING="$("$GO_BIN" version)"
[[ "$GO_VERSION_STRING" == *"go${GO_PINNED_VERSION}"* ]] \
  || die "Go ${GO_PINNED_VERSION} required, found: $GO_VERSION_STRING"

IMAGE_TAG="${IMAGE_REPO}:private-candidate-${SHORT_SHA}"
if [[ "${SKIP_DOCKER:-0}" == "1" ]]; then
  step "SKIP_DOCKER=1: product image build skipped"
else
  step "verifying Docker (linux/amd64)"
  command -v docker >/dev/null 2>&1 || die "Docker is not installed"
  docker info >/dev/null 2>&1 || die "Docker daemon is unavailable"
  DOCKER_ARCH="$(docker info --format '{{.Architecture}}' 2>/dev/null || true)"
  case "$DOCKER_ARCH" in
    x86_64|amd64) ;;
    *) die "Docker host must be linux/amd64, found: ${DOCKER_ARCH:-unknown}" ;;
  esac
fi

readonly PACKAGE="cloudsite_${VERSION}_private_candidate_${SHORT_SHA}_linux_amd64"
readonly ARCHIVE="${OUTPUT_DIR}/${PACKAGE}.tar.gz"
readonly CHECKSUMS="${OUTPUT_DIR}/SHA256SUMS"

step "building ${PACKAGE}"
WORK_DIR="$(mktemp -d "${OUTPUT_DIR}/.build-${PACKAGE}.XXXXXX")"
PACKAGE_DIR="${WORK_DIR}/${PACKAGE}"
mkdir -p "$PACKAGE_DIR"

# Build from a detached worktree of the exact commit so untracked local files
# (e.g. .codeartsdoer/) cannot make Go stamp vcs.modified=true.
BUILD_TREE="${WORK_DIR}/src-${SHORT_SHA}-$$"
git worktree add --detach "$BUILD_TREE" "$COMMIT" >&2

(
  cd "$BUILD_TREE"
  CGO_ENABLED=0 GOOS=linux GOARCH=amd64 "$GO_BIN" build -trimpath -ldflags '-s -w' \
    -o "${PACKAGE_DIR}/cloudsite" ./cmd/panta
  CGO_ENABLED=0 GOOS=linux GOARCH=amd64 "$GO_BIN" build -trimpath -ldflags '-s -w' \
    -o "${PACKAGE_DIR}/panta-db" ./cmd/panta-db
)

[[ -s "${PACKAGE_DIR}/cloudsite" ]] || die "missing cloudsite binary"
[[ -s "${PACKAGE_DIR}/panta-db" ]] || die "missing panta-db binary"

step "asserting clean VCS stamp"
(
  cd "$PACKAGE_DIR"
  for binary in cloudsite panta-db; do
    vcs_info="$("$GO_BIN" version -m "$binary")"
    if [[ "$vcs_info" == *"vcs.modified=true"* || "$vcs_info" == *"+dirty"* ]]; then
      die "${binary} embeds dirty VCS state (vcs.modified=true); refusing to package"
    fi
  done
)

step "writing source manifest and build metadata"
GENERATED_AT="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
cp "$REPO_ROOT/VERSION" "${PACKAGE_DIR}/VERSION"
cp "${SCRIPT_DIR}/product-upgrade.override.yml" "${PACKAGE_DIR}/product-upgrade.override.yml"
cp "$REPO_ROOT/docs/operations/private-cloud-candidate/README.md" "${PACKAGE_DIR}/UPGRADE-RUNBOOK.md"

cat > "${PACKAGE_DIR}/SOURCE-MANIFEST.json" <<EOF
{
  "schema": "cloudsite-private-candidate-source/v1",
  "package": "${PACKAGE}",
  "repository": "${REMOTE_URL}",
  "branch": "${BRANCH}",
  "commit": "${COMMIT}",
  "short_commit": "${SHORT_SHA}",
  "version": "${VERSION}",
  "generated_at": "${GENERATED_AT}",
  "candidate_status": "private-candidate-not-production-admitted",
  "components": ["cloudsite", "panta-db"],
  "indexcore_included": false,
  "excluded": [
    "cloudsite-index-reader",
    "cmd/indexcore",
    "catalog-entrypoint.py",
    "IndexCore catalog and reader sources/binaries/images"
  ]
}
EOF

{
  printf 'package: %s\n' "$PACKAGE"
  printf 'commit: %s\n' "$COMMIT"
  printf 'branch: %s\n' "$BRANCH"
  printf 'version: %s\n' "$VERSION"
  printf 'built_at: %s\n' "$GENERATED_AT"
  printf 'go: %s\n' "$GO_VERSION_STRING"
  printf 'image: %s\n' "$IMAGE_TAG"
} > "${PACKAGE_DIR}/BUILD-INFO.txt"

(
  cd "$PACKAGE_DIR"
  printf '\n--- cloudsite ---\n' >> BUILD-INFO.txt
  "$GO_BIN" version -m cloudsite >> BUILD-INFO.txt
  printf '\n--- panta-db ---\n' >> BUILD-INFO.txt
  "$GO_BIN" version -m panta-db >> BUILD-INFO.txt
)

(
  cd "$PACKAGE_DIR"
  sha256sum cloudsite panta-db > BINARY-SHA256SUMS
)

if [[ "${SKIP_DOCKER:-0}" == "1" ]]; then
  printf 'SKIP_DOCKER=1: no product image archive in package\n' >> "${PACKAGE_DIR}/BUILD-INFO.txt"
else
  step "building product image ${IMAGE_TAG}"
  IMAGE_CONTEXT="${WORK_DIR}/image-context"
  mkdir -p "$IMAGE_CONTEXT"
  cp "${PACKAGE_DIR}/cloudsite" "${IMAGE_CONTEXT}/cloudsite"
  cp "${SCRIPT_DIR}/Dockerfile.product" "${IMAGE_CONTEXT}/Dockerfile.product"
  docker build --platform linux/amd64 -t "$IMAGE_TAG" \
    --label "org.opencontainers.image.revision=${COMMIT}" \
    --label "org.opencontainers.image.source=${REMOTE_URL}" \
    --label "org.opencontainers.image.version=${VERSION}" \
    -f "${IMAGE_CONTEXT}/Dockerfile.product" "$IMAGE_CONTEXT" >&2
  docker save "$IMAGE_TAG" | gzip -9 > "${PACKAGE_DIR}/cloudsite-private-candidate-image.tar.gz"
  [[ -s "${PACKAGE_DIR}/cloudsite-private-candidate-image.tar.gz" ]] \
    || die "product image archive was not produced"
fi

step "publishing ${PACKAGE}"
rm -rf "${OUTPUT_DIR:?}/${PACKAGE}"
rm -f "$ARCHIVE"
mv "$PACKAGE_DIR" "${OUTPUT_DIR}/${PACKAGE}"
tar -C "$OUTPUT_DIR" -czf "$ARCHIVE" "$PACKAGE"
[[ -s "$ARCHIVE" ]] || die "package archive was not produced"

(
  cd "$OUTPUT_DIR"
  if [[ -f SHA256SUMS ]]; then
    grep -v "  ${PACKAGE}.tar.gz\$" SHA256SUMS > SHA256SUMS.tmp || true
    sha256sum "${PACKAGE}.tar.gz" >> SHA256SUMS.tmp
    mv SHA256SUMS.tmp SHA256SUMS
  else
    sha256sum "${PACKAGE}.tar.gz" > SHA256SUMS
  fi
)

cleanup
WORK_DIR=""
BUILD_TREE=""
printf 'candidate package: %s\n' "$ARCHIVE" >&2
printf 'checksums: %s\n' "$CHECKSUMS" >&2