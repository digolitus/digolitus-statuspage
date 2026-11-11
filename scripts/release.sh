#!/bin/sh
# Cut a release from main without running CI: computes the next version from
# Conventional Commits (git-cliff, same rules as semantic-release), updates
# CHANGELOG.md, commits it with [skip ci], tags it and publishes a GitHub
# Release with the notes. See docs/operations.md, "Releases".
#
# Usage: scripts/release.sh [--dry-run]
# Needs: git-cliff >= 2.14 (pip install git-cliff==2.14.2) and, to publish
# the GitHub Release, gh authenticated with write access to the repository.
#
# The tag points to a commit whose message has [skip ci], so GitHub Actions
# skips the push workflows for it (CI and, after it, image publishing), so no
# versioned images are published for a release cut this way.
set -eu

DRY_RUN=0
[ "${1:-}" = "--dry-run" ] && DRY_RUN=1

command -v git-cliff >/dev/null 2>&1 || { echo "git-cliff not found (pip install git-cliff==2.14.2)" >&2; exit 1; }

cd "$(git rev-parse --show-toplevel)"

branch=$(git rev-parse --abbrev-ref HEAD)
[ "$branch" = "main" ] || { echo "Run on main (current: $branch)." >&2; exit 1; }
[ -z "$(git status --porcelain)" ] || { echo "Working tree is not clean." >&2; exit 1; }

git fetch --quiet --tags origin main
[ "$(git rev-parse HEAD)" = "$(git rev-parse origin/main)" ] || { echo "Local main differs from origin/main; pull or push first." >&2; exit 1; }

last=$(git describe --tags --abbrev=0 --match 'v[0-9]*.[0-9]*.[0-9]*' 2>/dev/null || echo none)
next=$(git-cliff --bumped-version 2>/dev/null)
if [ "$next" = "$last" ]; then
  echo "No releasable commits since $last."
  exit 0
fi

notes=$(mktemp)
trap 'rm -f "$notes"' EXIT
git-cliff --unreleased --tag "$next" --strip header 2>/dev/null >"$notes"

echo "Release $last -> $next"
echo "------------------------------------------------------------"
cat "$notes"
echo "------------------------------------------------------------"
if [ "$DRY_RUN" = 1 ]; then
  echo "Dry run: nothing was committed, tagged or pushed."
  exit 0
fi

git-cliff --tag "$next" --output CHANGELOG.md 2>/dev/null
git add CHANGELOG.md
git commit --quiet -m "chore(release): $next [skip ci]"
git tag -a "$next" -m "$next"
git push --atomic origin main "refs/tags/$next"

if command -v gh >/dev/null 2>&1; then
  gh release create "$next" --verify-tag --title "$next" --notes-file "$notes"
else
  echo "gh not found: tag $next pushed; create the GitHub Release by hand with the notes above." >&2
fi
