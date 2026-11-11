#!/usr/bin/env bash
# Published-surface freeze gate (ADR-0120 §3, EXT-P3; the ADR-0023
# Amendment 2 patch-lane arm): backend/pkg is frozen published API from
# its first release — it evolves additively or through versioned
# successors, never in place. This gate apidiffs every published package
# against the merge target and reports every INCOMPATIBLE change
# (removed package, removed or re-signatured symbol, narrowed interface);
# additive growth always passes.
#
# TWO REGIMES, derived from the tree (no config):
#   advisory — before the first v1+ release tag exists: the surface is
#     design-fluid, incompatible changes print as ADVISORY and never
#     block. What you see is what v1.0.0 will freeze.
#   enforce  — from the first v1+ tag: incompatible changes FAIL. A
#     ratified change is recorded in the allowlist as its exact apidiff
#     finding plus a reason that cites the issue (the gatekit waiver
#     shape; the gate prints the line to fill in). The key names no
#     commit, so it holds on the pull request and in the merge queue,
#     where the merge base is the moving tip of main. A later, different
#     break of the same symbol is a different finding and is not excused.
#     Unused entries warn until removed. Package REMOVALS are never
#     allowlistable (deprecate-then-major cycle).
# PKG_FREEZE_MODE=advisory|enforce overrides (testing the other regime).
#
# Baseline: the PR's merge target (origin/$GITHUB_BASE_REF in CI), else
# origin/main (the merge queue and local runs). PKG_FREEZE_BASE=<ref>
# overrides.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

indent() { sed 's/^/  /'; }

# Pinned tool invocation (`go run` at an exact version, the
# check-contract-breaking.sh pattern): x/exp carries no tags, so the pin
# is a pseudo-version.
APIDIFF="${APIDIFF:-go run golang.org/x/exp/cmd/apidiff@v0.0.0-20260718201538-764159d718ef}"

if [[ -n "${PKG_FREEZE_MODE:-}" ]]; then
  MODE="$PKG_FREEZE_MODE"
else
  # The first STABLE v1+ release tag arms enforcement — exact release
  # syntax only (vMAJOR.MINOR.PATCH, major ≥ 1), so a prerelease
  # (v1.0.0-rc.1) or an unrelated v1* tag cannot arm it early. A failed
  # tag enumeration fails the gate: an error must never silently weaken
  # a release-enforcement threshold to advisory.
  if ! RELEASE_TAGS="$(git tag --list 'v*')"; then
    echo "FAIL: pkg-freeze — cannot enumerate release tags (git tag failed)" >&2
    exit 1
  fi
  # Here-string, not a pipe: `grep -q` exits on its first match and SIGPIPEs a
  # producer mid-write, which `set -o pipefail` reports as a failed pipeline —
  # so a tag list long enough to block printf would read as "no release tag".
  if grep -qE '^v[1-9][0-9]*\.[0-9]+\.[0-9]+$' <<<"$RELEASE_TAGS"; then
    MODE=enforce
  else
    MODE=advisory
  fi
fi
case "$MODE" in
  advisory | enforce) ;;
  *) echo "check-pkg-freeze: unknown PKG_FREEZE_MODE='$MODE' (want advisory|enforce)" >&2; exit 2 ;;
esac

# A pull request compares against exactly its merge target: a missing
# origin/$GITHUB_BASE_REF is a broken checkout, never a reason to compare
# against some other branch. Only the merge queue and local runs, where
# GITHUB_BASE_REF is empty, use origin/main.
if [[ -n "${PKG_FREEZE_BASE:-}" ]]; then
  BASE_REF="$PKG_FREEZE_BASE"
elif [[ -n "${GITHUB_BASE_REF:-}" ]]; then
  BASE_REF="origin/$GITHUB_BASE_REF"
  if ! git rev-parse -q --verify "$BASE_REF" > /dev/null; then
    echo "FAIL: pkg-freeze — base ref $BASE_REF missing; fetch the merge target before this gate" >&2
    exit 1
  fi
elif git rev-parse -q --verify origin/main > /dev/null; then
  BASE_REF=origin/main
else
  # In CI a missing base ref is a broken checkout, never a skip; locally
  # it just means the remote was never fetched.
  if [[ "${CI:-}" = "true" ]]; then
    echo "FAIL: pkg-freeze — no base ref (origin/main missing); fetch-depth must cover the merge base" >&2
    exit 1
  fi
  echo "SKIP pkg-freeze: no origin ref found — fetch origin to arm the published-surface gate"
  exit 0
fi

BASE="$(git merge-base HEAD "$BASE_REF")"
BASE12="$(git rev-parse --short=12 "$BASE")"

# Only a successful, empty listing means nothing is published yet; a git
# error must fail rather than read as an empty surface.
if ! BASE_PKG_TREE="$(git ls-tree -d "$BASE" backend/pkg)"; then
  echo "FAIL: pkg-freeze — cannot list backend/pkg at merge-base $BASE12 (git ls-tree failed)" >&2
  exit 1
fi
if [[ -z "$BASE_PKG_TREE" ]]; then
  echo "OK: pkg-freeze ($MODE) — no published surface at $BASE_REF merge-base; nothing frozen yet"
  exit 0
fi

WORKTREE="$(mktemp -d "${TMPDIR:-/tmp}/pkg-freeze.XXXXXX")"
EXPORT_DIR="$(mktemp -d "${TMPDIR:-/tmp}/pkg-freeze-export.XXXXXX")"
cleanup() {
  git worktree remove --force "$WORKTREE" > /dev/null 2>&1 || true
  rm -rf "$WORKTREE" "$EXPORT_DIR"
}
trap cleanup EXIT
git worktree add --detach --quiet "$WORKTREE" "$BASE"

# Fail-closed package discovery: a module or loading error must never
# read as "nothing frozen" — only the go tool's own "matched no
# packages" does.
golist_err="$(mktemp "${TMPDIR:-/tmp}/pkg-freeze-golist.XXXXXX")"
if ! OLD_PKGS="$(cd "$WORKTREE/backend" && go list ./pkg/... 2> "$golist_err")"; then
  if grep -q "matched no packages" "$golist_err"; then
    OLD_PKGS=""
  else
    echo "FAIL: pkg-freeze — cannot load the baseline's published packages:" >&2
    indent < "$golist_err" >&2
    rm -f "$golist_err"
    exit 1
  fi
fi
rm -f "$golist_err"
if [[ -z "$OLD_PKGS" ]]; then
  echo "OK: pkg-freeze ($MODE) — no published packages at the merge-base; nothing frozen yet"
  exit 0
fi

findings="$EXPORT_DIR/findings"
removals="$EXPORT_DIR/removals"
: > "$findings"
: > "$removals"
count=0
for pkg in $OLD_PKGS; do
  count=$((count + 1))
  rel="${pkg#github.com/margince/margince/backend/}"
  if [[ ! -d "backend/$rel" ]]; then
    echo "$pkg: package removed" >> "$removals"
    continue
  fi
  export_file="$EXPORT_DIR/$(echo "$pkg" | tr '/' '_').export"
  (cd "$WORKTREE/backend" && $APIDIFF -w "$export_file" "./$rel")
  (cd backend && $APIDIFF -incompatible "$export_file" "./$rel") \
    | sed -e 's/^- //' -e "s|^|$pkg: |" >> "$findings"
done

if [[ "$MODE" = "advisory" ]]; then
  if [[ -s "$removals" ]] || [[ -s "$findings" ]]; then
    echo "ADVISORY: pkg-freeze — incompatible published-surface changes vs $BASE_REF (design-fluid pre-v1.0.0; these HARD-FAIL from the first v1 release tag):"
    cat "$removals" "$findings" | indent
  else
    echo "OK: pkg-freeze (advisory) — published surface additive-or-unchanged vs $BASE_REF ($count packages)"
  fi
  exit 0
fi

# enforce: the allowlist licenses ratified findings, one exact finding
# per entry.
#
# An entry is `<package>: <finding> # <reason>`, the finding line exactly
# as the gate prints it. Go signatures never contain "#", so the first
# " # " ends the finding. The reason must state the cost in at least 20
# characters (inner whitespace squeezed, multibyte characters counted
# once), cite the ratifying issue as #<number>, and say more than the
# symbol. The key names no commit: the merge base differs between the
# pull request and the merge queue.
ALLOWLIST="${PKG_FREEZE_ALLOWLIST:-scripts/pkg-freeze-allowlist.txt}"
# Entries are squeezed the same way, so spacing never decides a match.
awk '{ gsub(/[[:space:]]+/, " "); sub(/^ /, ""); sub(/ $/, ""); if ($0 != "") print }' "$findings" > "$findings.squeezed"
mv "$findings.squeezed" "$findings"
ratified="$EXPORT_DIR/ratified"
malformed="$EXPORT_DIR/malformed"
: > "$ratified"
: > "$malformed"
if [[ -f "$ALLOWLIST" ]]; then
  # LC_ALL=C makes every awk count bytes; dropping UTF-8 continuation
  # bytes then leaves one byte per character.
  LC_ALL=C awk -v ratified="$ratified" -v malformed="$malformed" '
    function squeeze(s) { gsub(/[[:space:]]+/, " ", s); sub(/^ /, "", s); sub(/ $/, "", s); return s }
    function chars(s) { gsub(/[\200-\277]/, "", s); return length(s) }
    /^[[:space:]]*(#|$)/ { next }
    {
      at = index($0, " # ")
      finding = squeeze(at ? substr($0, 1, at - 1) : $0)
      reason = at ? squeeze(substr($0, at + 3)) : ""
      sym = finding
      sub(/^[^ ]*: /, "", sym)
      sub(/: .*/, "", sym)
      said = reason
      gsub(/#[0-9]+/, "", said)
      gsub(/[^[:alnum:]_]+/, " ", said)
      said = tolower(squeeze(said))
      if (finding !~ /^[^ ]+: [^ ]/ || chars(reason) < 20 || reason !~ /#[0-9]+/ ||
          said == "" || said == tolower(sym)) {
        print "line " NR ": " $0 > malformed
        next
      }
      print finding > ratified
    }
  ' "$ALLOWLIST"
fi

violations="$(grep -Fxv -f "$ratified" "$findings" || true)"
unused="$(sort -u "$ratified" | grep -Fxv -f "$findings" || true)"

failed=0
if [[ -s "$malformed" ]]; then
  echo "FAIL: pkg-freeze — malformed $ALLOWLIST entries (want '<package>: <finding> # <reason of 20+ characters citing #<issue>>'):" >&2
  indent < "$malformed" >&2
  failed=1
fi
if [[ -s "$removals" ]]; then
  echo "FAIL: pkg-freeze — published package removed (never allowlistable; deprecate, then remove with its major cycle — ADR-0120 §3):" >&2
  indent < "$removals" >&2
  failed=1
fi
if [[ -n "$violations" ]]; then
  echo "FAIL: pkg-freeze — incompatible published-surface change vs $BASE_REF (merge-base $BASE12):" >&2
  echo "$violations" | indent >&2
  echo "A ratified change is recorded in $ALLOWLIST as one line per finding (visible in the PR):" >&2
  echo "$violations" | sed 's/$/ # <what it costs, ratified in #issue>/' | indent >&2
  failed=1
fi
if [[ -n "$unused" ]]; then
  echo "WARN: pkg-freeze — allowlist entries match no finding (the ratified change has merged, or the entry has a typo — remove them):"
  echo "$unused" | indent
fi

if [[ "$failed" -ne 0 ]]; then
  echo "pkg-freeze: the published surface changes additively or via versioned successors, never in place (EXT-P3)." >&2
  exit 1
fi
active="$(sort -u "$ratified" | grep -Fx -f "$findings" | grep -c . || true)"
echo "OK: pkg-freeze (enforce) — published surface additive-or-unchanged vs $BASE_REF ($count packages, $active active exceptions)"
