#!/usr/bin/env bash
# ci-change-base.sh's own test: the rule for each event, a base the clone has to
# fetch, and both ways a run with no base ends (exit 1, or nothing with
# --fail-open). A rule that stopped matching would print a base for no event, and
# each case names the commit it must print.
#
# Usage: bash scripts/ci-change-base.test.sh

set -euo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
SCRIPT="$HERE/ci-change-base.sh"
# shellcheck source=scripts/lib-git-fixture.sh
source "$HERE/lib-git-fixture.sh"

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

FAILURES=0
fail() {
  echo "FAIL: $*" >&2
  FAILURES=$((FAILURES + 1))
}

commit() {
  git -C "$1" -c user.name=Fixture -c user.email=fixture@example.test commit --quiet --allow-empty -m "$2"
  git -C "$1" rev-parse HEAD
}

ORIGIN="$TMP/origin"
git init --quiet "$ORIGIN"
git -C "$ORIGIN" config uploadpack.allowAnySHA1InWant true
git -C "$ORIGIN" checkout --quiet -b main
ONE="$(commit "$ORIGIN" one)"
git -C "$ORIGIN" checkout --quiet -b feature
commit "$ORIGIN" feature >/dev/null
git -C "$ORIGIN" checkout --quiet main
TWO="$(commit "$ORIGIN" two)"
git -C "$ORIGIN" checkout --quiet -b pr-merge
git -C "$ORIGIN" -c user.name=Fixture -c user.email=fixture@example.test merge --quiet --no-edit feature
MERGE="$(git -C "$ORIGIN" rev-parse HEAD)"
git -C "$ORIGIN" checkout --quiet main

# A CI checkout: the one commit, and as many parents as the depth allows.
checkout_at() {
  local dir="$TMP/$1" sha="$2" depth="$3"
  git init --quiet "$dir"
  git -C "$dir" remote add origin "file://$ORIGIN"
  git -C "$dir" fetch --quiet --no-tags --depth="$depth" origin "$sha"
  git -C "$dir" checkout --quiet FETCH_HEAD
  echo "$dir"
}

mkdir -p "$TMP/bin"
cat >"$TMP/bin/curl" <<'FAKE'
#!/usr/bin/env bash
[[ -n "${COMPARE_SHA:-}" ]] || exit 22
echo "{\"merge_base_commit\":{\"sha\":\"$COMPARE_SHA\"}}"
FAKE
chmod +x "$TMP/bin/curl"

# expect LABEL WANT_EXIT WANT_OUT DIR EVENT EVENT_JSON REF [FLAG]
expect() {
  local label="$1" want_exit="$2" want_out="$3" dir="$4" event="$5" json="$6" ref="$7" flag="${8:-}"
  printf '%s\n' "$json" >"$TMP/event.json"
  local got_exit=0
  (cd "$dir" && env GITHUB_EVENT_NAME="$event" GITHUB_EVENT_PATH="$TMP/event.json" GITHUB_REF="$ref" \
    GITHUB_SHA="$(git -C "$dir" rev-parse HEAD)" GITHUB_REPOSITORY=owner/repo \
    GITHUB_API_URL=https://api.example.invalid GH_TOKEN=token PATH="$TMP/bin:$PATH" \
    bash "$SCRIPT" ${flag:+"$flag"}) >"$TMP/out" 2>"$TMP/err" || got_exit=$?
  local got_out
  got_out="$(cat "$TMP/out")"
  if [[ "$got_exit" != "$want_exit" || "$got_out" != "$want_out" ]]; then
    fail "$label: exit $got_exit and '$got_out', want exit $want_exit and '$want_out'"
    sed 's/^/      /' "$TMP/err" >&2
  fi
}

PR="$(checkout_at pr "$MERGE" 2)"
expect "a pull request's merge commit" 0 "$TWO" "$PR" pull_request '{}' refs/pull/1/merge

TIP="$(checkout_at tip "$TWO" 2)"
expect "a scheduled run on main" 0 "$ONE" "$TIP" schedule '{}' refs/heads/main
expect "a run by hand on the default branch" 0 "$ONE" "$TIP" workflow_dispatch \
  '{"repository":{"default_branch":"main"}}' refs/heads/main

SHALLOW="$(checkout_at shallow "$MERGE" 1)"
expect "a merge group whose base the clone lacks" 0 "$ONE" "$SHALLOW" merge_group \
  "{\"merge_group\":{\"base_sha\":\"$ONE\"}}" refs/heads/gh-readonly-queue/main/pr-1
git -C "$SHALLOW" cat-file -e "$ONE^{commit}" || fail "the merge group's base was not fetched"

BRANCH="$(checkout_at branch "$MERGE" 1)"
COMPARE_SHA="$ONE" expect "a run by hand on another branch" 0 "$ONE" "$BRANCH" workflow_dispatch \
  '{"repository":{"default_branch":"main"}}' refs/heads/feature
COMPARE_SHA="" expect "a compare call that fails" 1 "" "$BRANCH" workflow_dispatch \
  '{"repository":{"default_branch":"main"}}' refs/heads/feature
COMPARE_SHA="" expect "a compare call that fails, --fail-open" 0 "" "$BRANCH" workflow_dispatch \
  '{"repository":{"default_branch":"main"}}' refs/heads/feature --fail-open

expect "a pull request with no parent in the clone" 1 "" "$BRANCH" pull_request '{}' refs/pull/1/merge
expect "an event with no rule" 1 "" "$PR" push '{}' refs/heads/main
grep -q "no base rule for the 'push' event" "$TMP/err" || fail "an event with no rule does not say which event"
expect "an event with no rule, --fail-open" 0 "" "$PR" push '{}' refs/heads/main --fail-open
expect "a base no remote holds" 1 "" "$PR" merge_group \
  '{"merge_group":{"base_sha":"0123456789abcdef0123456789abcdef01234567"}}' refs/heads/q
expect "a base no remote holds, --fail-open" 0 "" "$PR" merge_group \
  '{"merge_group":{"base_sha":"0123456789abcdef0123456789abcdef01234567"}}' refs/heads/q --fail-open

if [[ "$FAILURES" -ne 0 ]]; then
  echo "ci-change-base.sh no longer prints the base each event is judged against." >&2
  exit 1
fi
echo "==> ci-change-base: every event's base, the fetch, and both ends of a run with no base"
