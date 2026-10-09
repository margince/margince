<!-- prose:plain -->
# SonarCloud findings this tree does not fix

SonarCloud is one of the gates a pull request has to pass, so we fix a finding
it reports as a rule. These are the exceptions. Each one is open in the
SonarCloud view, and a reader meets it again the next time they sort the
issue list. It stays open by decision.

Each entry says what the rule wants, what the code does instead, and what would
break if someone "fixed" it. A finding that is only a bother does not belong
here; only one where following the rule would make the product worse.

## `godre:S8188`: `laneLifetime` in `backend/cmd/worker/lanes.go`

**What the rule wants.** `context.WithCancel` should be followed by
`defer cancel()` in the same function, so the context cannot live longer than
the call that created it.

**What the code does.** `laneLifetime` creates the context that the event lanes
of the worker live on. It returns three values: the context, the wait group that
counts the lanes, and a closure. The closure cancels the context and waits for
every lane to stop. The caller, `run` in `backend/cmd/worker/main.go`, defers that
closure on the line after the call, before any lane exists.

**Why the rule cannot be applied.** The cancel is the *stop* of the lanes, not a
clean-up step of the function that builds them. A `defer` in `laneLifetime`
would cancel the context as that function returns, so every lane would get a
context that has already ended. The worker would read no events. 

The context must live longer than the function that builds it. `S8188` cannot tell that shape
from a context that never ends. It sees a `CancelFunc` leave the function, and
cannot follow it into the closure that calls it.

**What holds it instead.** The caller has to defer the closure; nothing in the
type system makes it. Returning the three values together removes the risk of a
wrong order.

A lane cannot exist before its own stop is in place. The wait
group a lane must get comes from the same call that made the closure. A
caller that drops the closure still has no stop, and the tests catch that.
`TestJoinCancelsTheLanesAndWaitsForThem` covers the cancel and the wait, and
`TestJoinReportsALaneThatDoesNotStop` covers a lane that runs past its limit.

## The accessibility rules, on a table and two menus that are already correct

14 findings in `frontend/src/design-system/`, `frontend/src/app/account.tsx`
and `frontend/src/app/agentrail.tsx`: `typescript:S6825`, `S6843`, `S6845`, `S6848`
and `S6852`. They stay open together because they are one error made five
ways. Each rule states a default that the code leaves for a reason WCAG itself
gives. Following the rule would make the product harder to use with a screen
reader or a keyboard. Six of the sites already carry a `biome-ignore`
that names the reason.

**`ListTable` needs its roles written out** (`S6843`, four sites). The rule
thinks `role="cell"` on a `<td>` puts a role for a fixed element on one a user
can act on. A `<td>` is not an element a user acts on, and the role is
not extra. On a screen up to `720px` across, `listtable.css` sets `display: block` and
`display: flex` on `.lt-table`, `thead`, `tbody`, `tr`, `th` and `td`. That
removes the ARIA roles a table has by default. The roles written out keep a
phone list a table for a screen reader.

The CSS says so where it does it, and `listtable.test.tsx` reads the rows and
cells back by role. Removing the roles
would break the phone table; the tests would stay green only if the roles were
removed from them too.

**The `lt-slack` cell only takes up space** (`S6825`, three sites). The rule does not allow
`aria-hidden` on an element that can take focus. `<td className="lt-slack">`
has no `tabindex` and a user cannot act on it, so it cannot take focus. Hiding
a cell that only takes up space is what `aria-hidden` is for. The header row,
every body row and the `colSpan` of the empty-state row all count the same
extra column. So the shape of the table stays the same with it hidden.

**An area that scrolls must be in reach** (`S6845`, three sites). The rule
does not allow `tabIndex` on an element a user does not act on. WCAG 2.1.1 requires
that a keyboard can work an area that scrolls, and the axe check
`scrollable-region-focusable` fails one that a keyboard cannot reach. That is
why `composed.tsx` and `trust.tsx` put the tab stop on the element that scrolls
itself. In `decisiondeck.tsx` the stop is the only way the keyboard reaches the
four verdicts on the `←` and `→` keys. Removing any of the three turns a lint finding
into a real accessibility error, which the axe pass in the `uat` lane would catch.

**The `Escape` handler is not a control** (`S6848`). The `<span>`
in `inlinechoice.tsx` carries a `keydown` that only sees an `Escape` that the
`Select` in it does not claim. Giving it a role and a tab stop, as the rule asks,
would tell a screen reader about a control that does not exist. It would also
put a stop in the tab order that leads to nothing.

**A larger click area adds no control** (`S6848`). The same reading applies to
the `<div className="arhit">` in `app/agentrail.tsx`. The control is the
`<button>` in it, which carries the name a screen reader says and the open or
closed state. The `div` exists so a pointer can also click the words and the
small `>` sign next to it. The keyboard already reaches the button, and its `Enter` and
`Space` reach the `div` as the same click.

A role here would tell a screen reader about a control that is not one. It
would also put a tab stop in front of the one that is. Two `biome-ignore`
lines next to it give the same reason.

**The menus move the tab stop** from item to item (`S6852`, two sites). The rule
wants the `role="menu"` container to take focus. In both menus focus lives on
the items: each `menuitem` carries `tabIndex={active ? 0 : -1}`, which is the
WAI-ARIA pattern for a menu. The container never takes focus.

Its `ref` is read
only for `contains(document.activeElement)`, and the menu moves focus to a row
when it opens. A `tabIndex` on the container would add a second tab stop in
front of the item that already has one. Or it would be a value nothing uses.

## `typescript:S8786`: `stripEveryKeyTag` in `frontend/src/screens/projectrecord.ts`

**What the rule wants.** The rule says ` ?\[[^\]]*\] ?` can take time that
grows much faster than the input. It does: a subject of 20 000 `[` with no
closing `]` takes 592 `ms`, and four times that each time the input grows to twice its size.
The rule is right about the shape.

**Why there is no safe rewrite.** The easy fix leaves `[` out of the inner
class: ` ?\[[^\][]*\] ?`. That one takes time in step with the input, but it changes which tags are
removed.

On `Fix [old[PROJ-1] thing` the current pattern reads the tag as `old[PROJ-1`,
which does not have the shape of a key. It leaves the subject byte for byte.
The "fixed" one finds `[PROJ-1]` in it and removes a tag this module
never wrote. The function's contract is that the rep's own text comes back
with no change.

Other patterns in the tree have a new form only where the new form gave the same
result on every string up to length five. This one has no such rewrite.

**What the code does instead.** `stripEveryKeyTag` no longer runs the pattern.
`bracketedGroups` scans the subject with `indexOf` and takes the first `]` after
each `[`, which is what the pattern means. The scan takes time in step with the
input, so it needs no limit on the length of a subject.
