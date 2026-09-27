#!/usr/bin/env bash
# Writes a shields.io endpoint file (coverage.json) to the orphan `badges`
# branch, which the README badge reads. Keeping it in the repository avoids
# an external coverage service and its token.
#
#   .github/scripts/publish-coverage-badge.sh 82.1%
set -euo pipefail

total="${1:-}"
if [[ ! "$total" =~ ^[0-9]+(\.[0-9]+)?%$ ]]; then
	echo "coverage total '$total' is not a percentage like 82.1%" >&2
	exit 1
fi

percent="${total%\%}"
color=red
if awk "BEGIN {exit !($percent >= 80)}"; then
	color=brightgreen
elif awk "BEGIN {exit !($percent >= 60)}"; then
	color=yellow
fi

worktree="$(mktemp -d)"
trap 'git worktree remove --force "$worktree"' EXIT
git config user.name "github-actions[bot]"
git config user.email "41898282+github-actions[bot]@users.noreply.github.com"
# Detached, so no local branch name can collide; pushed as HEAD:badges.
if git fetch -q --depth 1 origin badges 2>/dev/null; then
	git worktree add -q --detach "$worktree" FETCH_HEAD
else
	git worktree add -q --detach "$worktree"
	git -C "$worktree" switch -q --orphan "badges-$$"
fi

printf '{"schemaVersion":1,"label":"coverage","message":"%s","color":"%s"}\n' "$total" "$color" >"$worktree/coverage.json"
git -C "$worktree" add coverage.json
if git -C "$worktree" diff --cached --quiet; then
	echo "coverage unchanged at $total"
	exit 0
fi
git -C "$worktree" commit -q -m "coverage: $total"
git -C "$worktree" push -q origin HEAD:badges
