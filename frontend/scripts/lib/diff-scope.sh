#!/usr/bin/env bash
# Which lines does THIS branch add — the plumbing under the diff-scoped frontend
# gates, spelled once.
#
# Two gates need the same three answers (where is the checkout, what is the
# merge-base with origin/main, what does the diff for one file look like), and
# they have to answer them the SAME way: a gate that resolved its baseline
# differently from its sibling would hold new code to a different definition of
# "new", which is the drift both of them exist to catch.
#
# Every function is total — it echoes an empty string rather than failing —
# because the callers are fail-open by design: outside a git checkout, or in a
# shallow CI clone with no origin/main, a diff-scoped gate has nothing to say
# and must not turn a lane red for saying it.
#
# Sourced, never executed:
#   . "$(dirname "$0")/lib/diff-scope.sh"

# The two trees a frontend gate reads, spelled ONCE for every gate that reads
# them: frontend/src is the core UI and extensions/*/frontend is the unit tier,
# whose screens ship in the same bundle and are held to the same rules. Two
# gates keeping their own copies is how the two trees came to disagree in the
# first place.
#
# ONE pattern per tree, and it is the plain one. A git pathspec is not :(glob)
# magic, so its `*` spans directory separators: 'frontend/src/*.tsx' already
# matches every depth under frontend/src. It is `**/` that carries the
# requirement — an intermediate directory — so '<tree>/**/*.tsx' silently misses
# a file sitting DIRECTLY in <tree>. That is what the extension entry was, on
# its own, while every unit screen sits at extensions/<unit>/frontend/screen.tsx.
# Do not add a `**/` sibling back: it can only ever match a subset.
#
# check-ds-spacing.test.sh holds the census these have to keep collecting.
DIFF_SCOPE_TSX=('frontend/src/*.tsx' 'extensions/*/frontend/*.tsx')
DIFF_SCOPE_CSS=('frontend/src/*.css' 'extensions/*/frontend/*.css')

# The repository root holding a given directory, or "" when that is not a git
# checkout (a source export, a container refusing a dubious-ownership repo).
diff_scope_root() {
  local dir="$1"
  (cd "$dir" && git rev-parse --show-toplevel 2>/dev/null) || true
}

# The comparison point: the merge-base with origin/main, falling back to
# origin/main itself when no common ancestor resolves, and to "" when the remote
# ref is not in the clone at all.
diff_scope_base() {
  local root="$1"
  git -C "$root" rev-parse --verify --quiet origin/main >/dev/null || return 0
  git -C "$root" merge-base origin/main HEAD 2>/dev/null || echo origin/main
}

# The files matching PATHSPEC that this branch adds or edits: the tracked ones
# from the diff, plus the untracked ones, which `git diff` cannot see at all.
#
# A brand-new file is the strictest case there is — all of it is new code — so
# leaving it out would exempt exactly the change with the most to answer for.
diff_scope_changed() {
  local root="$1" base="$2"
  shift 2
  git -C "$root" diff --name-only --diff-filter=d "$base" -- "$@" 2>/dev/null || true
  git -C "$root" ls-files --others --exclude-standard -- "$@" 2>/dev/null || true
}

# One file's added-lines diff, tracked or not. An untracked file is rendered
# against /dev/null so it reads as a full-file addition and the caller needs no
# special case for it. `--no-index` exits non-zero whenever the two sides
# differ, which is the normal case here, so the status is deliberately dropped.
diff_scope_added_diff() {
  local root="$1" base="$2" file="$3"
  if git -C "$root" ls-files --error-unmatch "$file" >/dev/null 2>&1; then
    git -C "$root" diff --unified=0 "$base" -- "$file" 2>/dev/null || true
  else
    git -C "$root" diff --no-index --unified=0 -- /dev/null "$file" 2>/dev/null || true
  fi
}
