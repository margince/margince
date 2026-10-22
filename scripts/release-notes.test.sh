#!/usr/bin/env bash
# The release notes builder's own test. Its one caller is a tag push, so
# without this the builder's first run is the release page it writes.
#
# Usage: bash scripts/release-notes.test.sh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
BUILD="$SCRIPT_DIR/release-notes.sh"

FAILURES=0

# writes <reason> <expected notes> [budget]; the subjects come from stdin.
writes() {
	local got
	got="$(bash "$BUILD" v1.0.0 v1.1.0 margince/margince "${3:-100000}")"
	if [[ "$got" != "$2" ]]; then
		printf 'FAIL: %s\n--- want\n%s\n--- got\n%s\n' "$1" "$2" "$got"
		FAILURES=$((FAILURES + 1))
	fi
}

compare='Every change since v1.0.0: https://github.com/margince/margince/compare/v1.0.0...v1.1.0'

writes 'each kind a user notices gets its section, in a fixed order' "$(
	cat <<EOF
### Breaking

- **api:** offers drop the legacy total (#12)

### Added

- **deals:** a deal shows its next step (#10)

### Fixed

- **sign-in:** a link opened before sign-in opens after it (#11)
- dialogs survive a drag (#14)

### Faster

- **reports:** the pipeline report reads one query (#13)

$compare
EOF
)" <<'EOF'
feat(deals): a deal shows its next step (#10)
fix(sign-in): a link opened before sign-in opens after it (#11)
feat(api)!: offers drop the legacy total (#12)
perf(reports): the pipeline report reads one query (#13)
fix: dialogs survive a drag (#14)
EOF

writes 'work a user cannot notice, and subjects with no type, are left to the compare view' "$compare" <<'EOF'
refactor(store): one writer for the audit row (#20)
test(e2e): wait for the list to render (#21)
docs: the release how-to names the milestone (#22)
ci(release): close the milestone (#23)
Update dependency happy-dom to v20.14.6 (#24)
feature: not a conventional type (#25)
EOF

writes 'a release with nothing listed still links every change' "$compare" </dev/null

writes 'over the budget, fixes collapse first and breaking changes stay' "$(
	cat <<EOF
### Breaking

- **api:** offers drop the legacy total (#12)

### Added

- **deals:** a deal shows its next step (#10)

### Fixed

- 2 more, listed in the compare view.

$compare
EOF
)" 260 <<'EOF'
feat(deals): a deal shows its next step (#10)
feat(api)!: offers drop the legacy total (#12)
fix(sign-in): a link opened before provider sign-in opens after it (#11)
fix(frontend): dialogs ask before dropping typed text and survive a drag (#14)
EOF

writes 'far over the budget, features collapse too' "$(
	cat <<EOF
### Breaking

- **api:** offers drop the legacy total (#12)

### Added

- 1 more, listed in the compare view.

### Fixed

- 1 more, listed in the compare view.

$compare
EOF
)" 248 <<'EOF'
feat(deals): a deal shows its next step (#10)
feat(api)!: offers drop the legacy total (#12)
fix(sign-in): a link opened before sign-in opens after it (#11)
EOF

writes 'a shortened section keeps its own kind, and an empty kind stays out' "$(
	cat <<EOF
### Faster

- 2 more, listed in the compare view.

$compare
EOF
)" 150 <<'EOF'
perf(reports): one query for the whole pipeline report, not one per stage (#13)
perf(search): the index answers prefix search without a scan of every row (#15)
EOF

if printf 'feat(api)!: offers drop the legacy total (#12)\n' |
	bash "$BUILD" v1.0.0 v1.1.0 margince/margince 10 >/dev/null 2>&1; then
	echo "FAIL: notes that cannot fit the room were printed instead of refused"
	FAILURES=$((FAILURES + 1))
fi

if bash "$BUILD" v1.0.0 v1.1.0 </dev/null >/dev/null 2>&1; then
	echo "FAIL: a call with no repository was accepted"
	FAILURES=$((FAILURES + 1))
fi

if ((FAILURES)); then
	printf '%d case(s) failed\n' "$FAILURES"
	exit 1
fi
echo "release-notes: every case holds"
