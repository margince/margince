# Strip Go comments, leaving code (and string literals) in place.
#
# Character by character with three states, because every shorter version is
# wrong on the input this gate actually reads: `sed 's|//.*||'` cuts a URL out
# of a string literal, and any regex that tries to spare strings has to know
# about escapes, which regexes do not.
#
# What it must never do is drop CODE — a stripper that swallowed a real call
# would turn this gate green over the violation it exists to catch — so a
# character is only ever dropped while a comment is provably open.
{
  line = $0
  out = ""
  i = 1
  n = length(line)
  while (i <= n) {
    c = substr(line, i, 1)
    if (block) {
      if (c == "*" && substr(line, i + 1, 1) == "/") { block = 0; i += 2; continue }
      i++
      continue
    }
    if (raw) {
      out = out c
      if (c == "`") raw = 0
      i++
      continue
    }
    if (str) {
      out = out c
      if (c == "\\") { out = out substr(line, i + 1, 1); i += 2; continue }
      if (c == quote) str = 0
      i++
      continue
    }
    if (c == "`") { raw = 1; out = out c; i++; continue }
    if (c == "\"" || c == "'") { str = 1; quote = c; out = out c; i++; continue }
    if (c == "/" && substr(line, i + 1, 1) == "/") break
    if (c == "/" && substr(line, i + 1, 1) == "*") { block = 1; i += 2; continue }
    out = out c
    i++
  }
  # A raw string spans lines; an interpreted one cannot, so a `str` still open
  # at end of line is a lexical error in the file rather than state to carry.
  str = 0
  print out
}
