#!/usr/bin/env bash
#
# home-budget-compare.sh — whether a change made Home slower than its merge
# base, judged from two runs of the home-budget check on one runner.
#
#   scripts/home-budget-compare.sh HEAD_LOG BASE_LOG
#
# Each log holds the lines the check prints: `home-budget: row NAME p50=Xms
# p95=Yms ...` from the API half and `bench-home: vitals day_paint_p50_ms=X
# day_paint_p95_ms=Y cls_p95=Z ...` from the browser half. A base built before
# p50 was printed contributes its p95 alone.
#
# A metric regresses when the change's value is above the base's by more than
# the margin: 20% of the base or 100 ms, whichever is larger; for CLS, 20% or
# 0.05. A metric the base lacks is listed and not judged. Runner speed moves
# both runs alike, so only the change's own cost is left in the difference.
#
# Prints a Markdown table. Exits 1 when any metric regressed, 0 otherwise,
# including when the base measured nothing (it says so).

set -euo pipefail

head_log="${1:?usage: $0 HEAD_LOG BASE_LOG}"
base_log="${2:?usage: $0 HEAD_LOG BASE_LOG}"

# metrics LOG prints "name value" per measured metric.
metrics() {
  awk '
    /home-budget: row / {
      for (i = 1; i <= NF; i++) {
        if ($i == "row") name = $(i + 1)
        if ($i ~ /^p(50|95)=[0-9]+ms$/) {
          split($i, kv, "=")
          sub(/ms$/, "", kv[2])
          print name "." kv[1], kv[2]
        }
      }
    }
    /bench-home: vitals / {
      for (i = 1; i <= NF; i++) {
        if ($i ~ /^day_paint_p(50|95)_ms=[0-9]+$/) {
          split($i, kv, "=")
          sub(/_ms$/, "", kv[1])
          sub(/^day_paint_/, "day_paint.", kv[1])
          print kv[1], kv[2]
        }
        if ($i ~ /^cls_p95=[0-9.]+$/) {
          split($i, kv, "=")
          print "cls.p95", kv[2]
        }
      }
    }
  ' "$1"
}

head_metrics="$(metrics "$head_log")"
base_metrics="$(metrics "$base_log")"

if [[ -z "$head_metrics" ]]; then
  echo "home-budget-compare: the change measured nothing, so nothing can be compared" >&2
  exit 1
fi
if [[ -z "$base_metrics" ]]; then
  echo "home-budget-compare: the merge base measured nothing, so no metric is judged"
  exit 0
fi

awk '
  NR == FNR { base[$1] = $2; next }
  {
    name = $1; value = $2
    if (!(name in base)) {
      rows = rows sprintf("| %s | — | %s | — | not in base |\n", name, value)
      next
    }
    b = base[name] + 0
    floor = (name ~ /^cls\./) ? 0.05 : 100
    margin = b * 0.2
    if (margin < floor) margin = floor
    limit = b + margin
    verdict = "ok"
    if (value + 0 > limit) { verdict = "**slower**"; slower++ }
    fmt = (name ~ /^cls\./) ? "| %s | %s | %s | %.3f | %s |\n" : "| %s | %s | %s | %d | %s |\n"
    rows = rows sprintf(fmt, name, base[name], value, limit, verdict)
  }
  END {
    print "| Metric | Merge base | This change | Limit | Verdict |"
    print "| --- | --- | --- | --- | --- |"
    printf "%s", rows
    print ""
    if (slower > 0) {
      printf "home-budget-compare: %d metric(s) slower than the merge base past the margin\n", slower
      exit 1
    }
    print "home-budget-compare: no metric is slower than the merge base past the margin"
  }
' <(echo "$base_metrics") <(echo "$head_metrics")
