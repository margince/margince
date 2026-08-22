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
# the old one. A statement left buffered at end of file — a line ending in a
# trailing operator — was then reported at a line number in a DIFFERENT file,
# and worse, joined to that file's first line, which can pair an identifier
# from one file with a power of ten from another and invent a finding out of
# two innocent files.
function flush() { if (buf != "") print bufFile ":" start ":" buf; buf = ""; depth = 0; lines = 0 }
FNR == 1 { closeFile(); flush(); INBLOCK = 0; RAW = 0 }
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
  # A line that was ENTIRELY comment is not a statement boundary. codeOf hands
  # back "" for one, and treating that as a blank line flushed the statement
  # underway — so `const amountMinor =`, a `/* note */` line, and `major * 100`
  # were judged as three fragments and none of them matched. A real blank line
  # still ends a statement; `$0` distinguishes the two, because only the
  # comment line had something on it.
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
  # A trailing colon is different and used to be lumped in with the comma on
  # the strength of that same example — wrongly, because the example's members
  # end in a COMMA (`valueMinor: 1,`), not a colon. A line that ends ON the
  # colon has its value on the next line, which is how biome wraps a long
  # object property: `amountMinor:` then `major * 100`. That is the write
  # direction this gate exists for and it was escaping. Re-measured with the
  # example the old comment named, plus a `mode:` / `"fast"` pair over an
  # unrelated `ratio * 100` — neither is a finding, and the real tree stays
  # green. Braces are
  # left out of the depth above for the same reason: they open a BLOCK, and
  # counting them swallowed whole function bodies.
  trailing = c
  sub(/[[:space:]]+$/, "", trailing)
  # `%` belongs here: a remainder is how the cents half of an amount is taken,
  # and `amountMinor %` / `100` split across two lines was the one arithmetic
  # shape in the gate's own vocabulary that its continuation rule could not
  # rejoin. A postfix `++`/`--` does NOT continue a statement — it ENDS one —
  # so it is excluded rather than caught by the bare `+`/`-` alternatives.
  if (trailing ~ /(\+\+|--)$/) { if (depth <= 0 || lines >= 6) flush(); next }
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
