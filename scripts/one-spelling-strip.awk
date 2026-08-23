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

FNR == 1 { closeFile() }
END { closeFile() }
{
  c = $0
  # ONE call per line, and it must come BEFORE any early `next` — the waived
  # one included, or the cross-line states desync on exactly the lines somebody
  # deliberately excluded, and every line after them is read against the wrong
  # one. codeOf carries both and hands back the code with the comments gone.
  code = codeOf(c)

  # The waiver counts only where a waiver can be WRITTEN — in a real comment.
  # Taking the marker anywhere on the line let a defect be silenced by a marker
  # inside a string, which nobody wrote as one.
  if (waived(c, waiver)) next
  c = code
  t = c; sub(/^[[:space:]]+/, "", t)
  if (t == "") next
  print FILENAME ":" FNR ":" c
}
