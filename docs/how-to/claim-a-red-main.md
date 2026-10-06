# Claim a red `main`

`main` goes red here often, by design: merging past a red required check is a
standing decision, so a break reaches the tip and every open pull request
inherits it through its merge commit. With many sessions running at once,
several of them notice the same failure within minutes and all reach for the
same fix.

A claim removes that race. Diagnosing a red lane costs a session its whole
context window, and the second and third sessions to do it produce nothing but
duplicate pull requests while the original author's fix lands anyway.

The claim is a **draft pull request** labelled `claim: main-red`. One command
finds it, and it exists before the fix does.

## Before you investigate a failure you did not cause

```sh
gh pr list --state open --label "claim: main-red" \
  --json number,title,body,updatedAt,author
```

Read the bodies as well as the titles. Each claim lists the lanes and tests it
covers, and you match your failure against that list. Whether `main` is red is
nearly always yes, so it tells you nothing.

**If a claim covers your failure**, stop. Do not investigate it, do not open a
second fix, and do not sit and poll it either: waiting burns the tokens a claim
saves. Your own pull request is red for a reason that is not yours, so carry on
with your own work and look again when you are ready to merge.

**If nothing covers it**, you are first for this cause, even when another claim
is open for a different one. `main` is regularly red for two or more unrelated
reasons at once, and a claim naming one of them says nothing about the others.

## Opening a claim

Open the claim **before** you start diagnosing. Its value is in the minutes it
saves the next session.

```sh
git switch -c red/<lane-or-test-slug> origin/main
git commit --allow-empty -m "claim: <lane> is red on main"
git push -u origin red/<lane-or-test-slug>
gh pr create --draft --label "claim: main-red" \
  --title "main is red: <lane>" \
  --body "$(printf 'Claiming:\n- TestOne\n- TestTwo\n\nCause: unknown so far.\n')"
```

**A pull request with no diff is allowed.** GitHub asks only that the head
branch carry a commit the base does not, and the empty commit provides one, so
the claim opens before a line of the fix is written. Do not wait until you have
a fix to show: by then the second session has already started.

A draft runs almost no CI and gets no CodeRabbit review. `ci.yml` runs the
`changes` classifier only on `draft == false`, so every lane gated on its output
skips, and `.coderabbit.yaml` sets `drafts: false`. The draft state keeps it
cheap whatever the diff holds, so push your investigation to it as you work. It
costs CI only once you mark it ready.

If `main-health` has already filed a `main is red:` issue, assign yourself to it
at the same time. Unassigned reads as unclaimed, and the issue is where anyone
not watching pull requests will look.

## Keep the body current

**The body is the interface.** Another session decides whether to stand down by
reading your list, so keep it current: add a test when the failure turns out
wider than you thought, and say so if it turns out narrower. A claim that covers
less than it lists leaves a real failure owned by nobody.

**Say what you learn.** A comment naming the cause, or naming what you ruled
out, saves the next session from rediscovering it, and is worth more than the
diff.

## Releasing it

- **Fixed**: mark the pull request ready and merge it. The claim goes with it.
- **Abandoned**: close the draft, and say in a comment what you found, so the
  next session starts from your evidence.
- **Not actually broken**: close it and say why the verdict was wrong. A claim
  left open over a green `main` stops somebody looking at a real failure later.
- **Somebody else fixed the cause**: close it and name the pull request that
  did. Merging releases only a claim fixed by its own change. A claim fixed by
  another change stays open, and a dead problem sits at the top of the list the
  next session reads first.

## When two sessions claim the same thing

Listing the open claims and then creating one is not atomic. Two sessions
reaching the same red within the same few seconds both find nothing and both
open a claim.

**The lower pull request number wins.** It existed first, every session can see
it, and no clock has to agree. If yours is the higher number and its covered
tests overlap:

- close yours, with a comment pointing at the winner;
- post what you found on the winner's claim before you close yours;
- carry on with your own work.

Overlap is per test. If your claim covers three tests and only one is also on
the winner's list, drop that one from your body and keep going: the other two
are still unclaimed, and dropping them would leave them owned by nobody.

## Taking over a stale claim

A session can die, and a claim it left behind would block the repository for
good. A claim with no commit and no comment for **90 minutes** may be taken
over. Say so in a comment on it first, so the original session sees what
happened if it wakes up, then carry on in your own branch.

The threshold is 90 minutes because diagnosing a red integration lane takes
that long. A threshold short enough to catch a dead session quickly would also
take work from a live one.

## When several causes are in flight

Each cause gets its own claim, and they land in whatever order they are ready.
Required checks run against the merge commit, so **two pull requests each
fixing half of a red `main` are both red.** Neither can go green while the other
is unmerged, and branch protection refuses both.

The way out is one branch merging both fixes. Its merge commit carries the whole
repair, so it is green and needs no administrator bypass.
