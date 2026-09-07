#!/usr/bin/env bash
# The jurisdiction gate's own test — it gates the gate's REACH, which is the
# half that failed silently.
#
# check-no-jurisdiction.sh scanned --include='*.go' and nothing else while its
# header said it guards "core code". Core SQL was invisible, and core SQL does
# name a jurisdiction: the basis-kind CHECK in migration 1788407500 carries
# 'vn_subject_agreement' twice. The gate reported OK over a leak it could not
# see, which is the one way a census must not fail — under-recognition leaves no
# failing assertion to notice.
#
# So the reach is asserted rather than trusted. Four properties, each with a
# planted defect the gate must catch and a legitimate shape it must not:
#
#   1. A jurisdiction string in core GO fails.   (the original arm, still armed)
#   2. A jurisdiction string in core SQL fails.  (the arm that did not exist)
#   3. The same token in a GO comment passes.    (the header's own promise)
#   4. The same token in a SQL comment passes.   (that promise, in SQL)
#   5. A token after a `--` INSIDE a SQL literal fails. (the strip must not eat code)
#   6. The same, after a `//` inside a Go literal.      (the older twin of it)
#   7. The same, after an ESCAPED quote.                (a `\"` does not close a run)
#   8. An ISO code in a SINGLE-quoted SQL literal.      (SQL does not quote like Go)
#
# 3 and 4 are not politeness: without them the SQL arm would fire on the very
# first thing it meets — an Impressum named as motivation in a migration comment
# — and the honest fix would look like narrowing the gate rather than teaching
# it to read.
#
# The tree is FAKE and built here. Running the real gate against a mutated
# working tree would be a test that edits the repo to prove a point, and a
# failure partway through would leave the edit behind.
#
# Usage: bash scripts/check-no-jurisdiction.test.sh
set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
GATE="$SCRIPT_DIR/check-no-jurisdiction.sh"
FAILURES=0

fail() { echo "FAIL: $*" >&2; FAILURES=$((FAILURES + 1)); }

# A throwaway repo shaped like this one: the gate cds to its own parent and
# scans backend/internal plus backend/migrations, so those are what a fixture
# needs. The gate itself is copied in rather than symlinked, because it resolves
# its scan root from its own path.
newtree() {
  local root
  root="$(mktemp -d)"
  mkdir -p "$root/scripts" "$root/backend/internal/modules/x" "$root/backend/migrations/core"
  cp "$GATE" "$root/scripts/"
  # A file in each tree with nothing to find, so a fixture that fails to write
  # its subject is not indistinguishable from a clean one.
  printf 'package x\n\nconst ordinary = "nothing to see"\n' \
    > "$root/backend/internal/modules/x/x.go"
  printf 'CREATE TABLE ordinary (id uuid);\n' \
    > "$root/backend/migrations/core/fixture_baseline.up.sql"
  echo "$root"
}

# run answers the gate's exit status against one fixture tree.
run() {
  ( cd "$1" && bash scripts/check-no-jurisdiction.sh >/dev/null 2>&1 )
}

# The control: a tree with no jurisdiction string anywhere must PASS. Without
# this every assertion below could be passing because the fixture is broken.
clean="$(newtree)"
if ! run "$clean"; then
  fail "a tree with no jurisdiction string does not pass — the fixture is wrong, and every case below is meaningless"
fi
rm -rf "$clean"

# 1. Core Go, in code.
gohit="$(newtree)"
printf 'package x\n\nconst format = "ZUGFeRD"\n' \
  > "$gohit/backend/internal/modules/x/leak.go"
if run "$gohit"; then
  fail "a jurisdiction string in core Go passed"
fi
rm -rf "$gohit"

# 2. Core SQL, in code. THIS is the arm that did not exist.
sqlhit="$(newtree)"
printf "ALTER TABLE ordinary ADD COLUMN scheme text DEFAULT 'ZUGFeRD';\n" \
  > "$sqlhit/backend/migrations/core/fixture_leak.up.sql"
if run "$sqlhit"; then
  fail "a jurisdiction string in core SQL passed — the gate is reading a smaller tree than its subject"
fi
rm -rf "$sqlhit"

# 3. Core Go, in a comment: permitted, and the header says so.
gocomment="$(newtree)"
printf 'package x\n\n// ZUGFeRD is the motivating standard; nothing here is bound to it.\nconst generic = "invoice"\n' \
  > "$gocomment/backend/internal/modules/x/motivation.go"
if ! run "$gocomment"; then
  fail "a statute named in a GO comment failed the gate — the header promises comments are documentation, not code"
fi
rm -rf "$gocomment"

# 4. Core SQL, in a comment: the same promise, in the language the gate just
# learned to read. Migration 1788009369 really does name an Impressum this way.
sqlcomment="$(newtree)"
printf -- "-- ZUGFeRD is the motivating standard; the column below is generic.\nALTER TABLE ordinary ADD COLUMN scheme text;\n" \
  > "$sqlcomment/backend/migrations/core/fixture_motivation.up.sql"
if ! run "$sqlcomment"; then
  fail "a statute named in a SQL comment failed the gate — extending the reach must not break the header's own promise"
fi
rm -rf "$sqlcomment"

# 5. A jurisdiction string AFTER a `-- ` that sits inside a SQL string literal.
#
# This is the case the first version of the SQL arm fell through. An unanchored
# strip cut at the first ` -- ` and blanked the rest of the line, so the leak
# after it was invisible — the gate reading less than its subject, reintroduced
# by the change that widened its reach. `-- ` inside quoted prose is ordinary in
# a migration, so this needs no attacker.
sqlquoted="$(newtree)"
printf "INSERT INTO ordinary (note, scheme) VALUES ('a -- b', 'ZUGFeRD');\n" \
  > "$sqlquoted/backend/migrations/core/fixture_quoted.up.sql"
if run "$sqlquoted"; then
  fail "a jurisdiction string after a '--' inside a SQL literal passed — the comment strip is eating code"
fi
rm -rf "$sqlquoted"

# 6. The same evasion in Go, which the unanchored strip had always allowed.
# Fixing the SQL arm without this would leave the older hole open beside it.
goquoted="$(newtree)"
printf 'package x\n\nconst u = "http://x // ZUGFeRD"\n' \
  > "$goquoted/backend/internal/modules/x/quoted.go"
if run "$goquoted"; then
  fail "a jurisdiction string after a '//' inside a Go literal passed — the comment strip is eating code"
fi
rm -rf "$goquoted"

# 7. A jurisdiction string after a `//` that follows an ESCAPED quote.
#
# The quote walker has to know that `\"` does not close a run. Without that,
# the literal reads as closed at the escaped quote, the `//` after it looks like
# a comment marker outside quotes, and everything past it — including the leak —
# is cut. Codex found this; the first quote-aware version had it.
goescaped="$(newtree)"
printf 'package x\n\nconst s = "a \\" // ZUGFeRD"\n' \
  > "$goescaped/backend/internal/modules/x/escaped.go"
if run "$goescaped"; then
  fail "a jurisdiction string after an escaped quote passed — the walker thinks the literal closed"
fi
rm -rf "$goescaped"

# 8. An ISO country code in a SQL literal, which uses SINGLE quotes.
#
# The ISO pattern matched only Go's double quote. Extending the scan to SQL
# without extending the pattern would announce a reach the gate did not have.
sqliso="$(newtree)"
printf "ALTER TABLE ordinary ADD CONSTRAINT c CHECK (jurisdiction <> 'DE');\n" \
  > "$sqliso/backend/migrations/core/fixture_iso.up.sql"
if run "$sqliso"; then
  fail "an ISO country code in a single-quoted SQL literal passed — the pattern reads only Go quotes"
fi
rm -rf "$sqliso"

if [[ $FAILURES -gt 0 ]]; then
  echo "check-no-jurisdiction.test.sh: $FAILURES case(s) failed" >&2
  exit 1
fi
echo "==> jurisdiction gate reach: core Go and core SQL both read, comments spared in both"
