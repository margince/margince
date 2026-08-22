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

# lexLine is the ONE pass over a line, and everything else here reads its
# answers. It sets three:
#
#   CMT    where a real line comment begins, 0 for none
#   BLANK  the line with every string literal's CONTENTS replaced by spaces
#   RAW    whether the NEXT line begins inside a backtick string
#
# One function and not three, because scanLine and blankStrings used to be two
# hand-written quote scanners over the same grammar — two implementations of one
# reading, which is the defect this whole file exists to remove, reproduced
# inside the fix for it. They disagreed: one learned that a backslash is literal
# inside a Go raw string and the other did not, and only one tracked `${…}`.
# Now there is one lexer and the disagreement cannot exist.
#
# Cross-line state is carried because both of the constructs that need it span
# lines. A Go raw string does, so a per-line scanner reads the CLOSING backtick
# as an OPENING quote and swallows a real trailing comment as string content. A
# TypeScript `${…}` does too, and its contents are EXECUTABLE — blanking them
# hid the arithmetic the money gate exists to find.
#
# INTERP is the brace depth inside a template interpolation, > 0 while the
# scanner is in the executable part of one. It is Go-blind on purpose: Go's
# backtick string is RAW and never interpolates, so `${amountMinor / 100}`
# written inside one is prose, and treating it as code reported a comment
# describing the old bug as the bug.
function lexLine(s,   i, ch, quote, prev, interp, tmpl, out) {
  RAWIN = RAW
  CMT = 0; CMTKIND = ""
  quote = RAW ? "`" : ""
  interp = INTERP
  # An interpolation carried in from an earlier line is BY DEFINITION inside a
  # template, and `tmpl` is a local that does not survive the line. Without
  # this, a `${…}` spanning lines never handed the scanner back to its string:
  # the rest of the template read as code, its closing backtick opened a NEW
  # string, and five real files in frontend/src ran off the end still inside
  # one.
  tmpl = (quote == "`") || interp > 0
  out = ""
  for (i = 1; i <= length(s); i++) {
    ch = substr(s, i, 1)

    # Inside the executable half of a template interpolation, this is CODE:
    # comments count, strings count, and the closing brace hands the scanner
    # back to the string it came from.
    if (interp > 0 && quote == "") {
      if (ch == "{") { interp++; out = out ch; continue }
      if (ch == "}") {
        interp--
        out = out ch
        if (interp == 0 && tmpl) quote = "`"
        continue
      }
      if (ch == "\"" || ch == "\x27") { quote = ch; out = out ch; continue }
      if (ch == "/" && prev != "\\" && substr(s, i + 1, 1) == "/") { CMT = i; CMTKIND = "//"; break }
      if (ch == "/" && prev != "\\" && substr(s, i + 1, 1) == "*") { CMT = i; CMTKIND = "/*"; break }
      out = out ch
      prev = ch
      continue
    }

    if (quote != "") {
      # A backslash escapes inside a quote — except inside a GO raw string,
      # where it is an ordinary character. The two languages genuinely
      # disagree: TypeScript's `a\`b` escapes the inner backtick, Go's `a\`
      # is a complete two-character raw string. Treating Go's as an escape
      # eats the closing backtick, and with the state carried across lines
      # that does not merely mis-read one line — it leaves the scanner inside
      # a string for the rest of the FILE, which is the failure direction
      # that costs a gate everything.
      if (ch == "\\" && !(quote == "`" && FILENAME ~ /\.go$/)) { out = out "  "; i++; continue }
      if (ch == quote) { quote = ""; out = out ch; continue }
      # `${…}` opens the executable half — in TypeScript only.
      if (quote == "`" && !(FILENAME ~ /\.go$/) && ch == "$" && substr(s, i + 1, 1) == "{") {
        quote = ""; interp = 1; tmpl = 1
        out = out "${"; i++
        continue
      }
      out = out " "
      continue
    }

    if (ch == "\"" || ch == "\x27" || ch == "`") {
      quote = ch
      tmpl = (ch == "`")
      out = out ch
      continue
    }
    # An ESCAPED slash is not a comment opener. `u.replace(/^https?:\/\//,
    # "")` is a TypeScript regex literal, and reading its `\/\/` as a comment
    # truncated the rest of the line — including, on the line that found this,
    # a real `amountMinor / 100` after it. Regex literals are not tracked as a
    # state of their own (that needs to know whether a `/` is division or a
    # literal, which needs a parser); skipping an escaped slash covers the
    # spelling that actually occurs.
    #
    # There is no `prev != ":"` guard. It predated the quote scanning and was
    # meant to spare `https://`, but a scheme only ever appears inside a STRING
    # and the quote state already spares every one of the 1797 in this tree.
    # Outside a string `://` does not occur in either language, so the guard
    # could not fire — and where it could, on a `case x: //note`, it was wrong.
    if (ch == "/" && prev != "\\" && substr(s, i + 1, 1) == "/") { CMT = i; CMTKIND = "//"; break }
    if (ch == "/" && prev != "\\" && substr(s, i + 1, 1) == "*") { CMT = i; CMTKIND = "/*"; break }
    out = out ch
    prev = ch
  }
  # Everything from a comment onward is not code, so it is not blanked either —
  # the caller decides whether to keep it, and CMT tells it where.
  if (CMT > 0) out = out substr(s, CMT)
  BLANK = out
  # A comment cannot begin inside a string or mid-interpolation, so reaching
  # one closes both.
  if (CMT > 0) { RAW = 0; INTERP = 0 } else { RAW = (quote == "`"); INTERP = interp }
}

# commentAt answers only the first half, for a caller holding one line in hand
# rather than walking a file — including a caller re-scanning an already
# stripped copy of the CURRENT line, which is why it resumes from the state the
# line began in. It leaves the carried state exactly as it found it.
function commentAt(s,   keepRaw, keepIn, keepInterp, keepCmt, keepBlank, at) {
  keepRaw = RAW; keepIn = RAWIN; keepInterp = INTERP; keepCmt = CMT; keepBlank = BLANK
  RAW = RAWIN
  lexLine(s)
  at = CMT
  RAW = keepRaw; RAWIN = keepIn; INTERP = keepInterp; CMT = keepCmt; BLANK = keepBlank
  return at
}

# blankStrings returns the CURRENT line with every string literal's contents
# replaced by spaces, the code around them and the length both intact. It reads
# what codeOf's lexLine already computed rather than scanning again — a second
# scanner over the same grammar is how the two came to disagree in the first
# place. Call it after codeOf, and only for the line codeOf just judged.
function blankStrings() {
  return CODEBLANK
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
# It sets two globals rather than one, because both callers want the same line
# read two ways and neither should re-lex it: CODE is the comments-removed line
# that one-spelling scans (its arms are ABOUT string literals, so it must keep
# their contents), and CODEBLANK is the same line with those contents blanked,
# which money-scale needs or a comment describing the defect reads as the
# defect.
#
# The bound on the splice loop is a hundred inline block comments on one line.
# It is not a real limit; it is there so a scanner bug cannot hang a merge gate.
function codeOf(s,   rest, entry, guard) {
  CODE = ""; CODEBLANK = ""
  if (INBLOCK) {
    if (match(s, /\*\//)) { INBLOCK = 0; s = substr(s, RSTART + RLENGTH) } else return ""
  }
  entry = RAW
  CODE = s
  for (guard = 0; guard < 100; guard++) {
    RAW = entry
    lexLine(CODE)
    if (CMT == 0) { CODEBLANK = BLANK; return CODE }
    if (CMTKIND == "//") {
      CODEBLANK = substr(BLANK, 1, CMT - 1)
      CODE = substr(CODE, 1, CMT - 1)
      return CODE
    }
    rest = substr(CODE, CMT)
    # A `*/` inside a block comment is its terminator; there are no strings in
    # comment prose to confuse it with.
    if (match(rest, /\*\//)) { CODE = substr(CODE, 1, CMT - 1) substr(rest, RSTART + RLENGTH); continue }
    INBLOCK = 1
    CODEBLANK = substr(BLANK, 1, CMT - 1)
    CODE = substr(CODE, 1, CMT - 1)
    return CODE
  }
  CODEBLANK = BLANK
  return CODE
}

# UNCLOSED is the line a strip pass emits when it is about to leave a file with
# a string or a block comment still open. That state means the scanner lost
# track partway through and read the rest of the file as something it is not —
# which is a gate reporting OK over code it never looked at, the one failure a
# gate must not have.
#
# It is an assertion rather than a residue paragraph because it is CHEAP and it
# is CHECKABLE: measured over this tree, no scanned file ends in either state
# (0 of 3763 Go, 0 of 926 TypeScript once the *.test.ts the gates already skip
# are set aside). So the one construct still known to desync the scanner — a
# backtick inside a TypeScript regex literal, which opens a template literal
# the language never closes — stops being a silent hole and becomes a failure
# that names the file.
#
# Call closeFile() at FNR == 1, BEFORE resetting the state, and again in END.
function closeFile() {
  if (SCANNED != "" && (RAW || INBLOCK)) print "commentscan-unclosed:" SCANNED
  SCANNED = FILENAME
}

# waived: the marker appears in a REAL comment on this line.
function waived(s, marker,   at) {
  at = commentAt(s)
  return at > 0 && index(substr(s, at), marker) > 0
}
