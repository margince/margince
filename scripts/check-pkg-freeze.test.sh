#!/usr/bin/env bash
# The published-surface freeze gate's own test (check-pkg-freeze.sh).
#
# The gate once keyed an allowlist entry on the merge-base sha. In the merge
# queue GITHUB_BASE_REF is empty and the merge base is the current tip of main,
# so a ratified break passed its pull request and failed every queue run. An
# entry is now keyed on its subject: the package and the symbol. These cases
# hold that, in enforce mode:
#
#   1. A ratified break passes on the pull request (GITHUB_BASE_REF=main).
#   2. The same entry passes in the merge queue: GITHUB_BASE_REF is empty and
#      the merge base is a later tip of main.
#   3. A break with no entry fails.
#   4. An entry for a different symbol does not excuse this one.
#   5. An entry with no reason, or a reason citing no issue, fails.
#   6. Advisory mode never fails, entry or not.
#
# The repo is a throwaway fixture. `go list` and apidiff are stubs: the gate's
# own logic is under test, not apidiff's.
#
# Usage: bash scripts/check-pkg-freeze.test.sh
set -uo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
source "$ROOT/scripts/lib-git-fixture.sh"

FAILURES=0
fail() { echo "FAIL: $*" >&2; FAILURES=$((FAILURES + 1)); }

fixture="$(mktemp -d)"
trap 'git -C "$fixture/repo" worktree prune > /dev/null 2>&1; rm -rf "$fixture"' EXIT
PKG=github.com/margince/margince/backend/pkg/ext

mkdir -p "$fixture/bin"
# go list prints the one published package; apidiff -incompatible prints
# whatever the case wrote to $FIXTURE_FINDINGS.
cat > "$fixture/bin/go" <<GO
#!/usr/bin/env bash
[[ "\$1" == list ]] || exit 70
echo $PKG
GO
cat > "$fixture/bin/apidiff" <<'APIDIFF'
#!/usr/bin/env bash
case "$1" in
  -w) : > "$2" ;;
  -incompatible) cat "$FIXTURE_FINDINGS" ;;
  *) exit 71 ;;
esac
APIDIFF
chmod +x "$fixture/bin/go" "$fixture/bin/apidiff"
export PATH="$fixture/bin:$PATH" APIDIFF=apidiff FIXTURE_FINDINGS="$fixture/findings"
unset PKG_FREEZE_BASE PKG_FREEZE_ALLOWLIST CI

repo="$fixture/repo"
mkdir -p "$repo/scripts" "$repo/backend/pkg/ext"
cp "$ROOT/scripts/check-pkg-freeze.sh" "$repo/scripts/"
cd "$repo" || exit 1
git init -q
git config user.name Fixture
git config user.email fixture@example.test
echo 'package ext' > backend/pkg/ext/ext.go
git add -A
git commit -qm published
git update-ref refs/remotes/origin/main HEAD
fork="$(git rev-parse HEAD)"

git switch -qc feature
echo 'package ext // changed' > backend/pkg/ext/ext.go
git commit -qam break

# main moves on while the pull request waits; the queue tests the PR merged
# onto that later tip.
git switch -qc later "$fork"
echo later > README
git add README
git commit -qm later
later="$(git rev-parse HEAD)"
git switch -qc queue
git merge -q --no-edit feature
git switch -q feature

echo '- Handler: changed from func() to func(int)' > "$FIXTURE_FINDINGS"

# gate <allowlist line> <base ref env> <head> runs the gate in enforce mode
# and answers its exit status.
gate() {
  printf '%s\n' "$1" > "$fixture/allowlist"
  git switch -q "$3"
  GITHUB_BASE_REF="$2" PKG_FREEZE_MODE=enforce PKG_FREEZE_ALLOWLIST="$fixture/allowlist" \
    bash scripts/check-pkg-freeze.sh > "$fixture/out" 2>&1
}

ratified="$PKG Handler widens Handler for tenant scoping, ratified in #11033"

git update-ref refs/remotes/origin/main "$fork"
gate "$ratified" main feature || { cat "$fixture/out" >&2; fail "1: a ratified break fails its pull request"; }

git update-ref refs/remotes/origin/main "$later"
gate "$ratified" "" queue || { cat "$fixture/out" >&2; fail "2: a ratified break fails the merge queue run"; }
[[ "$(git merge-base queue origin/main)" == "$later" ]] || fail "2: the queue fixture does not put the merge base on the later tip"

if gate "" "" queue; then fail "3: an unratified break passed"; fi
grep -q "$PKG Handler <what it costs" "$fixture/out" || fail "3: the gate does not print the ready-to-paste entry"

if gate "$PKG Other widens Other for tenant scoping, ratified in #11033" "" queue; then
  fail "4: an entry for another symbol excused this one"
fi

if gate "$PKG Handler" "" queue; then fail "5: an entry with no reason passed"; fi
if gate "$PKG Handler widens Handler for tenant scoping, no issue named" "" queue; then
  fail "5: an entry citing no issue passed"
fi

printf '' > "$fixture/allowlist"
git switch -q queue
GITHUB_BASE_REF="" PKG_FREEZE_MODE=advisory PKG_FREEZE_ALLOWLIST="$fixture/allowlist" \
  bash scripts/check-pkg-freeze.sh > "$fixture/out" 2>&1 || fail "6: advisory mode failed"

if [[ "$FAILURES" -ne 0 ]]; then
  echo "check-pkg-freeze.test: $FAILURES failure(s)" >&2
  exit 1
fi
echo "OK: check-pkg-freeze.test — subject-keyed entries pass the pull request and the merge queue"
