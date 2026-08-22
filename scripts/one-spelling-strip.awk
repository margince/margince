# The one-spelling gate's own strip pass.
#
# Loaded after scripts/lib-commentscan.awk, which supplies commentAt() and
# waived() — the reading of "where does a comment start" that this gate shares
# with money-scale-strip.awk.
#
# It does NOT call blankStrings(), and that is the difference between the two
# callers. All three of this gate's arms are ABOUT string literals — "23505",
# "constraint_violated", the ISO-4217 regexp — so blanking string contents
# would blind it completely, where money-scale must blank them or a line
# describing the defect reads as the defect.

FNR == 1 { inblock = 0; RAW = 0 }
{
  c = $0
  # The waiver counts only where a waiver can be WRITTEN — in a real comment.
  # Taking the marker anywhere on the line let a defect be silenced by a marker
  # inside a string, which nobody wrote as one.
  if (inblock) { if (match(c, /\*\//)) { inblock = 0; c = substr(c, RSTART + RLENGTH) } else next }
  # ONE scan per line of code, and it must run BEFORE any early `next` — the
  # waived one included, or the raw-string state desyncs on exactly the lines
  # somebody deliberately excluded, and every line after them is read against
  # the wrong state. A block-comment continuation is the one line it skips,
  # because nothing there is code and no string opens in prose.
  scanLine(c)
  if (waived(c, waiver)) next
  t = c; sub(/^[[:space:]]+/, "", t)
  if (t ~ /^(\/\/|\*)/) next
  while (match(c, /\/\*[^*]*\*+([^\/*][^*]*\*+)*\//)) { c = substr(c, 1, RSTART - 1) substr(c, RSTART + RLENGTH) }
  if (match(c, /\/\*/)) { inblock = 1; c = substr(c, 1, RSTART - 1) }
  # The shared scanner decides where a trailing comment starts, so a `//` inside
  # a string no longer truncates the line — 167 lines in this tree carry one,
  # mostly //nolint: directives and URL paths.
  at = commentAt(c)
  if (at > 0) c = substr(c, 1, at - 1)
  print FILENAME ":" FNR ":" c
}
