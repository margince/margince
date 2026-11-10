#!/usr/bin/env bash
# strict-lint-packages.sh — prints the backend packages the strict lint pass
# loads: each one holding a .go file changed since the merge base with $1, one
# `./dir` per line.
# The pass reports only changed lines (--new-from-merge-base), and those lines
# sit in these packages, so loading the rest of the tree adds time and no
# finding. Prints nothing when no package changed.
#
# The scope matches `./...` from backend/: a nested module (backend/tools),
# testdata, and `_` or `.` directories are left out, as the go tool leaves them
# out. Committed, staged, unstaged and untracked files all count, because
# golangci's own diff reads the working tree too.
set -euo pipefail

base_ref="${1:?usage: strict-lint-packages.sh <base-ref>}"
root="$(git rev-parse --show-toplevel)"
cd "$root"

base="$(git merge-base "$base_ref" HEAD)" || {
	echo "strict-lint-packages: no merge base between $base_ref and HEAD" >&2
	exit 1
}

# True when dir (relative to backend/) belongs to a module nested in backend/.
in_nested_module() {
	local dir="$1"
	while [[ "$dir" != "." ]]; do
		[[ -f "backend/$dir/go.mod" ]] && return 0
		dir="$(dirname "$dir")"
	done
	return 1
}

{
	git diff --name-only "$base" -- 'backend/*.go'
	git ls-files --others --exclude-standard -- 'backend/*.go'
} | sed 's#^backend/##' | while IFS= read -r file; do
	dirname "$file"
done | sort -u | while IFS= read -r dir; do
	case "/$dir/" in
	*/testdata/* | */_* | */.[!/]* | */vendor/*) continue ;;
	esac
	# A package the change deleted outright has nothing left to lint.
	compgen -G "backend/$dir/*.go" >/dev/null || continue
	in_nested_module "$dir" && continue
	if [[ "$dir" == "." ]]; then echo "."; else echo "./$dir"; fi
done
