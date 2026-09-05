# The selector-aware spacing scanner behind check-ds-spacing-roles.sh.
#
# One parser, two modes, because the gate's two questions are the same question
# asked of two trees:
#
#   mode=owned   over frontend/src/design-system/*.css — print the class each
#                rule SPACES, which is the set of classes the design system owns
#                the spacing of. This is the gate's corpus, derived from the
#                owner rather than listed here, so a primitive added tomorrow is
#                protected the day it exists.
#
#   mode=check   over one screen stylesheet — report the declarations this
#                branch adds that either re-space one of those owned classes or
#                spell a raw rung in a context that has a named role. Three
#                inputs, in this order: the owned list, the file's
#                `git diff --unified=0`, the file itself.
#
# Why the file itself and not the diff alone: a declaration, a comment and a
# selector can each span lines, and a diff line read in isolation carries none
# of that state. The diff says WHICH lines are new; the file says what they mean.

function decomment(line,   out, p) {
  out = ""
  while (length(line) > 0) {
    if (incomment) {
      p = index(line, "*/")
      if (p == 0) return out
      line = substr(line, p + 2)
      incomment = 0
    } else {
      p = index(line, "/*")
      if (p == 0) return out line
      out = out substr(line, 1, p - 1)
      line = substr(line, p + 2)
      incomment = 1
    }
  }
  return out
}

# The class a compound selector actually styles: the LAST class in it, with
# pseudo-classes, pseudo-elements and attribute tests removed. `.panel-body`
# alone is the panel's body; `.panel-body.co-360-head` is a screen's own
# element that happens to also be one, and the screen owns what it spaces.
function subject(part,   n, i, arr, last) {
  gsub(/\[[^]]*\]/, " ", part)
  gsub(/::?[a-zA-Z-]+(\([^)]*\))?/, " ", part)
  n = split(part, arr, /[^A-Za-z0-9_.-]+/)
  last = ""
  for (i = 1; i <= n; i++) {
    if (arr[i] ~ /^\./) last = substr(arr[i], 2)
  }
  return last
}

function spacing_prop(p) {
  return p ~ /^(padding|margin)(-[a-z]+)*$/ || p ~ /^(gap|row-gap|column-gap)$/
}

# A value that measures nothing: every length in it is zero, and it reaches for
# no token. `margin: 0 auto` centres, `padding: 0` resets — neither is a rhythm
# decision, and a gate that argued with them would be arguing with correct code.
function measures(value) {
  return value ~ /var\(/ || value ~ /[1-9]/
}

function role_hint(token) {
  return "var(" token ")"
}

# Removes every occurrence of a LITERAL from a string. `var(--padCard)` is not
# a regular expression — its parentheses and dashes would read as one — so the
# cut is done by index rather than by gsub.
function strip_literal(s, lit,   out, p) {
  out = ""
  while ((p = index(s, lit)) > 0) {
    out = out substr(s, 1, p - 1) " "
    s = substr(s, p + length(lit))
  }
  return out s
}

function report(tag, lineno, decl, message,   shown) {
  shown = decl
  gsub(/[ \t]+/, " ", shown)
  sub(/^ /, "", shown)
  sub(/ +$/, "", shown)
  printf "%s\t%s:%d: %s — %s\n", tag, target, lineno, shown, message
}

# Judges ONE complete declaration against the rules, in the order that puts the
# most specific message first: a screen re-spacing a primitive is told that,
# rather than being told to pick a role token for a surface it does not own.
function judge(sel, prop, value, lineno, decl,   subj, i, n, parts, bad) {
  if (!spacing_prop(prop) || !measures(value)) return

  n = split(sel, parts, ",")
  for (i = 1; i <= n; i++) {
    subj = subject(parts[i])
    if (subj == "") continue

    if (subj in owned) {
      report("primitive", lineno, decl,
             "." subj " is a design-system primitive and carries its own spacing")
      return
    }

    if (subj ~ /(^|-)actions$/ && (prop == "gap" || prop == "column-gap")) {
      if (value != role_hint(roleActions)) {
        report("role", lineno, decl,
               "buttons side by side take " role_hint(roleActions))
        return
      }
    }

    if (subj ~ /(^|-)(cards|card-stack|card-list|card-grid)$/ && (prop == "gap" || prop == "row-gap")) {
      if (value != role_hint(roleCards)) {
        report("role", lineno, decl,
               "sibling card surfaces take " role_hint(roleCards))
        return
      }
    }

    if (subj ~ /(^|-)(card|panel)$/ && prop == "padding") {
      bad = strip_literal(value, role_hint(padCard))
      bad = strip_literal(bad, role_hint(padPanel))
      if (measures(bad)) {
        report("role", lineno, decl,
               "a card's own inset is " role_hint(padCard) ", a panel's " role_hint(padPanel))
        return
      }
    }
  }
}

# Feeds one line of a stylesheet to the scanner. `{`, `}` and `;` are the only
# structure that matters: the first opens a rule (or an at-rule, which carries
# no subject of its own), the second closes one, the third ends a declaration.
# Text before any of them is carried forward, so a selector or a value split
# across lines is read as the one thing it is.
function feed(lineno, raw,   code, p, ch, seg, waived) {
  code = decomment(raw)
  waived = (raw ~ /ds:ignore/)
  if (waived) pending_waived = 1
  while (length(code) > 0) {
    p = 0
    if (match(code, /[{};]/)) p = RSTART
    if (p == 0) {
      pending = pending " " code
      if (pending_line == 0) pending_line = lineno
      return
    }
    ch = substr(code, p, 1)
    seg = substr(code, 1, p - 1)
    if (pending_line == 0) pending_line = lineno
    if (ch == "{") {
      pending_waived = 0
      sel = pending seg
      gsub(/[ \t]+/, " ", sel)
      sub(/^ /, "", sel)
      sub(/ $/, "", sel)
      depth++
      selstack[depth] = (sel ~ /^@/) ? "" : sel
    } else if (ch == "}") {
      # The last declaration in a rule may carry no trailing semicolon, so the
      # closing brace is what ends it. Judged before the pop, or the rule it
      # belonged to would already be off the stack.
      inspect(pending seg, waived)
      pending_waived = 0
      if (depth > 0) {
        delete selstack[depth]
        depth--
      }
    } else {
      inspect(pending seg, waived)
      pending_waived = 0
    }
    pending = ""
    pending_line = 0
    code = substr(code, p + 1)
  }
}

# One declaration, however many lines it spanned and whatever ended it. The
# property is lowercased because CSS property names are case-insensitive; the
# value is NOT, because a custom property name is case-sensitive and
# `var(--padcard)` is a different (undefined) property from `var(--padCard)`.
function inspect(decl, waived,   prop, value) {
  if (index(decl, ":") == 0) return
  if (!(pending_line in added) || pending_waived || waived) return
  prop = tolower(substr(decl, 1, index(decl, ":") - 1))
  value = substr(decl, index(decl, ":") + 1)
  gsub(/^[ \t]+|[ \t]+$/, "", prop)
  gsub(/^[ \t]+|[ \t]+$/, "", value)
  gsub(/[ \t]+/, " ", value)
  if (mode == "owned") {
    if (spacing_prop(prop) && measures(value)) collect(selstack[depth])
    return
  }
  judge(selstack[depth], prop, value, pending_line, decl)
}

# mode=owned: the subject of every rule the design system spaces.
function collect(sel,   i, n, parts, subj) {
  n = split(sel, parts, ",")
  for (i = 1; i <= n; i++) {
    subj = subject(parts[i])
    if (subj != "") print subj
  }
}

BEGIN {
  depth = 0
  part = 0
}

# Each new input file opens the next part. In mode=owned there is only one part
# and every line of it is a subject, so `added` is filled unconditionally there.
FNR == 1 { part++; incomment = 0; pending = ""; pending_line = 0; pending_waived = 0 }

mode == "owned" { added[FNR] = 1; feed(FNR, $0); next }

part == 1 { owned[$0] = 1; next }

# The diff: which line numbers of the NEW file this branch adds. A hunk header
# resets the counter, an added line consumes one, and a removed line consumes
# none because it does not exist in the new file.
part == 2 {
  if (/^@@/) {
    match($0, /\+[0-9]+/)
    ln = substr($0, RSTART + 1, RLENGTH - 1) + 0
    next
  }
  if (/^\+\+\+/ || /^-/ || /^\\/) next
  if (/^\+/) { added[ln++] = 1; next }
  ln++
  next
}

part == 3 { feed(FNR, $0) }
