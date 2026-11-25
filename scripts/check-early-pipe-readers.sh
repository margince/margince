#!/usr/bin/env bash
# check-early-pipe-readers.sh — fail on a pipeline in a pipefail script whose
# last reader stops before its writer is done (grep -q, grep -m, head, an awk or
# sed program that exits or quits). The writer then dies of SIGPIPE, the
# pipeline returns 141, and an `if`, `&&` or `set -e` reads that as the answer.
# Fix with a variable (`out="$(writer)"; grep -q x <<<"$out"`) or a reader that
# reads to the end (`sed -n 1p`, `grep ... >/dev/null`, awk without exit).
# A line that is safe for a reason a pattern cannot see carries
# `# early-pipe-ok: <reason>`.
#
# Usage: check-early-pipe-readers.sh [file ...]   (default: every tracked *.sh)
set -euo pipefail

cd "$(git rev-parse --show-toplevel)"

if [[ $# -gt 0 ]]; then
  files=("$@")
else
  files=()
  while IFS= read -r f; do files+=("$f"); done < <(git ls-files '*.sh')
fi

scanned=0
bad=0
for f in "${files[@]}"; do
  [[ -f "$f" ]] || continue
  grep -qE '^[[:space:]]*set[[:space:]]+-[a-zA-Z]*o[[:space:]]+pipefail|^[[:space:]]*set[[:space:]]+-o[[:space:]]+pipefail' "$f" || continue
  scanned=$((scanned + 1))
  # Matches lines, not statements: a trailing `|` joins the next line, and a
  # quoted `|` inside a string can false-positive (waive it with early-pipe-ok).
  if ! awk -v file="$f" '
    function openquote(t,   n) { n = gsub(/'\''/, "", t); return n % 2 == 1 }
    function flag(n, text) { printf "%s:%d: %s\n", file, n, text; found = 1 }
    {
      line = $0
      if (line ~ /early-pipe-ok:/) { waive = 1 }
      if (line ~ /^[[:space:]]*#/) { next }
      if (carry != "") { cur = carry " " line; startn = carryn } else { cur = line; startn = NR }
      if (line ~ /\|[[:space:]]*\\?[[:space:]]*$/ && line !~ /\|\|[[:space:]]*\\?[[:space:]]*$/) { carry = cur; carryn = startn; next }
      # An awk or sed program is quoted and may span lines: keep reading until
      # its opening quote closes, so an exit on a later line is seen.
      if (match(cur, /\|[[:space:]]*(\\[[:space:]]*)?(awk|sed)[[:space:]]/) && openquote(substr(cur, RSTART)) && ++joined < 60) {
        carry = cur; carryn = startn; next
      }
      joined = 0
      carry = ""
      if (waive) { waive = 0; next }
      stmt = cur
      gsub(/\|\|/, "  ", stmt)
      if (stmt ~ /\|[[:space:]]*(\\[[:space:]]*)?(grep|egrep|fgrep)([[:space:]]+-[a-zA-Z]+)*[[:space:]]+(-[a-zA-Z]*[qm][a-zA-Z0-9]*|--quiet|--max-count[= ])/ ||
          stmt ~ /\|[[:space:]]*(\\[[:space:]]*)?head([[:space:]]|$)/ ||
          stmt ~ /\|[[:space:]]*(\\[[:space:]]*)?tar[[:space:]]+-?[a-zA-Z]*x/ ||
          stmt ~ /\|[[:space:]]*(\\[[:space:]]*)?(awk|sed)[[:space:]].*(exit|[;{][[:space:]]*q[[:space:]]*[;}'\''"]|\/q[;'\'']|[0-9]q['\''[:space:]])/) {
        flag(startn, cur)
      }
    }
    END { exit found ? 1 : 0 }
  ' "$f"; then
    bad=1
  fi
done

if [[ $scanned -eq 0 ]]; then
  echo "check-early-pipe-readers: scanned no pipefail script; the file census is empty, so a pass would mean nothing." >&2
  exit 1
fi
if [[ $bad -ne 0 ]]; then
  echo "check-early-pipe-readers: a reader above stops before its writer finishes; under pipefail the writer dies of SIGPIPE and the pipeline reads as failed. Capture the output in a variable first, or use a reader that reads to the end." >&2
  exit 1
fi
echo "check-early-pipe-readers: $scanned pipefail scripts, no early-closing pipe readers"
