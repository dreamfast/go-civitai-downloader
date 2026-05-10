#!/usr/bin/env bash
set -euo pipefail

# Automated release script for go-civitai-download
# Usage: ./scripts/release.sh [version]
#   version: optional, e.g. v10-05-2026 or v10-05-2026-beta.
#   If omitted, auto-generates a date-based tag (vDD-MM-YYYY).

BINARY_NAME="civitai-downloader"
REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"

cd "$REPO_ROOT"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

log_info() { echo -e "${BLUE}[INFO]${NC} $*"; }
log_warn() { echo -e "${YELLOW}[WARN]${NC} $*"; }
log_success() { echo -e "${GREEN}[OK]${NC} $*"; }
log_error() { echo -e "${RED}[ERROR]${NC} $*" >&2; }

# ---------------------------------------------------------------------------
# Parse args
# ---------------------------------------------------------------------------
VERSION=""
SKIP_CHECKS=false
DRY_RUN=false

while [[ $# -gt 0 ]]; do
    case "$1" in
        --no-check)
            SKIP_CHECKS=true
            shift
            ;;
        --dry-run)
            DRY_RUN=true
            shift
            ;;
        -h|--help)
            cat <<'EOF'
Usage: ./scripts/release.sh [OPTIONS] [VERSION]

Automate building cross-platform binaries, creating a git tag,
and publishing a GitHub release with release notes and assets.

Arguments:
  VERSION       Tag version, e.g. v10-05-2026 or v10-05-2026-beta.
                If omitted, generates vDD-MM-YYYY automatically.

Options:
  --no-check    Skip pre-release quality checks (fmt, vet, lint, tests).
  --dry-run     Show what would be done without executing.
  -h, --help    Show this help message.

Examples:
  ./scripts/release.sh v10-05-2026
  ./scripts/release.sh --no-check v10-05-2026-beta
  ./scripts/release.sh              # auto-generate date tag
EOF
            exit 0
            ;;
        -*)
            log_error "Unknown option: $1"
            exit 1
            ;;
        *)
            if [[ -z "$VERSION" ]]; then
                VERSION="$1"
            else
                log_error "Unexpected argument: $1"
                exit 1
            fi
            shift
            ;;
    esac
done

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------
get_latest_tag() {
    git describe --tags --abbrev=0 2>/dev/null || true
}

generate_date_tag() {
    local tag="$1"
    local today
    today="$(date +%d-%m-%Y)"
    local base="v${today}"

    # If there's no existing tag with today's date, use it directly
    if ! git tag -l "${base}*" | grep -q "^${base}"; then
        echo "$base"
        return
    fi

    # Otherwise, find the highest suffix and increment
    local max_suffix=0
    while IFS= read -r existing; do
        local suffix="${existing#${base}}"
        suffix="${suffix#-}"  # remove leading dash
        if [[ "$suffix" =~ ^[0-9]+$ ]]; then
            if [[ "$suffix" -gt "$max_suffix" ]]; then
                max_suffix="$suffix"
            fi
        fi
    done < <(git tag -l "${base}*")

    echo "${base}-$((max_suffix + 1))"
}

generate_notes() {
    local new_tag="$1"
    local latest_tag
    latest_tag="$(get_latest_tag)"

    local header="## What's Changed\n\n"
    local body=""

    if [[ -n "$latest_tag" ]]; then
        body="$(git log "${latest_tag}..HEAD" --pretty=format:'- %s (%h)' --no-merges)"
    else
        body="$(git log --pretty=format:'- %s (%h)' --no-merges)"
    fi

    if [[ -z "$body" ]]; then
        body="_No new commits since last release._"
    fi

    printf "%s\n\n**Full Changelog**: %s...%s" "$header$body" "$latest_tag" "$new_tag"
}

# ---------------------------------------------------------------------------
# Preflight checks
# ---------------------------------------------------------------------------
log_info "Preflight checks..."

if ! git rev-parse --git-dir >/dev/null 2>&1; then
    log_error "Not a git repository"
    exit 1
fi

# Check for uncommitted changes
if ! git diff-index --quiet HEAD --; then
    log_error "Working tree has uncommitted changes. Commit or stash first."
    exit 1
fi

# Check remote access (gh auth)
if ! gh auth status >/dev/null 2>&1; then
    log_error "Not authenticated with GitHub CLI. Run: gh auth login"
    exit 1
fi

# ---------------------------------------------------------------------------
# Determine version
# ---------------------------------------------------------------------------
if [[ -z "$VERSION" ]]; then
    latest_tag="$(get_latest_tag)"
    VERSION="$(generate_date_tag "$latest_tag")"
    log_info "No version provided. Auto-generated: ${YELLOW}$VERSION${NC}"
else
    # Normalize: ensure leading v
    if [[ ! "$VERSION" =~ ^v ]]; then
        VERSION="v${VERSION}"
    fi
fi

# Validate version format (vDD-MM-YYYY or vDD-MM-YYYY-suffix)
if [[ ! "$VERSION" =~ ^v[0-9]{2}-[0-9]{2}-[0-9]{4}(-[a-zA-Z0-9-]+)?$ ]]; then
    log_error "Invalid version format: $VERSION (expected vDD-MM-YYYY or vDD-MM-YYYY-suffix)"
    exit 1
fi

# Check if tag already exists
if git rev-parse "$VERSION" >/dev/null 2>&1; then
    log_error "Tag $VERSION already exists"
    exit 1
fi

log_success "Version: $VERSION"

# ---------------------------------------------------------------------------
# Quality checks
# ---------------------------------------------------------------------------
if [[ "$SKIP_CHECKS" == false ]]; then
    log_info "Running quality checks (fmt, vet, lint, security, short tests)..."
    if [[ "$DRY_RUN" == true ]]; then
        log_warn "[DRY-RUN] Would run: make check"
    else
        make check
    fi
    log_success "Quality checks passed"
else
    log_warn "Skipping quality checks (--no-check)"
fi

# ---------------------------------------------------------------------------
# Build
# ---------------------------------------------------------------------------
log_info "Building cross-platform release binaries..."
if [[ "$DRY_RUN" == true ]]; then
    log_warn "[DRY-RUN] Would run: make release-cross"
else
    make release-cross
fi

# Verify release artifacts exist
artifacts=()
for f in "${BINARY_NAME}-linux-amd64.tar.gz" \
         "${BINARY_NAME}-linux-arm64.tar.gz" \
         "${BINARY_NAME}-darwin-amd64.tar.gz" \
         "${BINARY_NAME}-darwin-arm64.tar.gz" \
         "${BINARY_NAME}-windows-amd64.zip"; do
    if [[ "$DRY_RUN" == false ]] && [[ ! -f "release/$f" ]]; then
        log_error "Expected artifact not found: release/$f"
        exit 1
    fi
    artifacts+=("release/$f")
done
log_success "All release artifacts ready"

# ---------------------------------------------------------------------------
# Confirmation
# ---------------------------------------------------------------------------
latest_tag="$(get_latest_tag)"
if [[ -n "$latest_tag" ]]; then
    log_info "Previous release: $latest_tag"
fi

echo ""
echo -e "${YELLOW}Release Summary:${NC}"
echo "  Version:      $VERSION"
echo "  Artifacts:    ${#artifacts[@]} files in release/"
echo "  Skip checks:  $SKIP_CHECKS"
echo ""
read -rp "Proceed with release? [y/N]: " confirm
echo ""

if [[ ! "$confirm" =~ ^[Yy]$ ]]; then
    log_warn "Release aborted by user"
    exit 0
fi

# ---------------------------------------------------------------------------
# Create tag
# ---------------------------------------------------------------------------
log_info "Creating git tag: $VERSION"
if [[ "$DRY_RUN" == true ]]; then
    log_warn "[DRY-RUN] Would run: git tag -a $VERSION -m 'Release $VERSION'"
else
    git tag -a "$VERSION" -m "Release $VERSION"
    log_success "Tag created"
fi

# ---------------------------------------------------------------------------
# Generate notes
# ---------------------------------------------------------------------------
notes_file="$(mktemp)"
generate_notes "$VERSION" > "$notes_file"

log_info "Release notes preview:"
echo "---"
head -n 15 "$notes_file"
echo "---"

# ---------------------------------------------------------------------------
# Create GitHub release
# ---------------------------------------------------------------------------
log_info "Creating GitHub release: $VERSION"
if [[ "$DRY_RUN" == true ]]; then
    log_warn "[DRY-RUN] Would run: gh release create $VERSION --title ... --prerelease --notes-file ... ${artifacts[*]}"
else
    gh release create "$VERSION" \
        --title "Release $VERSION" \
        --prerelease \
        --notes-file "$notes_file" \
        "${artifacts[@]}"
    log_success "GitHub release published: $VERSION"
fi

# Cleanup
rm -f "$notes_file"

# ---------------------------------------------------------------------------
# Push tag
# ---------------------------------------------------------------------------
log_info "Pushing tag to remote..."
if [[ "$DRY_RUN" == true ]]; then
    log_warn "[DRY-RUN] Would run: git push origin $VERSION"
else
    git push origin "$VERSION"
    log_success "Tag pushed"
fi

# ---------------------------------------------------------------------------
# Done
# ---------------------------------------------------------------------------
echo ""
log_success "Release $VERSION completed successfully!"
echo ""
log_info "Download URL: https://github.com/$(gh repo view --json url -q '.url' | sed 's|https://github.com/||')/releases/tag/$VERSION"
