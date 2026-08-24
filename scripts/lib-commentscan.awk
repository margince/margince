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
#   STACK  the lexical contexts still open, for the NEXT line
#
# One function and not three, because scanLine and blankStrings used to be two
# hand-written quote scanners over the same grammar — two implementations of one
# reading, which is the defect this whole file exists to remove, reproduced
# inside the fix for it. They disagreed: one learned that a backslash is literal
# inside a Go raw string and the other had not, and only one tracked `${…}`.
#
# STACK is a string, one character per open context, innermost last:
#
#   q  a double-quoted string      s  a single-quoted string
#   t  a template / Go raw string  i  a brace inside an interpolation
#
# A stack and not a pair of counters, because the contexts NEST and a pair
# cannot say so. `${`${minor}`}` opens a template, an interpolation, a second
# template and a second interpolation; a flat "am I in a template" flag was
# overwritten by the inner one and its contents were read as code, which
# reported a nested string's prose as arithmetic.
#
# Only t and the i's beneath it carry to the next line, because only they
# legally span one. A q or s left open at end of line is a syntax error, and
# carrying it would blind the rest of the FILE over a typo — the one direction
# a scanner must not fail in. TypeScript's backslash-continued string is the
# exception and is honoured.
function top(   n) { n = length(STACK); return n == 0 ? "" : substr(STACK, n, 1) }
function push(c) { STACK = STACK c }
function pop() { STACK = substr(STACK, 1, length(STACK) - 1) }

function lexLine(s,   i, ch, nxt, prev, out, goFile, t) {
  STACKIN = STACK
  CMT = 0; CMTKIND = ""
  out = ""
  goFile = (FILENAME ~ /\.go$/)
  for (i = 1; i <= length(s); i++) {
    ch = substr(s, i, 1)
    nxt = substr(s, i + 1, 1)
    t = top()

    if (t == "q" || t == "s") {
      if (ch == "\\") { out = out "  "; i++; continue }
      if ((t == "q" && ch == "\"") || (t == "s" && ch == "\x27")) { pop(); out = out ch; continue }
      out = out " "
      continue
    }

    if (t == "t") {
      # A backslash escapes inside a template — but NOT inside a Go raw string,
      # where it is an ordinary character. The two languages genuinely disagree:
      # TypeScript's `a\`b` escapes the inner backtick, Go's `a\` is a complete
      # two-character raw string. Treating Go's as an escape eats the closing
      # backtick and leaves the scanner inside a string for the rest of the
      # file.
      if (ch == "\\" && !goFile) { out = out "  "; i++; continue }
      if (ch == "`") { pop(); out = out ch; continue }
      # Go's backtick string is RAW and never interpolates, so `${…}` inside one
      # is prose; reading it as code reported a comment describing the old bug
      # as the bug.
      if (!goFile && ch == "$" && nxt == "{") { push("i"); out = out "${"; i++; continue }
      out = out " "
      continue
    }

    # Code: either the top level, or the executable half of an interpolation.
    if (ch == "\"") { push("q"); out = out ch; continue }
    if (ch == "\x27") { push("s"); out = out ch; continue }
    if (ch == "`") { push("t"); out = out ch; continue }
    if (t == "i" && ch == "{") { push("i"); out = out ch; continue }
    if (t == "i" && ch == "}") { pop(); out = out ch; continue }
    # An ESCAPED slash is not a comment opener. `u.replace(/^https?:\/\//, "")`
    # is a TypeScript regex literal, and reading its `\/\/` as a comment
    # truncated the rest of the line — including, on the line that found this,
    # a real `amountMinor / 100` after it. Regex literals are not tracked as a
    # state of their own (that needs to know whether a `/` is division or a
    # literal, which needs a parser); skipping an escaped slash covers the
    # spelling that actually occurs.
    #
    # There is no `prev != ":"` guard. It predated the quote scanning and was
    # meant to spare `https://`, but a scheme only ever appears inside a STRING
    # and the quote state already spares every one of the 1797 in this tree.
    if (ch == "/" && prev != "\\" && nxt == "/") { CMT = i; CMTKIND = "//"; break }
    if (ch == "/" && prev != "\\" && nxt == "*") { CMT = i; CMTKIND = "/*"; break }
    out = out ch
    prev = ch
  }
  # Everything from a comment onward is not code, so it is not blanked either —
  # the caller decides whether to keep it, and CMT says where it starts.
  if (CMT > 0) out = out substr(s, CMT)
  BLANK = out
  # A q or s cannot legally span a line, so it does not carry — unless the line
  # ends in the backslash TypeScript continues one with. An interpolation and
  # the template around it DO carry, comment or not: a `//` inside `${…}` is a
  # real comment and the interpolation is still open after it.
  while ((top() == "q" || top() == "s") && s !~ /\\$/) pop()
}

# commentAt answers only the first half, for a caller holding one line in hand
# rather than walking a file — including a caller re-scanning an already
# stripped copy of the CURRENT line, which is why it resumes from the state the
# line began in. It leaves the carried state exactly as it found it.
function commentAt(s,   keepStack, keepIn, keepCmt, keepBlank, at) {
  keepStack = STACK; keepIn = STACKIN; keepCmt = CMT; keepBlank = BLANK
  # Resume from the contexts the line STARTED in — including an interpolation
  # still open from an earlier line, which is where a waiver can legally sit.
  STACK = STACKIN
  lexLine(s)
  at = CMT
  STACK = keepStack; STACKIN = keepIn; CMT = keepCmt; BLANK = keepBlank
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
# has not closed, STACK for every string and interpolation that has not. An all-comment
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
  entry = STACK
  CODE = s
  for (guard = 0; guard < 100; guard++) {
    STACK = entry
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
# Call closeFile() at FNR == 1 and again in END. It owns the reset too, so a
# context added later cannot be forgotten at a file boundary.
function closeFile() {
  # STACK and not just the template flag: an interpolation left open leaks into
  # the NEXT file exactly as a string does, and reporting the file as readable
  # because only the outer context happened to be closed is the same lie in a
  # narrower place.
  if (SCANNED != "" && (STACK != "" || INBLOCK)) print "commentscan-unclosed:" SCANNED
  SCANNED = FILENAME
  STACK = ""; INBLOCK = 0
}

# waived: the marker appears in a REAL comment on this line.
function waived(s, marker,   at) {
  at = commentAt(s)
  return at > 0 && index(substr(s, at), marker) > 0
}
