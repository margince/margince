# SonarCloud findings this tree does not fix

SonarCloud is one of the gates a pull request has to pass, so a finding it
raises is normally something to fix rather than something to argue with. This
page is the short list of the exceptions: findings that are open in the
dashboard, that a reader will meet again the next time they sort the issue list,
and that stay open on purpose.

One entry per finding, each saying what the rule wants, what the code does
instead, and what would break if somebody "fixed" it. A finding that is merely
inconvenient does not belong here — only one where applying the rule would make
the product worse.

## godre:S8188 — `laneLifetime` in `backend/cmd/worker/lanes.go`

**What the rule wants.** `context.WithCancel` should be followed by
`defer cancel()` in the same function, so the context cannot outlive the call
that created it.

**What the code does.** `laneLifetime` creates the context the worker's event
lanes live on and returns three values: the context, the wait group that counts
the lanes, and a closure that cancels the context and waits for every lane to
leave. The caller — `run` in `backend/cmd/worker/main.go` — defers that closure
on the line after the call, before any lane exists.

**Why the rule cannot be applied.** The cancel is the *shutdown* of the lanes,
not a cleanup of the constructor. Deferring it inside `laneLifetime` would
cancel the context as that function returns, so every lane would be handed a
context that is already done and the worker would consume nothing. The lifetime
deliberately outlives its constructor, which is the one shape S8188 has no way
to distinguish from a leak: it reads a `CancelFunc` that leaves the function and
cannot follow it into the closure that calls it.

**What holds it instead.** The caller has to defer the closure — nothing in the
type system makes it. What returning the three values together removes is the
*ordering* hazard — the half nothing was holding when a deleted `defer` left
every test in the repository passing: a lane cannot exist before its own
shutdown is registered, because the wait group a lane must be given comes from
the same call that produced the closure. A caller that discards the closure is
still a caller with no shutdown, and the tests are what say otherwise —
`TestJoinCancelsTheLanesAndWaitsForThem` covers the cancel and the wait,
`TestJoinReportsALaneThatDoesNotStop` the bounded overrun.

## The accessibility rules, on a table and two menus that are already correct

Thirteen findings across `frontend/src/design-system/` and
`frontend/src/app/account.tsx` — `typescript:S6825`, `S6843`, `S6845`, `S6848`
and `S6852`. They stay open together because they are one mistake made five
ways: each rule states a default posture that the code deliberately departs
from, for a reason WCAG itself gives, and applying the rule would take
accessibility away rather than add it. Six of the sites already carry a
`biome-ignore` naming the reason, written before SonarCloud ever ran.

**`ListTable`'s explicit roles are load-bearing (`S6843`, four sites).** The
rule reads `role="cell"` on a `<td>` as an interactive element given a
non-interactive role. A `<td>` is not interactive, and the role is not
redundant: at 720px and below, `listtable.css` sets `display: block` and
`display: flex` on `.lt-table`, `thead`, `tbody`, `tr`, `th` and `td`, which is
exactly what strips a table of its implicit ARIA semantics. The explicit roles
are what keep a phone list a table for a screen reader — the CSS says so where
it does it, and `listtable.test.tsx` reads the rows and cells back by role.
Removing them would break the phone table silently, with every test still green
only if the roles were removed from the tests too.

**The slack cell is a spacer, not a control (`S6825`, three sites).** The rule
forbids `aria-hidden` on a focusable element. `<td className="lt-slack">` has no
`tabindex` and is not an interactive element, so it is not focusable; hiding a
layout spacer is what `aria-hidden` is for. The header row, every body row and
the empty-state row's `colSpan` all account for the same extra column, so the
table's geometry stays consistent with it hidden.

**A scrollable region must be reachable (`S6845`, three sites).** The rule
forbids `tabIndex` on a non-interactive element. WCAG 2.1.1 requires a
scrollable region to be operable by keyboard, and axe's
`scrollable-region-focusable` fails a scroller that is not — which is why
`composed.tsx` and `trust.tsx` take the stop on the scroller itself. In
`decisiondeck.tsx` the stop is what makes the four arrow-key verdicts reachable
at all. Removing any of the three replaces a lint finding with a real
accessibility failure, one the `uat` lane's axe pass would catch.

**The Escape catcher is not a control (`S6848`).** The `<span>` in
`inlinechoice.tsx` carries a `keydown` that only ever sees an Escape the `Select`
inside it declined to claim. Giving it a role and a tab stop, which is what the
rule asks for, would announce a control that does not exist and put a stop in
the tab order that leads nowhere.

**The menus use a roving tabindex (`S6852`, two sites).** The rule wants the
`role="menu"` container focusable. In both menus focus lives on the items —
each `menuitem` carries `tabIndex={active ? 0 : -1}` — which is the WAI-ARIA
authoring practice for a menu. The container is never focused: its ref is read
only for `contains(document.activeElement)`, and the menu focuses a row on open.
A `tabIndex` on the container would be either a second tab stop in front of the
item that already has one, or an attribute nothing uses.

## typescript:S8786 — `stripEveryKeyTag` in `frontend/src/screens/projectrecord.ts`

**What the rule wants.** ` ?\[[^\]]*\] ?` backtracks super-linearly. It does:
a subject of 20 000 unclosed `[` takes 592 ms, and four times that for each
doubling. The rule is right about the shape.

**Why this one is not rewritten with the others.** Five sibling findings were
fixed in the same change, each by a rewrite proven identical on every string up
to length five. This one has no such rewrite. The tempting fix — excluding `[`
from the inner class, ` ?\[[^\][]*\] ?` — is linear and silently changes which
tags are stripped: on `Fix [old[PROJ-1] thing` the current pattern reads the tag
as `old[PROJ-1`, is not key-shaped, and leaves the subject byte for byte, while
the "fixed" one finds `[PROJ-1]` inside it and strips a tag this module never
wrote. The function's contract is that the rep's own text comes back unchanged.

**What bounds it instead.** The input is a mail subject, which RFC 5322 lines
cap near 1 000 bytes; at that length the pattern costs microseconds. If a
subject ever arrives unbounded, the fix is a forward scan with `indexOf` — the
first `]` after a `[` is what the pattern means — and not the one-character
class change.
