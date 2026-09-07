#!/usr/bin/env bash
# Jurisdiction-isolation gate. A
# fitness function for the pack boundary: country-specific regulatory
# identifiers must live in the jurisdiction seam, never in core. Core code
# that hard-codes a country string cannot be reused across jurisdictions and
# leaks one market's rules into everyone's build.
#
# This repo's seam is internal/shared/ports/jurisdiction (the Tier-0 port,
# aliasing the published pkg/extension/jurisdiction contract); the packs
# themselves live OUTSIDE core as stable-tier extensions (extensions/de, the
# ADR-0069 pilot), which this gate does not scan — an extension is
# jurisdiction-specific by design. Everything else under internal/ is core and
# must stay country-neutral. Generated contract code (*_gen.go / *.gen.go) and
# tests (which legitimately exercise pack behavior) are out of scope — this
# gate guards hand-written core source.
#
# CORE IS NOT ONLY GO. This scanned --include='*.go' and nothing else, while its
# own header said "core code", so a jurisdiction string in a core migration was
# invisible. A gate that reads a smaller tree than its subject reports PASS over
# the part it cannot see, which is the one way a census must not fail.
# Migrations are scanned too, with SQL's own comment marker understood.
#
# KNOWN LEAK, NOT CAUGHT BY THE PATTERNS BELOW: the communication basis
# vocabulary carries 'vn_subject_agreement' and 'existing_customer_exception' —
# in core Go (internal/shared/ports/commsauthz/decision.go) and in two CHECK
# constraints of shipped migration 1788407500. Core must stay country-neutral,
# so that is a real violation. Whether the vocabulary should carry per-pack
# values is a product decision about the engine and not one a grep settles;
# it is filed as issue #4733, not argued here.

set -euo pipefail
cd "$(dirname "$0")/.."

# The two core trees this gate reads. Migrations are core: an installation's
# schema is as much the product as the code that queries it, and a CHECK
# constraint naming one market's rule is exactly the leak this gate exists to
# refuse.
scan="backend/internal"
scan_sql="backend/migrations"
seam='/ports/jurisdiction/'
# Anchored to the path segment of a `file:line:content` grep row (…_gen.go:NN),
# not `$` — which would match the END of the content, so generated files never
# got excluded (the whole point of this filter).
generated='(_gen|\.gen)\.go:[0-9]'

# Strip comments from `file:line:content` rows: drop whole-line comments (//,
# /*, * continuation) AND remove a trailing ` // …` line-comment from a code
# line, then the caller re-matches. This makes the gate read CODE only — a
# statute named in a comment (the header's promise) never fails the build, while
# the same token in a literal/identifier still does. The space before // avoids
# eating a `scheme://…` inside a string.
#
# SQL's marker is `--`, and it is handled here rather than in a second function:
# the two languages differ only in the token, and two strippers would be two
# answers to one question — the SQL one would be the copy that goes stale.
#
# QUOTE-AWARE, and it has to be. An unanchored `sub` on the first `-- ` blanks
# the rest of the line, including a jurisdiction string in the CODE that follows
# — and `-- ` inside a single-quoted SQL string is ordinary prose, not a
# contrivance: migrations carry sentences in COMMENT ON, in DEFAULTs and in
# CHECK error text. A stripper that ate them would hide the very leak this gate
# widened to find, one line below the header condemning exactly that. The Go
# marker is scanned the same way for the same reason: `"http://x // ZUGFeRD"`
# defeated the old unanchored strip too, which this fixes rather than inherits.
#
# The scan walks the line once, toggling on quote characters (a backslash
# escapes the next one), and cuts at the first marker standing OUTSIDE a quoted
# run.
#
# WHAT IT STILL CANNOT SEE, stated rather than implied: it is line-oriented, so
# a construct spanning lines is beyond it — a Go raw string or a PostgreSQL
# dollar-quoted body whose continuation line begins with a marker reads as a
# comment, and a /* … */ block is matched by its opening line only. Each is a
# false PASS for a token hidden inside one. Closing them needs a parser per
# language, which is a different gate from this one; the shapes that actually
# occur in this tree — a marker inside a single-line literal, in either
# language, escaped or not — are the ones asserted in the test beside it.
strip_comments() {
  awk '{
    if (match($0, /^[^:]+:[0-9]+:/)) { prefix=substr($0,1,RLENGTH); c=substr($0,RLENGTH+1) }
    else { prefix=""; c=$0 }
    t=c; sub(/^[[:space:]]+/,"",t)
    if (t ~ /^(\/\/|\/\*|\*|--)/) next
    # Walk once; q tracks the quote character we are inside, empty when outside.
    q=""; cut=0
    for (i = 1; i <= length(c); i++) {
      ch = substr(c, i, 1)
      # A backslash escapes the next character INSIDE a quoted run, so `\"`
      # does not close it. Without this, `"a \" // ZUGFeRD"` reads as closed at
      # the escaped quote and the leak after it is cut away as a comment.
      # Backticks are Go raw strings, where a backslash escapes nothing.
      if (q != "" && q != "`" && ch == "\\") { i++; continue }
      if (q != "") { if (ch == q) q=""; continue }
      if (ch == "\"" || ch == "'\''" || ch == "`") { q=ch; continue }
      two = substr(c, i, 2)
      if (two == "//" || two == "--") { cut=i; break }
    }
    if (cut > 0) c = substr(c, 1, cut - 1)
    print prefix c
  }'
}

# Named regulatory identifiers. Case-SENSITIVE with word boundaries: these are
# proper nouns with fixed spellings, and a case-insensitive match false-fires
# on incidental substrings (e.g. DATEV inside "UpdateVoice").
named='\b(XRechnung|ZUGFeRD|DATEV|GoBD|eIDAS|Impressum)\b'
hits="$( { grep -rnE "$named" "$scan" --include='*.go' 2>/dev/null; \
             grep -rnE "$named" "$scan_sql" --include='*.sql' 2>/dev/null; } \
  | grep -vE "$seam" | grep -vE "$generated" | grep -v '_test.go' \
  | strip_comments | grep -E "$named" || true)"

# Conservative ISO-3166: a quoted UPPER-case alpha-2 only when it shares a line
# with a country-ish keyword, so incidental two-letter strings (HTTP verbs,
# enum codes) do not false-fire. The alpha-2 must be upper-case (no -i).
#
# BOTH QUOTE STYLES, because the corpus now includes SQL and SQL quotes strings
# with a single quote. Matching only Go's double quote would have left
# `CHECK (jurisdiction <> 'DE')` passing in a tree this gate had just claimed to
# read — the widening would have been the announcement of a reach it did not
# have, which is the failure this whole change is about.
kw='[Cc]ountry|[Jj]urisdiction|[Ii][Ss][Oo][_-]?3166'
q="[\"']"
iso="($kw).*${q}[A-Z]{2}${q}|${q}[A-Z]{2}${q}.*($kw)"
iso_hits="$( { grep -rnE "$iso" "$scan" --include='*.go' 2>/dev/null; \
                 grep -rnE "$iso" "$scan_sql" --include='*.sql' 2>/dev/null; } \
  | grep -vE "$seam" | grep -vE "$generated" | grep -v '_test.go' \
  | strip_comments | grep -E "$iso" || true)"

if [[ -n "$hits" ]] || [[ -n "$iso_hits" ]]; then
  echo "FAIL: jurisdiction-specific strings in core (move to the owning jurisdiction pack under extensions/<jurisdiction>):"
  [[ -n "$hits" ]] && echo "$hits"
  [[ -n "$iso_hits" ]] && echo "$iso_hits"
  exit 1
fi

echo "OK: no jurisdiction strings in core"
