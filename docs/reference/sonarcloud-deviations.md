# SonarCloud findings this tree does not fix

SonarCloud is one of the gates a pull request has to pass, so a finding it
raises is normally fixed. These are the exceptions: findings that are open in
the dashboard, that a reader will meet again the next time they sort the issue
list, and that stay open by decision.

Each entry says what the rule wants, what the code does instead, and what would
break if somebody "fixed" it. A finding that is merely inconvenient does not
belong here; only one where applying the rule would make the product worse.

## godre:S8188: `laneLifetime` in `backend/cmd/worker/lanes.go`

**What the rule wants.** `context.WithCancel` should be followed by
`defer cancel()` in the same function, so the context cannot outlive the call
that created it.

**What the code does.** `laneLifetime` creates the context the worker's event
lanes live on. It returns three values: the context, the wait group that counts
the lanes, and a closure that cancels the context and waits for every lane to
leave. The caller, `run` in `backend/cmd/worker/main.go`, defers that closure
on the line after the call, before any lane exists.

**Why the rule cannot be applied.** The cancel is the *shutdown* of the lanes,
not a cleanup of the constructor. Deferring it inside `laneLifetime` would
cancel the context as that function returns, so every lane would be handed a
context that is already done and the worker would consume nothing. The lifetime
is meant to outlive its constructor. S8188 cannot tell that shape from a leak:
it sees a `CancelFunc` leave the function and cannot follow it into the closure
that calls it.

**What holds it instead.** The caller has to defer the closure; nothing in the
type system makes it. Returning the three values together removes the ordering
hazard. A lane cannot exist before its own shutdown is registered, because the
wait group a lane must be given comes from the same call that produced the
closure. A caller that discards the closure still has no shutdown, and the
tests catch that. `TestJoinCancelsTheLanesAndWaitsForThem` covers the cancel
and the wait, and `TestJoinReportsALaneThatDoesNotStop` the bounded overrun.

## The accessibility rules, on a table and two menus that are already correct

Thirteen findings across `frontend/src/design-system/` and
`frontend/src/app/account.tsx`: `typescript:S6825`, `S6843`, `S6845`, `S6848`
and `S6852`. They stay open together because they are one mistake made five
ways. Each rule states a default posture that the code departs from for a
reason WCAG itself gives, and applying the rule would take accessibility away.
Six of the sites already carry a `biome-ignore` naming the reason.

**`ListTable` needs its explicit roles (`S6843`, four sites).** The rule reads
`role="cell"` on a `<td>` as an interactive element given a non-interactive
role. A `<td>` is not interactive, and the role is not redundant. At 720px and
below, `listtable.css` sets `display: block` and `display: flex` on
`.lt-table`, `thead`, `tbody`, `tr`, `th` and `td`, which strips a table of its
implicit ARIA semantics. The explicit roles keep a phone list a table for a
screen reader. The CSS says so where it does it, and `listtable.test.tsx` reads
the rows and cells back by role. Removing the roles would break the phone
table; the tests would stay green only if the roles were removed from them too.

**The slack cell is a spacer** (`S6825`, three sites). The rule forbids
`aria-hidden` on a focusable element. `<td className="lt-slack">` has no
`tabindex` and is not an interactive element, so it is not focusable, and
hiding a layout spacer is what `aria-hidden` is for. The header row, every body
row and the empty-state row's `colSpan` all account for the same extra column,
so the table's geometry stays consistent with it hidden.

**A scrollable region must be reachable** (`S6845`, three sites). The rule
forbids `tabIndex` on a non-interactive element. WCAG 2.1.1 requires a
scrollable region to be operable by keyboard, and axe's
`scrollable-region-focusable` fails a scroller that is not. That is why
`composed.tsx` and `trust.tsx` put the tab stop on the scroller itself. In
`decisiondeck.tsx` the stop makes the four arrow-key verdicts reachable at all.
Removing any of the three replaces a lint finding with a real accessibility
failure, which the `uat` lane's axe pass would catch.

**The Escape catcher is not a control (`S6848`).** The `<span>` in
`inlinechoice.tsx` carries a `keydown` that only sees an Escape the `Select`
inside it declined to claim. Giving it a role and a tab stop, as the rule asks,
would announce a control that does not exist and put a stop in the tab order
that leads nowhere.

**A widened hit area adds no control** (`S6848`). The same reading
applies to the `<div className="arhit">` in `app/agentrail.tsx`. The control is
the `<button>` inside it, which carries the accessible name and the expanded
state. The div exists so a pointer can also press the words and the chevron
beside it. The keyboard already reaches the button, whose Enter and Space
arrive at the div as the same click. A role here would announce a control that
is not one and put a tab stop in front of the one that is. Two `biome-ignore`
lines beside it give the same reason.

**The menus use a roving tabindex** (`S6852`, two sites). The rule wants the
`role="menu"` container focusable. In both menus focus lives on the items: each
`menuitem` carries `tabIndex={active ? 0 : -1}`, which is the WAI-ARIA
authoring practice for a menu. The container is never focused. Its ref is read
only for `contains(document.activeElement)`, and the menu focuses a row on open.
A `tabIndex` on the container would be either a second tab stop in front of the
item that already has one, or an attribute nothing uses.

## typescript:S8786: `stripEveryKeyTag` in `frontend/src/screens/projectrecord.ts`

**What the rule wants.** ` ?\[[^\]]*\] ?` backtracks super-linearly. It does:
a subject of 20 000 unclosed `[` takes 592 ms, and four times that for each
doubling. The rule is right about the shape.

**Why there is no safe rewrite.** The tempting fix excludes `[` from the inner
class, ` ?\[[^\][]*\] ?`. It is linear, but it changes which tags are stripped.
On `Fix [old[PROJ-1] thing` the current pattern reads the tag as `old[PROJ-1`,
which is not key-shaped, and leaves the subject byte for byte. The "fixed" one
finds `[PROJ-1]` inside it and strips a tag this module never wrote. The
function's contract is that the rep's own text comes back unchanged. Sibling
patterns in the tree were rewritten only where a rewrite was proven identical
on every string up to length five, and this one has no such rewrite.

**What bounds it instead.** The input is a mail subject, which RFC 5322 lines
cap near 1 000 bytes; at that length the pattern costs microseconds. If a
subject ever arrives unbounded, the fix is a forward scan with `indexOf` (the
first `]` after a `[` is what the pattern means), not the one-character class
change.
