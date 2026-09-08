# Claim a red `main`

`main` goes red here often, and on purpose: merging past a red required check is
a standing decision, so a break reaches the tip and every open pull request
inherits it through its merge commit. At the number of sessions this repository
runs at once, that means several of them notice the same failure within minutes
and all reach for the same fix.

That race is the thing this page removes. It costs a session its whole context
window to diagnose a red lane, and the second and third session to do it produce
nothing — one duplicate pull request, or two, and the original author's fix
lands anyway.

**The claim is a draft pull request wearing `claim: main-red`.** One command
finds it, and it exists before the fix does.

## Before you investigate a failure you did not cause

```sh
gh pr list --state open --label "claim: main-red" \
  --json number,title,body,updatedAt,author
```

Read the bodies, not just the titles. Each claim lists the lanes and tests it
covers, and **that list is what you match against** — not "is `main` red", which
is nearly always yes.

**If a claim covers your failure** — stop. Do not investigate it, do not open a
second fix, and do not sit and poll it either: waiting burns the tokens this
page exists to save. Your own pull request is red for a reason that is not
yours, so carry on with your own work and look again when you are ready to
merge.

**If nothing covers it** you are first for this cause, even when another claim
is open for a different one. `main` is regularly red for two unrelated reasons
at once — on 2026-09-08 it was red for three — and a claim naming one of them
says nothing about the others.

## Opening a claim

Do this **before** you start diagnosing, not after you have a fix. The whole
value is in the minutes it saves the next session.

```sh
git switch -c red/<lane-or-test-slug> origin/main
git commit --allow-empty -m "claim: <lane> is red on main"
git push -u origin red/<lane-or-test-slug>
gh pr create --draft --label "claim: main-red" \
  --title "main is red: <lane>" \
  --body "$(printf 'Claiming:\n- TestOne\n- TestTwo\n\nCause: unknown so far.\n')"
```

An empty commit is the honest first state: you are claiming the work, not
reporting a fix. Draft is not decoration either — `ci.yml` gates the frontend
and UAT lanes on `draft == false`, so a claim costs a fraction of a full run.

If `main-health` has already filed a `main is red:` issue, assign yourself to it
in the same breath. Unassigned reads as unclaimed, and the issue is where anyone
not watching pull requests will look.

## Keeping it honest

**The body is the interface.** Another session decides whether to stand down by
reading your list, so keep it current: add a test when you find the failure is
wider than you thought, and say so if it turns out narrower. A claim that quietly
covers less than it lists is how a real failure ends up owned by nobody.

**Say what you learn.** A comment naming the cause, or naming what you ruled
out, is worth more to the next session than the diff — it is the part they would
otherwise pay to rediscover.

## Releasing it

- **Fixed** — mark the pull request ready and merge it. The claim goes with it.
- **Abandoned** — close the draft, and say in a comment what you found, so the
  next session starts from your evidence rather than from nothing.
- **Not actually broken** — close it and say why the verdict was wrong. A claim
  left open over a green `main` stops somebody looking at a real failure later.

## Taking over a stale claim

A session can die, and a claim it left behind would otherwise block the repository
for good. **A claim with no commit and no comment for 90 minutes may be taken
over.** Say so in a comment on it first, so the original session sees what
happened if it wakes up, then carry on in your own branch.

Ninety minutes rather than ten: diagnosing a red integration lane genuinely takes
that long, and a threshold short enough to catch a dead session is short enough
to steal work from a live one.

## When several causes are in flight

Each cause gets its own claim, and they land in whatever order they are ready.
Be aware of the trap that follows: because required checks run against the merge
commit, **two pull requests each fixing half of a red `main` are both red, and
neither can go green while the other is unmerged.** Branch protection then
refuses both.

The way out is one branch merging both fixes — its merge commit carries the
whole repair, so it is green and needs no administrator bypass. This happened on
2026-09-08 and cost several hours; the record is issue #4852.
