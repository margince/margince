#!/usr/bin/env bash
# Reconcile this repository's labels from .github/labels.yml.
#
# The file is the source (issue #1508). Without this, it is a fourth copy of a
# list that already had three — and the copy that does anything is still the one
# on GitHub, which nothing derives from. This is what makes the tree's copy true.
#
# ADDS AND UPDATES; NEVER DELETES. A label removed from the file stays on the
# repository and is REPORTED here instead, because deleting one strips it from
# every issue that carries it and there is no undo — an issue's provenance is
# the one thing nobody can reconstruct later (docs/reference/issue-labels.md
# says so about these very labels). Retiring a label is a decision with a cost,
# so it belongs to a person who has decided to pay it, not to a sync run.
#
# `gh label create --force` is create-or-update, so the two cases are one call
# and the run is idempotent: a second pass changes nothing and says so.
#
# No third-party action. Every `uses:` in this repo is pinned to a commit sha,
# and a label reconciler is twenty lines of `gh` — a pinned dependency to run
# them would be more supply chain than the task is worth.
#
# RUN BY HAND for now, and by a workflow once one exists: the workflow that
# would call this on a push to main is the one piece of #1508 not in this tree,
# because pushing a file under .github/workflows/ needs an OAuth scope the agent
# that wrote this does not hold. Until then the file is authoritative the moment
# somebody runs this, and the gate keeps the reference page honest either way.
set -euo pipefail
cd "$(dirname "$0")/.."

readonly SOURCE=".github/labels.yml"
[[ -f "$SOURCE" ]] || { echo "FAIL: $SOURCE is missing"; exit 1; }

# Parsed with awk rather than a YAML reader, because the file's shape is fixed
# by this script and the gate that reads it: three scalar fields in a fixed
# order. A parser that accepted more than those two do would let a file through
# that one of them then rejects.
declared=0
while IFS=$'\t' read -r name color description; do
  declared=$((declared + 1))
  if gh label create "$name" --color "$color" --description "$description" --force >/dev/null; then
    echo "ok: $name"
  else
    echo "FAIL: could not reconcile $name"
    exit 1
  fi
done < <(awk '
  /^- name: "/ { name = $0; sub(/^- name: "/, "", name); sub(/"$/, "", name); next }
  /^  color: "/ { color = $0; sub(/^  color: "/, "", color); sub(/"$/, "", color); next }
  /^  description: "/ {
    description = $0
    sub(/^  description: "/, "", description); sub(/"$/, "", description)
    printf "%s\t%s\t%s\n", name, color, description
    name = ""; color = ""; description = ""
  }
' "$SOURCE")

# A sync that parsed nothing would report success over a file it could not read,
# which is the failure this whole exercise is about.
if [[ "$declared" -eq 0 ]]; then
  echo "FAIL: $SOURCE declared no label this script could read — the file is empty, or its"
  echo "shape changed and the parser above now matches nothing."
  exit 1
fi

echo
echo "reconciled $declared label(s) from $SOURCE"

# What the repository has and the file does not name. Reported, never removed.
extra="$(comm -13 \
  <(awk '/^- name: "/ { n = $0; sub(/^- name: "/, "", n); sub(/"$/, "", n); print n }' "$SOURCE" | sort) \
  <(gh label list --limit 200 --json name --jq '.[].name' | sort))"
if [[ -n "$extra" ]]; then
  echo
  echo "NOT IN $SOURCE, and left alone:"
  # One line at a time, quoted. A label name may hold spaces — `good first issue`
  # and `help wanted` both do — and an unquoted expansion would report one label
  # as three, then let the shell try to glob any `*` or `?` in it. The report is
  # what somebody reads before deciding to retire a label; it has to name the
  # label they would type.
  while IFS= read -r label; do
    printf '  %s\n' "$label"
  done <<<"$extra"
  echo
  echo "Each is a label somebody added by hand. Add it to the file if it belongs to the"
  echo "taxonomy, or retire it deliberately with 'gh label delete' — which strips it from"
  echo "every issue carrying it, and cannot be undone."
fi
