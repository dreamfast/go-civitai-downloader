#!/usr/bin/env bash
set -euo pipefail

# Automated release script for go-civitai-download
# Usage: ./scripts/release.sh [version]
#   version: optional, e.g. v10-05-2026 or v10-05-2026-beta.
#   If omitted, auto-generates a date-based tag (vDD-MM-YYYY).
#
# Fully automatic: tag → push tag → build → create GitHub release → upload binaries

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
YES=false

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
        -y|--yes)
            YES=true
            shift
            ;;
        -h|--help)
            cat <<'EOF'
Usage: ./scripts/release.sh [OPTIONS] [VERSION]

Automate building cross-platform binaries, creating a git tag,
pushing to remote, and publishing a GitHub release with assets.

Arguments:
  VERSION       Tag version, e.g. v10-05-2026 or v10-05-2026-beta.
                If omitted, generates vDD-MM-YYYY automatically.

Options:
  --no-check    Skip pre-release quality checks (fmt, vet, lint, tests).
  --dry-run     Show what would be done without executing.
  -y, --yes     Skip confirmation prompt (fully non-interactive).
  -h, --help    Show this help message.

Examples:
  ./scripts/release.sh v10-05-2026
  ./scripts/release.sh --no-check v10-05-2026-beta
  ./scripts/release.sh --yes              # auto-tag, no prompt
  ./scripts/release.sh                    # auto-generate date tag, prompt
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

    local body=""

    if [[ -n "$latest_tag" ]]; then
        body="$(git log "${latest_tag}..HEAD" --pretty=format:'- %s (%h)' --no-merges)"
    else
        body="$(git log --pretty=format:'- %s (%h)' --no-merges)"
    fi

    if [[ -z "$body" ]]; then
        body="_No new commits since last release._"
    fi

    printf "## What's Changed\n\n%s\n\n**Full Changelog**: %s...%s" "$body" "${latest_tag:-initial}" "$new_tag"
}

cleanup() {
    if [[ -n "${notes_file:-}" ]] && [[ -f "$notes_file" ]]; then
        rm -f "$notes_file"
    fi
}
trap cleanup EXIT

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

# Check that current branch is pushed to remote (tags need a reachable commit)
local_branch="$(git rev-parse --abbrev-ref HEAD)"
remote_branch="origin/${local_branch}"
if git rev-parse --verify "$remote_branch" >/dev/null 2>&1; then
    ahead="$(git rev-list --count "${remote_branch}..HEAD" 2>/dev/null || echo "0")"
    if [[ "$ahead" -gt 0 ]]; then
        log_warn "Current branch '$local_branch' is $ahead commit(s) ahead of remote."
        log_info "Pushing commits to remote first..."
        if [[ "$DRY_RUN" == true ]]; then
            log_warn "[DRY-RUN] Would run: git push origin $local_branch"
        else
            git push origin "$local_branch"
            log_success "Commits pushed"
        fi
    fi
else
    log_warn "Branch '$local_branch' not found on remote. Pushing..."
    if [[ "$DRY_RUN" == true ]]; then
        log_warn "[DRY-RUN] Would run: git push -u origin $local_branch"
    else
        git push -u origin "$local_branch"
        log_success "Branch pushed to remote"
    fi
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

# Check if tag already exists locally
if git rev-parse "$VERSION" >/dev/null 2>&1; then
    log_error "Tag $VERSION already exists locally. Delete it first: git tag -d $VERSION"
    exit 1
fi

# Check if tag already exists on remote
if git ls-remote --tags origin "refs/tags/${VERSION}" 2>/dev/null | grep -q "$VERSION"; then
    log_error "Tag $VERSION already exists on remote. Choose a different version."
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
# Confirmation
# ---------------------------------------------------------------------------
latest_tag="$(get_latest_tag)"
if [[ -n "$latest_tag" ]]; then
    log_info "Previous release: $latest_tag"
fi

echo ""
echo -e "${YELLOW}Release Summary:${NC}"
echo "  Version:      $VERSION"
echo "  Skip checks:  $SKIP_CHECKS"
echo "  Previous:     ${latest_tag:-none}"
echo ""

if [[ "$YES" == false ]]; then
    read -rp "Proceed with release? [y/N]: " confirm
    echo ""
    if [[ ! "$confirm" =~ ^[Yy]$ ]]; then
        log_warn "Release aborted by user"
        exit 0
    fi
else
    log_info "Skipping confirmation (--yes)"
fi

# ---------------------------------------------------------------------------
# Build cross-platform binaries
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
log_success "All ${#artifacts[@]} release artifacts ready"

# ---------------------------------------------------------------------------
# Create tag + push to remote (must be pushed BEFORE gh release create)
# ---------------------------------------------------------------------------
log_info "Creating git tag: $VERSION"
if [[ "$DRY_RUN" == true ]]; then
    log_warn "[DRY-RUN] Would run: git tag -a $VERSION -m 'Release $VERSION'"
    log_warn "[DRY-RUN] Would run: git push origin $VERSION"
else
    git tag -a "$VERSION" -m "Release $VERSION"
    log_success "Tag created locally"
    git push origin "$VERSION"
    log_success "Tag pushed to remote"
fi

# ---------------------------------------------------------------------------
# Generate release notes
# ---------------------------------------------------------------------------
notes_file="$(mktemp)"
generate_notes "$VERSION" > "$notes_file"

log_info "Release notes preview:"
echo "---"
head -n 15 "$notes_file"
echo "---"

# ---------------------------------------------------------------------------
# Create GitHub release + upload artifacts
# ---------------------------------------------------------------------------
log_info "Creating GitHub release: $VERSION"
log_info "Uploading ${#artifacts[@]} artifacts..."

if [[ "$DRY_RUN" == true ]]; then
    log_warn "[DRY-RUN] Would run: gh release create $VERSION --title 'Release $VERSION' --prerelease --notes-file ... ${artifacts[*]}"
else
    gh release create "$VERSION" \
        --title "Release $VERSION" \
        --prerelease \
        --notes-file "$notes_file" \
        "${artifacts[@]}"
    log_success "GitHub release published: $VERSION"
fi

# ---------------------------------------------------------------------------
# Done
# ---------------------------------------------------------------------------
repo_slug="$(gh repo view --json nameWithOwner -q '.nameWithOwner')"
echo ""
log_success "Release $VERSION completed successfully!"
echo ""
log_info "Download URL: https://github.com/${repo_slug}/releases/tag/$VERSION"
