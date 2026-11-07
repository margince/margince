#!/usr/bin/env bash
# The first screen of `make help`: the everyday commands, in groups, each with
# the first clause of its own description. The description is read from the
# Makefile that defines the target, so it has one source; this file holds only
# which names a newcomer needs and in what order.
#
# A listed name with no description in either Makefile fails the run, so a
# renamed target cannot drop off this screen unseen. `make help-all` lists every
# target, the gates CI runs among them.
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

groups=(
  "Start|install dev dev-stop dev-logs seed-dev check-config"
  "Check before ready|check check-go check-gates check-fe test-it"
  "Frontend|fe-unit fe-lint fe-typecheck storybook"
  "Database|db-up migrate migrate-create psql"
  "Generate|gen config-gen drift"
  "Release|sbom-gate secret-scan vuln"
)

# One description per target. A root target's text is its `## name — …`
# comment block, which may name several targets ("## a / b — …") and run over
# several lines; a backend target's text follows `## ` on its rule line.
describe() {
  awk -v want="$1" '
    function emit(text) {
      sub(/(\. |: | \(| — |; |, ).*/, "", text)
      sub(/\.$/, "", text)
      if (length(text) > 64) { text = substr(text, 1, 64); sub(/ [^ ]*$/, " …", text) }
      print text
      found = 1
      exit
    }
    FILENAME ~ /backend\/Makefile$/ && $0 ~ ("^" want ":.*## ") {
      text = $0
      sub(/^[^#]*## /, "", text)
      emit(text)
    }
    collecting && /^## ./ { text = text " " substr($0, 4); next }
    collecting { emit(text) }
    /^## [A-Za-z0-9_\/ -]+ — / {
      head = $0
      sub(/^## /, "", head)
      sub(/ — .*/, "", head)
      n = split(head, names, " / ")
      for (i = 1; i <= n; i++) {
        if (names[i] == want) {
          text = substr($0, index($0, " — ") + length(" — "))
          collecting = 1
          next
        }
      }
    }
    END { if (!found && collecting) emit(text) }
  ' "$root/Makefile" "$root/backend/Makefile"
}

echo "Margince: everyday make commands"
missing=0
for group in "${groups[@]}"; do
  echo ""
  echo "${group%%|*}:"
  for target in ${group#*|}; do
    text="$(describe "$target")"
    if [[ -z "$text" ]]; then
      echo "make-help: \`$target\` is listed here but neither Makefile describes it; rename or remove it in scripts/make-help.sh" >&2
      missing=1
      continue
    fi
    printf '  %-16s %s\n' "$target" "$text"
  done
done
echo ""
echo "Every target, including the gates CI runs: make help-all"
exit "$missing"
