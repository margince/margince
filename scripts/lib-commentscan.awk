# The ONE reading of "where does a comment start, and what is inside a string",
# shared by the gate scripts that need it.
#
# An awk source file rather than a shell snippet: both callers splice it in with
# `awk -f scripts/lib-commentscan.awk -f <their own program>`, which composes
# without any quoting between the two.
#
# It exists because the two gates had a copy each and the copies were NOT equal.
# The money-scale copy had learned that a `//` inside a STRING is not a comment,
# and that a waiver marker inside one is forged; the one-spelling copy still
# took the marker anywhere on the line, so a defect could be waived with a
# marker nobody wrote as one:
#
#   func probe(c string) (bool, string) { return c == "23505", "one-spelling-exempt: fake" }
#
# That bypass was live. Two writers of one invariant either share a helper or
# say why they do not, and these two had no reason.
#
# What this does NOT decide is what to do with the string CONTENTS. one-spelling
# looks for string literals — "23505", "constraint_violated", the ISO-4217
# regexp — so it must keep them, while money-scale must blank them or a line
# describing the defect reads as the defect. That difference is real and stays
# with each caller.

# scanLine is the one pass over a line, and it answers two things at once
# because they are the same question asked twice: CMT is where a real line
# comment begins (0 for none), and RAW says whether the NEXT line begins
# inside a Go raw string.
#
# The second answer is why this is a function and not a regex. A Go raw
# string spans lines, and a per-line scanner that does not carry that state
# reads the CLOSING backtick as an OPENING quote — so everything after it on
# that line, including a real trailing comment, is taken for string content:
#
#   const q = `SELECT 1
#   FROM person` // the store maps "23505" via storekit, never here
#
# which put a truthful comment into the corpus as CODE, and separately made a
# waiver on such a line unreadable. Both directions were live.
#
# Scanning quote by quote rather than matching a pattern, because the pattern
# cannot tell a comment from its lookalike either: a line holding
# `return x / 100, "// money-scale-exempt: fake"` has a real arithmetic defect
# and a fake marker, and a regex reading left to right waives the whole line
# along with the defect on it.
#
# Call it once per line, before reading CMT or calling blankStrings. RAWIN is
# the state the line STARTED in, which blankStrings needs and RAW no longer
# holds once the line is done.
function scanLine(s,   i, ch, quote, prev) {
  RAWIN = RAW
  quote = RAW ? "`" : ""
  CMT = 0; CMTKIND = ""
  for (i = 1; i <= length(s); i++) {
    ch = substr(s, i, 1)
    if (quote != "") {
      # A backslash escapes inside a quote — except inside a GO raw string,
      # where it is an ordinary character. The two languages genuinely
      # disagree: TypeScript's `a\`b` escapes the inner backtick, Go's `a\`
      # is a complete two-character raw string. Treating Go's as an escape
      # eats the closing backtick, and with the state now carried across
      # lines that does not merely mis-read one line — it leaves the scanner
      # inside a string for the rest of the FILE, which is the failure
      # direction that costs a gate everything.
      if (ch == "\\" && !(quote == "`" && FILENAME ~ /\.go$/)) { i++; continue }
      if (ch == quote) quote = ""
      continue
    }
    if (ch == "\"" || ch == "\x27" || ch == "`") { quote = ch; continue }
    # An ESCAPED slash is not a comment opener. `u.replace(/^https?:\/\//,
    # "")` is a TypeScript regex literal, and reading its `\/\/` as a
    # comment truncated the rest of the line — including, on the line that
    # found this, a real `amountMinor / 100` after it. Regex literals are
    # not tracked as a state of their own (that needs to know whether a `/`
    # is division or a literal, which needs a parser); skipping an escaped
    # slash covers the spelling that actually occurs.
    if (ch == "/" && prev != "\\" && prev != ":" && substr(s, i + 1, 1) == "/") { CMT = i; CMTKIND = "//"; break }
    if (ch == "/" && prev != "\\" && substr(s, i + 1, 1) == "*") { CMT = i; CMTKIND = "/*"; break }
    prev = ch
  }
  # A comment cannot begin inside a string, so reaching one means no quote is
  # open and none carries to the next line.
  RAW = (quote == "`")
}

# commentAt answers only the first half, for a caller holding one line in hand
# rather than walking a file — including a caller re-scanning an already
# stripped copy of the CURRENT line, which is why it resumes from RAWIN rather
# than from cold. It leaves the carried state exactly as it found it.
function commentAt(s,   keepRaw, keepIn, keepCmt, at) {
  keepRaw = RAW; keepIn = RAWIN; keepCmt = CMT
  RAW = RAWIN
  scanLine(s)
  at = CMT
  RAW = keepRaw; RAWIN = keepIn; CMT = keepCmt
  return at
}

# blankStrings replaces the inside of every string literal with spaces,
# keeping the line length and the code around it.
# It resumes from RAWIN, so a line in the middle of a Go raw string is blanked
# as the string content it is.
function blankStrings(s,   i, ch, quote, out, braces) {
  out = ""
  quote = RAWIN ? "`" : ""
  for (i = 1; i <= length(s); i++) {
    ch = substr(s, i, 1)
    if (quote != "") {
      # Same language-specific rule as scanLine: a backslash is literal inside
      # a Go raw string, and eating the closing backtick blanks the rest of the
      # line as string content — which hid `amountMinor/100` after a
      # `strings.TrimPrefix(path, \`\\\`)`.
      if (ch == "\\" && !(quote == "`" && FILENAME ~ /\.go$/)) { out = out "  "; i++; continue }
      if (ch == quote) { quote = ""; out = out ch; continue }
      # `${…}` inside a template literal is EXECUTABLE, not string content,
      # so it is kept. Blanking it hid `${amountMinor / 100}` entirely.
      if (quote == "`" && ch == "$" && substr(s, i + 1, 1) == "{") {
        # `braces` is a LOCAL. It was `depth` and it was not in this
        # function's parameter list, so it was a global — and the money-scale
        # strip pass uses a global `depth` as its own bracket accumulator. A
        # single closed `${…}` on a continuation line reset that accumulator
        # to zero mid-statement, flushing the buffer and splitting a wrapped
        # `major * 100` in half so neither half matched. awk has no other way
        # to declare a local, which is exactly how the collision happened.
        braces = 1; out = out "${"; i += 2
        while (i <= length(s) && braces > 0) {
          ch = substr(s, i, 1)
          if (ch == "{") braces++
          if (ch == "}") braces--
          out = out ch
          i++
        }
        i--
        continue
      }
      out = out " "
      continue
    }
    if (ch == "\"" || ch == "\x27" || ch == "`") { quote = ch; out = out ch; continue }
    out = out ch
  }
  return out
}

# codeOf returns the CODE on a line — every comment removed, string literals
# left intact — and carries both cross-line states: INBLOCK for a `/* */` that
# has not closed, RAW for a Go raw string that has not closed. An all-comment
# line comes back as "".
#
# It is HERE rather than in each strip pass because both passes had written the
# same nine lines out and neither had noticed that they detected a block comment
# with a bare `match(c, /\/\*/)` on the raw line — which a string is enough to
# forge:
#
#   var globPattern = "**/*.go"
#
# opened a block comment that never closed, and every arm of the gate went blind
# from that line to the end of the file. The scanner that can tell the two apart
# shipped beside this and was not being used for the one job it was written for.
#
# The bound on the splice loop is a hundred inline block comments on one line.
# It is not a real limit; it is there so a scanner bug cannot hang a merge gate.
function codeOf(s,   out, rest, entry, guard) {
  if (INBLOCK) {
    if (match(s, /\*\//)) { INBLOCK = 0; s = substr(s, RSTART + RLENGTH) } else return ""
  }
  entry = RAW
  out = s
  for (guard = 0; guard < 100; guard++) {
    RAW = entry
    scanLine(out)
    if (CMT == 0) return out
    if (CMTKIND == "//") return substr(out, 1, CMT - 1)
    rest = substr(out, CMT)
    # A `*/` inside a block comment is its terminator; there are no strings in
    # comment prose to confuse it with.
    if (match(rest, /\*\//)) { out = substr(out, 1, CMT - 1) substr(rest, RSTART + RLENGTH); continue }
    INBLOCK = 1
    return substr(out, 1, CMT - 1)
  }
  return out
}

# waived: the marker appears in a REAL comment on this line.
function waived(s, marker,   at) {
  at = commentAt(s)
  return at > 0 && index(substr(s, at), marker) > 0
}
