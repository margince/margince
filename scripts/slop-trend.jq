# slop-trend.jq turns two runs of `craft slop --json` and `craft stats --json`
# into one Markdown table. $now/$was hold the slop reports, $snow/$swas the
# comment stats; scripts/slop-trend.sh passes them.

def round3: if . == null then null else . * 1000 | round / 1000 end;
def change($a; $b): if $a == null or $b == null then "" else ($b - $a | round3 | tostring | if startswith("-") then . else "+" + . end) end;
def row($label; $a; $b): "| \($label) | \($a // "" | tostring) | \($b // "" | tostring) | \(change($a; $b)) |";
def prod($r; $lang): [$r.languages[] | select(.name == $lang) | .scopes[] | select(.name == "production")][0];
def areas($r): [$r.languages[] | .production_areas[]? | {key: .name, value: .erosion}] | from_entries;

(($ARGS.named.first // "") == "yes") as $first |
($now[0]) as $n | (if $first then {languages: []} else $was[0] end) as $w |
($snow[0].metrics) as $sn | (if $first then {} else $swas[0].metrics end) as $sw |
[
  "## Slop trend",
  "",
  (if $first then "No earlier run to compare with, so only this run's numbers show.\n" else empty end),
  "The scores show a trend. The paper they come from found they do not predict a failing test.",
  "",
  "| Measure | Last run | This run | Change |",
  "|---|---|---|---|",
  ( ["go", "typescript"][] as $lang
    | prod($n; $lang) as $pn | (prod($w; $lang) // {}) as $pw
    | select($pn != null)
    | row("\($lang) verbosity"; $pw.verbosity | round3; $pn.verbosity | round3),
      row("\($lang) erosion"; $pw.erosion | round3; $pn.erosion | round3),
      row("\($lang) cognitive erosion"; $pw.cognitive_erosion | round3; $pn.cognitive_erosion | round3),
      row("\($lang) share of functions over cognitive 15"; $pw.cog_over_cutoff_share | round3; $pn.cog_over_cutoff_share | round3)
  ),
  ( $sn | keys[] as $k | row("comments: \($k)"; $sw[$k]; $sn[$k]) ),
  "",
  (if $first then "### Production areas with the most erosion" else "### Production areas whose erosion moved most" end),
  "",
  "| Area | Last run | This run | Change |",
  "|---|---|---|---|",
  ( areas($n) as $an | areas($w) as $aw
    | if $first then
        [$an | to_entries[] | {area: .key, now: (.value | round3)}] | sort_by(-.now) | .[:10][]
        | row(.area; null; .now)
      else
        [$an | keys[] | select($aw[.] != null) | {area: ., was: ($aw[.] | round3), now: ($an[.] | round3)}]
        | map(. + {delta: ((.now - .was) | fabs)}) | sort_by(-.delta) | .[:10][]
        | row(.area; .was; .now)
      end )
] | .[]
