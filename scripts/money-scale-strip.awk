# The money-scale gate's own strip pass: comments removed and continuation lines
# joined, so a wrapped expression is judged whole.
#
# Loaded after scripts/lib-commentscan.awk, which supplies commentAt(),
# blankStrings() and waived() — the reading of "where does a comment start"
# that this gate shares with check-one-spelling.awk.

function opens(s,   n, t) { t = s; n = gsub(/[([]/, "", t); return n }
function closes(s,  n, t) { t = s; n = gsub(/[)\]]/, "", t); return n }

# `bufFile` and not FILENAME: flush() also runs at FNR == 1, where awk has
# already moved FILENAME on to the new file while buf still holds the tail of
# the old one. The buffer must be reported under the file it came from, or an
# identifier in one file pairs with a power of ten in the next.
function flush() { if (buf != "") print bufFile ":" start ":" buf; buf = ""; depth = 0; lines = 0 }
FNR == 1 { closeFile(); flush() }
{
  c = $0
  # ONE call per line, and it must come BEFORE any early `next` — the waived
  # one included, or the cross-line states desync on exactly the lines somebody
  # deliberately excluded, and every line after them is read against the wrong
  # one. codeOf carries both and hands back the code with the comments gone.
  code = codeOf(c)

  # The waiver counts only where a waiver can be WRITTEN — in a comment. A
  # line carrying the marker inside a string literal was skipping the whole
  # line, which let any code on it bypass the gate under a marker its author
  # never wrote as one.
  if (waived(c, waiver)) { flush(); next }
  c = code
  t = c; sub(/^[[:space:]]+/, "", t)
  # A line that was ENTIRELY comment is not a statement boundary, but a blank
  # one is. codeOf empties both, so `$0` is what tells them apart.
  if (t == "" && $0 ~ /[^[:space:]]/) next
  # And the contents of a STRING are not code. A line mentioning the shape
  # in prose — "see amountMinor / 100 in the old code" — was reported as
  # the arithmetic it describes. The same quote scanner that finds the
  # comment blanks the literals, so the identifier and the power have to be
  # in the CODE to pair.
  c = blankStrings()
  if (t == "") { flush(); next }
  if (buf == "") { start = FNR; bufFile = FILENAME }
  buf = buf " " c
  lines++
  depth += opens(c) - closes(c)
  # An expression may also be broken after a trailing operator with no
  # bracket open — `amountMinor :=` on one line and `major * 100` on the
  # next — so a line ENDING in one keeps the statement open. Without this
  # the gate flushed before the arithmetic arrived and saw two halves,
  # neither of them a finding.
  # Only an operator that CANNOT end a statement continues one. A trailing
  # COMMA ends a perfectly good line in a struct literal, and treating it as a
  # continuation joined unrelated members.
  #
  # A trailing COLON continues, and the distinction from the comma is the whole
  # rule rather than an accident: a member ending in a comma is complete
  # (`valueMinor: 1,`), while a line ending ON the colon has its value on the
  # next one — which is how biome wraps a long object property, `amountMinor:`
  # then `major * 100`. Collapsing the two back together, which reads like a
  # simplification, loses the write direction this gate exists for. Braces are
  # left out of the depth above for the same reason: they open a BLOCK, and
  # counting them swallowed whole function bodies.
  trailing = c
  sub(/[[:space:]]+$/, "", trailing)
  # `%` continues: a remainder is how the cents half of an amount is taken. A
  # postfix `++`/`--` ENDS a statement, so it is excluded rather than caught by
  # the bare `+`/`-` alternatives.
  if (trailing ~ /(\+\+|--)$/) { if (depth <= 0 || lines >= 6) flush(); next }
  # A `case x:` or a bare label ends with a colon and does NOT continue — its
  # body is a separate statement, and joining it paired a `case valueMinor:`
  # with an unrelated `ratio * 100` in the arm below.
  # `[^A-Za-z0-9_]` and not `\b`: POSIX ERE has no word boundary, and awk reads
  # `\b` as a backspace — the guard matched nothing and the case label went on
  # joining the arm below it.
  # A `case`/`default` label ends its statement: its body is a separate one, and
  # joining them paired a `case valueMinor:` with an unrelated `ratio * 100` in
  # the arm below.
  #
  # Two lines can be a label — `case pick(` and its closing `valueMinor):` — so
  # both are boundaries, and a line ending in `):` or `]:` is the second of
  # them. Stateless on purpose: carrying "am I still in a label" across lines
  # meant a label with code on it (`case x: doThing()`) never cleared the flag,
  # and every later line in the FILE was flushed on its own — so a wrapped
  # expression anywhere below the switch stopped being judged at all. A state
  # that can get stuck open is worse here than a rule that occasionally splits
  # one statement too many.
  #
  # Two residues, both false NEGATIVES and both stated rather than hidden.
  #
  # An object key literally named `default` or `case` reads as a label, so a
  # value wrapped onto the next line under one is not joined. The alternative —
  # brace depth — swallowed whole function bodies when it was tried.
  #
  # A ternary split across the colon is not joined either, in either spelling:
  # a line ending in `)` does not continue a statement (it would join every
  # closing paren in the tree), and a line ending in `):` reads as a wrapped
  # label. Telling a label colon from a ternary colon needs a parser. Both
  # spellings are missed identically before this change and after it, so the
  # rule here is not what loses them.
  trimmed = c; sub(/[[:space:]]+$/, "", trimmed)
  if (t ~ /^(case|default)([^A-Za-z0-9_]|$)/ || trimmed ~ /[)\]]:$/) { flush(); next }
  if (trailing ~ /(=|\+|-|\*|\/|%|:|&&|\|\|)$/ && lines < 6) next
  # Bounded at SIX lines. Four was the first bound and it missed the shape
  # this gate exists for, one line longer: biome wraps
  # `const amountMinor = Math.round(Number(amount) * 100)` across five when
  # the expression is long enough, and the buffer flushed with the name in
  # one half and the power in the other. A `const (` block is thirty lines,
  # so six still refuses to join one — measured on compose/report.go, whose
  # block holds `amount_minor` and a `/ 100.0` thirty lines apart with
  # nothing to do with each other. A blank line ends a statement too.
  if (depth <= 0 || lines >= 6) flush()
}
END { flush(); closeFile() }
