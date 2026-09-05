# The selector-aware spacing scanner behind check-ds-spacing-roles.sh.
#
# One parser, two modes, because the gate's two questions are the same question
# asked of two trees:
#
#   mode=owned   over frontend/src/design-system/*.css — print two claims per
#                rule: `spaced <class>` for a class this tier gives an interval
#                to, and `own <class>` for a class it declares ON ITS OWN, with
#                no ancestor above it in the selector. The gate keeps the
#                intersection, which is the set of primitives, derived from the
#                owner rather than listed there.
#
#                Both halves are needed, and the second is what keeps the corpus
#                honest: a design-system sheet may space a class belonging to a
#                SCREEN (`.mw-conversation .ob-conv-thread` places the screen's
#                thread inside the workbench). Spacing alone would read that as
#                the tier owning `.ob-conv-thread` and turn the screen's own
#                base rule into a finding. Declaring a class with nothing above
#                it is what owning it looks like.
#
#   mode=check   over one screen stylesheet — report every declaration that
#                either re-spaces one of those owned classes or spells a raw
#                rung in a context that has a named role. Two inputs, in this
#                order: the owned list, then the stylesheet.
#
# A declaration, a comment and a selector can each span lines, so the scanner
# carries all three states across them: the unit judged is one declaration,
# whatever it was spelled over and whatever ended it.

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

# The class a selector actually styles: the last class of its LAST compound,
# with pseudo-classes, pseudo-elements and attribute tests removed.
#
# The last compound and not the last class anywhere in the selector, because
# `.rdeck-card input` styles the INPUT — the card is where it sits. Reading the
# trailing class out of the whole selector made every rule that reached a plain
# element inside a card look like a rule about the card.
#
# `.panel-body` alone is the panel's body; `.panel-body.co-360-head` is a
# screen's own element that happens to also be one, and the screen owns what it
# spaces. A last compound carrying no class at all (`.panel-body > p`) makes no
# claim: this gate is about named surfaces.
function subject(part,   n, i, arr, comp, last) {
  gsub(/\[[^]]*\]/, " ", part)
  gsub(/::?[a-zA-Z-]+(\([^)]*\))?/, " ", part)
  gsub(/[>+~]/, " ", part)
  gsub(/^[ \t]+|[ \t]+$/, "", part)
  n = split(part, arr, /[ \t]+/)
  comp = arr[n]
  last = ""
  n = split(comp, arr, /[^A-Za-z0-9_.-]+/)
  for (i = 1; i <= n; i++) {
    if (arr[i] ~ /^\./) last = substr(arr[i], 2)
  }
  return last
}

# Splits a selector LIST on its top-level commas. `:is(.card, .panel)` carries
# commas of its own, and a plain split on "," cuts that pseudo in half — which
# leaves fragments that still parse as selectors and name classes nobody wrote a
# rule about.
function split_parts(sel, parts,   n, i, depth, ch, cur) {
  n = 0
  depth = 0
  cur = ""
  for (i = 1; i <= length(sel); i++) {
    ch = substr(sel, i, 1)
    if (ch == "(") depth++
    else if (ch == ")") { if (depth > 0) depth-- }
    if (ch == "," && depth == 0) {
      parts[++n] = cur
      cur = ""
      continue
    }
    cur = cur ch
  }
  parts[++n] = cur
  return n
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

# The value with every role token cut out of it, so what is left is whatever
# the declaration says that the role layer does not.
function without_roles(value,   rest) {
  rest = strip_literal(value, role_hint(roleActions))
  rest = strip_literal(rest, role_hint(roleCards))
  rest = strip_literal(rest, role_hint(padCard))
  return strip_literal(rest, role_hint(padPanel))
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

  n = split_parts(sel, parts)
  for (i = 1; i <= n; i++) {
    subj = subject(parts[i])
    if (subj == "") continue

    if (primitives && subj in owned) {
      # A variant spelled in the house's own vocabulary is not a second
      # opinion: `padding: var(--padCard)` on a rail's panel body says which
      # surface it means and moves when that surface is retuned. An ad-hoc rung
      # says only a number, and the number is what drifts.
      if (measures(without_roles(value))) {
        report("primitive", lineno, decl,
               "." subj " is a design-system primitive — vary it with a role token, or say why not")
      }
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
  # A waiver on a line of its own waives the declaration UNDER it, which is
  # where a CSS comment normally goes and the only place a long reason fits:
  # the formatter wraps a declaration whose trailing comment overruns, and a
  # value broken across three lines to make room for its excuse is worse code
  # than the one being excused.
  if (waived && code !~ /[^ \t]/) {
    armed = 1
    return
  }
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
  if (armed) {
    armed = 0
    return
  }
  if (pending_waived || waived) return
  prop = tolower(substr(decl, 1, index(decl, ":") - 1))
  value = substr(decl, index(decl, ":") + 1)
  gsub(/^[ \t]+|[ \t]+$/, "", prop)
  gsub(/^[ \t]+|[ \t]+$/, "", value)
  gsub(/[ \t]+/, " ", value)
  if (mode == "owned") {
    collect(selstack[depth], spacing_prop(prop) && measures(value))
    return
  }
  judge(selstack[depth], prop, value, pending_line, decl)
}

# mode=owned: what one design-system rule claims about the classes in it.
function collect(sel, spaces,   i, n, parts, subj, bare) {
  n = split_parts(sel, parts)
  for (i = 1; i <= n; i++) {
    subj = subject(parts[i])
    if (subj == "") continue
    if (spaces) print "spaced " subj
    # `.card`, `.panel-head:has(.panel-head-sub)` and `.card.card-inset` all
    # declare their own subject; `.panel-body > .empty` and `.settinglist >
    # .disclosure` place someone else's inside them.
    bare = parts[i]
    gsub(/\[[^]]*\]/, " ", bare)
    gsub(/::?[a-zA-Z-]+(\([^)]*\))?/, " ", bare)
    gsub(/^[ \t]+|[ \t]+$/, "", bare)
    if (bare ~ /^\.[A-Za-z0-9_-]+(\.[A-Za-z0-9_-]+)*$/) print "own " subj
  }
}

BEGIN {
  depth = 0
  part = 0
  armed = 0
  # A document that does not load the class layer cannot collide with it; the
  # caller says so by passing primitives=0. Everything else is judged.
  if (primitives == "") primitives = 1
}

# Each new input file opens the next part, and resets the state that belongs to
# one file rather than to the run.
FNR == 1 { part++; incomment = 0; pending = ""; pending_line = 0; pending_waived = 0; armed = 0 }

mode == "owned" { feed(FNR, $0); next }

# mode=check reads the owned list first, then the stylesheet it is judging.
part == 1 { owned[$0] = 1; next }

{ feed(FNR, $0) }
