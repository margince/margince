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
#   4. An entry excuses its exact finding only: not the same symbol in another
#      package, not a longer name (Handler vs HandlerExtra), not a member (T vs
#      T.M), and not a later, different break of the same symbol.
#   5. A reason that is padded, short in characters, restates the symbol, or
#      cites no issue fails.
#   6. An unused entry warns and does not fail.
#   7. A failing git ls-tree and a missing origin/$GITHUB_BASE_REF fail closed.
#   8. Advisory mode never fails, entry or not.
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

REAL_GIT="$(command -v git)"
# A git that fails `ls-tree` when the case asks, and is real git otherwise.
cat > "$fixture/bin/git" <<GIT
#!/usr/bin/env bash
if [[ -n "\${FIXTURE_LSTREE_FAILS:-}" && "\$1" == ls-tree ]]; then exit 128; fi
exec "$REAL_GIT" "\$@"
GIT
chmod +x "$fixture/bin/git"

broken='Handler: changed from func() to func(int)'
reason='widens Handler for tenant scoping, ratified in #11033'
entry="$PKG: $broken # $reason"

# gate <allowlist> <findings> <base ref env> <head> runs the gate in enforce
# mode and answers its exit status. Findings are written the way apidiff
# prints them; the gate prefixes the package.
gate() {
  printf '%s\n' "$1" > "$fixture/allowlist"
  printf '%s\n' "$2" | sed '/^$/d; s/^/- /' > "$FIXTURE_FINDINGS"
  git switch -q "$4"
  GITHUB_BASE_REF="$3" PKG_FREEZE_MODE=enforce PKG_FREEZE_ALLOWLIST="$fixture/allowlist" \
    bash scripts/check-pkg-freeze.sh > "$fixture/out" 2>&1
}
passes() { gate "$@" || { cat "$fixture/out" >&2; return 1; }; }

git update-ref refs/remotes/origin/main "$fork"
passes "$entry" "$broken" main feature || fail "1: a ratified break fails its pull request"

git update-ref refs/remotes/origin/main "$later"
passes "$entry" "$broken" "" queue || fail "2: a ratified break fails the merge queue run"
[[ "$(git merge-base queue origin/main)" == "$later" ]] || fail "2: the queue fixture does not put the merge base on the later tip"

if gate "" "$broken" "" queue; then fail "3: an unratified break passed"; fi
grep -qF "$PKG: $broken # <what it costs" "$fixture/out" || fail "3: the gate does not print the line to fill in"

if gate "github.com/margince/margince/backend/pkg/other: $broken # $reason" "$broken" "" queue; then
  fail "4: an entry for the same symbol in another package excused this one"
fi
if gate "$entry" "HandlerExtra: changed from func() to func(int)" "" queue; then
  fail "4: an entry for Handler excused HandlerExtra"
fi
if gate "$PKG: T: removed # $reason" "T.M: removed" "" queue; then
  fail "4: an entry for T excused T.M"
fi
if gate "$entry" "Handler: changed from func(int) to func(string)" "" queue; then
  fail "4: an entry for one break of Handler excused a later, different one"
fi
if gate "$entry" "$broken
Other: removed" "" queue; then
  fail "4: one entry excused a second finding in the same run"
fi

if gate "$PKG: $broken" "$broken" "" queue; then fail "5: an entry with no reason passed"; fi
if gate "$PKG: $broken # widens                        #11033" "$broken" "" queue; then
  fail "5: a reason padded with spaces passed"
fi
if gate "$PKG: $broken # ääääääääääää ok #1" "$broken" "" queue; then
  fail "5: a reason under 20 characters passed because it is over 20 bytes"
fi
long='TenantScopedHandlerRegistry: removed'
if gate "$PKG: $long # (TenantScopedHandlerRegistry) #11033" "$long" "" queue; then
  fail "5: a reason that is only the symbol and issue numbers passed"
fi
if gate "$PKG: $broken # widens Handler for tenant scoping, no issue named" "$broken" "" queue; then
  fail "5: an entry citing no issue passed"
fi

passes "$entry
$PKG: Gone: removed # $reason" "$broken" "" queue || fail "6: an unused entry failed the gate"
grep -q "WARN: pkg-freeze" "$fixture/out" || fail "6: an unused entry did not warn"

if FIXTURE_LSTREE_FAILS=1 gate "$entry" "$broken" "" queue; then fail "7: a failing git ls-tree passed"; fi
grep -q "git ls-tree failed" "$fixture/out" || fail "7: a failing git ls-tree did not name the failure"
if gate "$entry" "$broken" release-9 queue; then fail "7: a missing origin/\$GITHUB_BASE_REF passed"; fi
grep -q "origin/release-9 missing" "$fixture/out" || fail "7: a missing base ref did not name the ref"

printf '' > "$fixture/allowlist"
printf -- '- %s\n' "$broken" > "$FIXTURE_FINDINGS"
git switch -q queue
GITHUB_BASE_REF="" PKG_FREEZE_MODE=advisory PKG_FREEZE_ALLOWLIST="$fixture/allowlist" \
  bash scripts/check-pkg-freeze.sh > "$fixture/out" 2>&1 || fail "8: advisory mode failed"
grep -q "ADVISORY" "$fixture/out" || fail "8: advisory mode did not print the break"

if [[ "$FAILURES" -ne 0 ]]; then
  echo "check-pkg-freeze.test: $FAILURES failure(s)" >&2
  exit 1
fi
echo "OK: check-pkg-freeze.test — entries excuse their exact finding on the pull request and in the merge queue"
