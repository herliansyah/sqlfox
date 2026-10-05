#!/usr/bin/env bash
# ==============================================================================
# SqlFox Release Pipeline Automation Script
# Usage: ./scripts/release.sh [patch|minor|major|vX.Y.Z] [--push] [--skip-tests] [--dry-run]
# ==============================================================================

set -euo pipefail

PUSH_REMOTE=false
SKIP_TESTS=false
DRY_RUN=false
TARGET_INPUT=""

for arg in "$@"; do
  case "$arg" in
    --push)
      PUSH_REMOTE=true
      ;;
    --skip-tests)
      SKIP_TESTS=true
      ;;
    --dry-run)
      DRY_RUN=true
      ;;
    -h|--help)
      echo "SqlFox Release Pipeline"
      echo "Usage: $0 [patch|minor|major|vX.Y.Z] [--push] [--skip-tests] [--dry-run]"
      echo ""
      echo "Arguments:"
      echo "  patch|minor|major  Automatically increment the corresponding SemVer segment"
      echo "  vX.Y.Z             Explicit release version (e.g. v0.2.0)"
      echo "  --push             Push commit and annotated tag to origin to trigger GitHub Actions release"
      echo "  --skip-tests       Skip running pre-release unit tests"
      echo "  --dry-run          Simulate the release without making git commits or tags"
      exit 0
      ;;
    *)
      if [ -z "$TARGET_INPUT" ]; then
        TARGET_INPUT="$arg"
      else
        echo "❌ Unknown argument: $arg"
        exit 1
      fi
      ;;
  esac
done

if [ -z "$TARGET_INPUT" ]; then
  echo "❌ Error: Target version or bump type required."
  echo "Usage: $0 [patch|minor|major|vX.Y.Z] [--push] [--skip-tests] [--dry-run]"
  exit 1
fi

echo "🔍 Running pre-flight checks..."

# 1. Ensure working directory is clean
if [ -n "$(git status --porcelain)" ]; then
  if [ "$DRY_RUN" = true ]; then
    echo "⚠️ [DRY-RUN] Notice: Git working directory has uncommitted changes."
  else
    echo "❌ Git working directory has uncommitted changes. Please commit or stash before releasing."
    git status -s
    exit 1
  fi
fi

# 2. Run test suite
if [ "$SKIP_TESTS" = false ]; then
  echo "🧪 Running unit test suite (go test ./...)..."
  if ! go test ./...; then
    echo "❌ Unit tests failed! Aborting release."
    exit 1
  fi
  echo "✅ All unit tests passed."
else
  echo "⚠️ Skipping test execution (--skip-tests)."
fi

# 3. Resolve Current Version
LATEST_TAG=$(git describe --tags --abbrev=0 2>/dev/null || echo "v0.0.0")
CLEAN_LATEST="${LATEST_TAG#v}"
IFS='.' read -r MAJOR MINOR PATCH <<< "$CLEAN_LATEST"

# Default fallback if parsing fails
MAJOR="${MAJOR:-0}"
MINOR="${MINOR:-0}"
PATCH="${PATCH:-0}"

# 4. Compute New Version
case "$TARGET_INPUT" in
  patch)
    NEXT_PATCH=$((PATCH + 1))
    NEW_VERSION="v${MAJOR}.${MINOR}.${NEXT_PATCH}"
    ;;
  minor)
    NEXT_MINOR=$((MINOR + 1))
    NEW_VERSION="v${MAJOR}.${NEXT_MINOR}.0"
    ;;
  major)
    NEXT_MAJOR=$((MAJOR + 1))
    NEW_VERSION="v${NEXT_MAJOR}.0.0"
    ;;
  *)
    if [[ "$TARGET_INPUT" =~ ^v?[0-9]+\.[0-9]+\.[0-9]+ ]]; then
      if [[ "$TARGET_INPUT" =~ ^v ]]; then
        NEW_VERSION="$TARGET_INPUT"
      else
        NEW_VERSION="v$TARGET_INPUT"
      fi
    else
      echo "❌ Invalid version format: $TARGET_INPUT (expected patch, minor, major, or vX.Y.Z)"
      exit 1
    fi
    ;;
esac

echo "🚀 Preparing release: ${LATEST_TAG} ➔ ${NEW_VERSION}"

# Check if tag already exists
if git rev-parse "$NEW_VERSION" >/dev/null 2>&1; then
  echo "❌ Git tag ${NEW_VERSION} already exists! Aborting."
  exit 1
fi

RELEASE_DATE=$(date +%Y-%m-%d)

if [ "$DRY_RUN" = true ]; then
  echo "🔎 [DRY-RUN] Would update CHANGELOG.md with [${NEW_VERSION}] - ${RELEASE_DATE}"
  echo "🔎 [DRY-RUN] Would update cmd/sqlfox/main.go Version = \"${NEW_VERSION}\""
  echo "🔎 [DRY-RUN] Would commit and create tag ${NEW_VERSION}"
  if [ "$PUSH_REMOTE" = true ]; then
    echo "🔎 [DRY-RUN] Would push to git origin"
  fi
  echo "✨ Dry run completed successfully."
  exit 0
fi

# 5. Update CHANGELOG.md
if [ ! -f "CHANGELOG.md" ]; then
  echo "❌ CHANGELOG.md not found at project root."
  exit 1
fi

if ! grep -q "## \[Unreleased\]" CHANGELOG.md; then
  echo "❌ CHANGELOG.md does not contain '## [Unreleased]' section header."
  exit 1
fi

# ponytail: simple portable awk replaces unreleased header with new release section
awk -v ver="$NEW_VERSION" -v dt="$RELEASE_DATE" '
  /## \[Unreleased\]/ {
    print "## [Unreleased]\n"
    print "## [" ver "] - " dt
    next
  }
  { print }
' CHANGELOG.md > CHANGELOG.md.tmp && mv CHANGELOG.md.tmp CHANGELOG.md

# Synchronize to web/dist/CHANGELOG.md
cp CHANGELOG.md web/dist/CHANGELOG.md

echo "📝 Updated CHANGELOG.md and web/dist/CHANGELOG.md for ${NEW_VERSION} (${RELEASE_DATE})"

# 6. Update cmd/sqlfox/main.go Version variable
if [ -f "cmd/sqlfox/main.go" ]; then
  awk -v ver="$NEW_VERSION" '
    /var Version = ".*"/ {
      print "var Version = \"" ver "\""
      next
    }
    { print }
  ' cmd/sqlfox/main.go > cmd/sqlfox/main.go.tmp && mv cmd/sqlfox/main.go.tmp cmd/sqlfox/main.go
  echo "📝 Updated cmd/sqlfox/main.go Version to ${NEW_VERSION}"
fi

# 7. Commit & Tag
git add CHANGELOG.md web/dist/CHANGELOG.md cmd/sqlfox/main.go
git commit -m "chore(release): ${NEW_VERSION}"
git tag -a "${NEW_VERSION}" -m "Release ${NEW_VERSION} (${RELEASE_DATE})"

echo "🏷️ Created annotated tag: ${NEW_VERSION}"

# 8. Push to Remote if requested
if [ "$PUSH_REMOTE" = true ]; then
  CURRENT_BRANCH=$(git branch --show-current)
  echo "⬆️ Pushing ${CURRENT_BRANCH} and tag ${NEW_VERSION} to origin..."
  git push origin "${CURRENT_BRANCH}"
  git push origin "${NEW_VERSION}"
  echo "🎉 Release ${NEW_VERSION} pushed successfully! GitHub Actions build triggered."
else
  echo ""
  echo "💡 Release commit and tag created locally."
  echo "To publish and trigger the multi-OS build on GitHub Actions, run:"
  echo "    git push origin $(git branch --show-current) && git push origin ${NEW_VERSION}"
fi
