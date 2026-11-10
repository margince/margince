#!/usr/bin/env bash
# SPDX-License-Identifier: BUSL-1.1
# SPDX-FileCopyrightText: 2026 Gradion
#
# aicert-run-cost.sh prints what one `make e2e-ai` run took and cost: its time,
# the candidate's calls, tokens and serving provider, and a cost per model.
#
#   scripts/aicert-run-cost.sh <run log> <resume dir> [trace file]
#
# <run log> is the run's whole output. <resume dir> is the RESUME= it ran with;
# only journal rows written during the run count, so a reused journal is fine.
# The trace (default: the aicert-trace-*.jsonl beside the journal that this run
# started) names the provider that served each call.
#
# Prices: an OpenRouter model's price is looked up for the provider that served
# most of its calls. Any other model needs PRICE_<model>="<in>,<out>", in USD per
# million tokens, with every character outside [A-Za-z0-9] in the model id
# written as "_": PRICE_gemini_3_6_flash="0.75,3.75".
set -euo pipefail

log=${1:?usage: aicert-run-cost.sh <run log> <resume dir> [trace file]}
resume=${2:?usage: aicert-run-cost.sh <run log> <resume dir> [trace file]}
journal="$resume/aicert-resume.jsonl"
[[ -f $log ]] || { echo "aicert-run-cost: no run log at $log" >&2; exit 2; }
[[ -f $journal ]] || { echo "aicert-run-cost: no journal at $journal" >&2; exit 2; }

seconds=$(sed -nE 's/.*TestE2ECertify \(([0-9.]+)s\).*/\1/p' "$log" | tail -1)
[[ -n $seconds ]] || { echo "aicert-run-cost: $log has no TestE2ECertify line; did the run finish?" >&2; exit 2; }
seconds=${seconds%.*}
# The log ends when the run does; a minute of slack covers the build before the test starts.
end=$(date -r "$log" +%s)
start=$((end - seconds - 60))

trace=${3:-}
if [[ -z $trace ]]; then
	# A trace is named for the second its run started, in UTC.
	first=$(jq -nr --argjson t "$start" '$t | strftime("aicert-trace-%Y%m%dT%H%M%SZ")')
	last=$(jq -nr --argjson t "$end" '$t | strftime("aicert-trace-%Y%m%dT%H%M%SZ")')
	trace=$(find "$(dirname "$resume")" -maxdepth 1 -name 'aicert-trace-*.jsonl' -print 2>/dev/null | sort |
		awk -F/ -v a="$first" -v b="$last" '{ n = $NF; sub(/\.jsonl$/, "", n) } n >= a && n <= b { print; exit }')
fi

calls=$(grep 'INFO ai.call' "$log" | grep -vc 'provider=claude_cli' || true)
judge_calls=$(grep -c 'task=cert_judge' "$log" || true)

printf 'Time: %d minutes (%ss)\n' $(((seconds + 30) / 60)) "$seconds"
printf 'Model calls: %s (judge calls: %s)\n' "$calls" "$judge_calls"

jq -s --argjson start "$start" --argjson end "$end" -r '
	map(select((.at | sub("\\.[0-9]+Z$"; "Z") | fromdateiso8601) as $t | $t >= $start and $t <= $end))
	| group_by(.outcome.served_model)[]
	| [.[0].outcome.served_model, length, (map(.outcome.tokens_in // 0) | add), (map(.outcome.tokens_out // 0) | add)]
	| @tsv' "$journal" |
	while IFS=$'\t' read -r model runs tokens_in tokens_out; do
		provider=""
		if [[ -n $trace && -f $trace ]]; then
			provider=$(jq -r --arg m "$model" 'select(.role == "candidate" and .served_model == $m) | .served_provider' "$trace" |
				sort | uniq -c | sort -rn | awk 'NR == 1 { $1 = ""; sub(/^ /, ""); print }')
		fi
		key="PRICE_$(printf '%s' "$model" | tr -c 'A-Za-z0-9' '_')"
		price=${!key:-}
		if [[ -z $price && $model == */* && -n $provider ]]; then
			price=$(curl -fsS "https://openrouter.ai/api/v1/models/$model/endpoints" |
				jq -r --arg p "$provider" 'first(.data.endpoints[] | select(.provider_name == $p)) |
					"\((.pricing.prompt | tonumber) * 1e6),\((.pricing.completion | tonumber) * 1e6)"' || true)
		fi
		cost="unknown: set $key=\"<in>,<out>\""
		if [[ -n $price ]]; then
			cost=$(awk -v i="$tokens_in" -v o="$tokens_out" -v p="$price" \
				'BEGIN { split(p, r, ","); printf "$%.2f (at $%s / $%s per million)", (i * r[1] + o * r[2]) / 1e6, r[1], r[2] }')
		fi
		printf '%s: served by %s, %s runs, tokens in / out %s / %s, cost %s\n' \
			"$model" "${provider:-unknown}" "$runs" "$tokens_in" "$tokens_out" "$cost"
	done
