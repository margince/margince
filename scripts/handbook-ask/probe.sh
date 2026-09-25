#!/usr/bin/env bash
#
# probe.sh — ask the default document set (the shipped handbook) every question
# in a bank, and report which ones came back answered.
#
# A pure client of a running, seeded stack: it signs in and calls
# POST /v1/knowledge/corpora/{id}/ask exactly as the "Ask your documents"
# dialog does, so what it measures is what a user gets.
#
#   scripts/handbook-ask/probe.sh [questions.txt] [out.tsv]
#
#   API_BASE   the API, not the Vite port (default http://localhost:8080;
#              a linked worktree's stack prints its own port at startup)
#   PROBE_EMAIL / PROBE_PASSWORD   default the seed-dev admin
#   PARALLEL   concurrent asks (default 2 — a cloud writer rate-limits)
#
# `unreviewed` means the writer model failed (a provider 503, say), which says
# nothing about the handbook, so it is retried before it is reported.
set -euo pipefail

here=$(cd "$(dirname "$0")" && pwd)
QFILE=${1:-$here/questions.txt}
OUT=${2:-handbook-ask.tsv}
API=${API_BASE:-http://localhost:8080}
EMAIL=${PROBE_EMAIL:-admin@demo.test}
PASSWORD=${PROBE_PASSWORD:-demo-password-123}
PARALLEL=${PARALLEL:-2}
mkdir -p "$(dirname "$OUT")"
JAR=$(mktemp)
trap 'rm -f "$JAR" "$OUT.rows"' EXIT

curl -sf -c "$JAR" -H 'content-type: application/json' \
  -d "$(jq -nc --arg e "$EMAIL" --arg p "$PASSWORD" '{email:$e,password:$p}')" \
  "$API/v1/auth/login" >/dev/null \
  || { echo "probe: sign-in to $API as $EMAIL failed — is the stack up and seeded (make seed-dev)?" >&2; exit 1; }

CORPUS=$(curl -sf -b "$JAR" "$API/v1/knowledge/corpora" \
  | jq -r '.items[] | select(.default_ask == true) | .id' | head -1)
[[ -n "$CORPUS" ]] || { echo "probe: no default document set — has the handbook been filed yet?" >&2; exit 1; }

coverage=$(curl -sf -b "$JAR" "$API/v1/knowledge/corpora" \
  | jq -r --arg id "$CORPUS" '.items[] | select(.id == $id) | .coverage | "\(.chunks_embedded) \(.chunks_total)"')
read -r embedded total <<<"$coverage"
if [[ -z "$total" || "$embedded" != "$total" || "$total" == 0 ]]; then
  echo "probe: the handbook is still being read ($embedded of $total passages); wait and re-run" >&2
  exit 1
fi

ask_one() {
  local question=$1 body resp outcome try
  body=$(jq -nc --arg q "$question" '{question:$q}')
  for try in 1 2 3 4; do
    resp=$(curl -s -b "$JAR" -H 'content-type: application/json' -d "$body" \
      "$API/v1/knowledge/corpora/$CORPUS/ask" || true)
    outcome=$(jq -r '.outcome // empty' <<<"$resp" 2>/dev/null || true)
    [[ -n "$outcome" && "$outcome" != unreviewed ]] && break
    [[ $try -lt 4 ]] && sleep $((try * 5))
  done
  jq -r --arg q "$question" '[ $q, (.outcome // "error"),
      ([.claims[]?.document_name // empty] | unique | join(",")),
      ((.summary // "") | gsub("[\t\n]"; " ")) ] | @tsv' <<<"$resp" 2>/dev/null \
    || printf '%s\terror\t\t\n' "$question"
}
export -f ask_one
export JAR API CORPUS

grep -v -e '^[[:space:]]*#' -e '^[[:space:]]*$' "$QFILE" | tr '\n' '\0' \
  | xargs -0 -P "$PARALLEL" -I{} bash -c 'ask_one "$1"' _ {} > "$OUT.rows"
{ printf 'question\toutcome\tcited\tsummary\n'; sort "$OUT.rows"; } > "$OUT"

asked=$(wc -l < "$OUT.rows" | tr -d ' ')
answered=$(cut -f2 "$OUT.rows" | grep -c '^answered$' || true)
echo "answered $answered of $asked (passages: $total) — rows in $OUT"
cut -f2 "$OUT.rows" | sort | uniq -c
echo "not answered:"
awk -F'\t' '$2 != "answered" { printf "  [%s] %s\n", $2, $1 }' "$OUT.rows"

# An error row is a request that never got an answer either way, so the run
# measured less than it claims; fail rather than report a partial score.
if grep -q $'\terror\t' "$OUT.rows"; then
  echo "probe: some asks failed outright (outcome error); the score above is incomplete" >&2
  exit 1
fi
