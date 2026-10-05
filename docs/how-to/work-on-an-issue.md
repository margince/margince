# Work on an issue

Before you work an issue, check that nobody holds it and that its ruling still
matches the tree. The last section covers the ruling.

A comment such as "Taking this one" does not claim an issue: it sits in a thread
nobody opens, while the issue list keeps showing the row as free. The claim is **an assignee plus
`status: in progress`**, because those are what the list shows and a search can
filter on.

## Before you start

```sh
gh issue view <n> --json assignees,labels,closedByPullRequestsReferences
```

Every signal below is about somebody **else**, so start by learning who you are:
`gh api user -q .login`, which is not always who ran the last session on this
machine. The issue is **taken** if any of these is true:

- an assignee who is not you;
- the label `status: in progress` while the assignee is not you. With no
  assignee at all it is still taken and there is nobody to name. Say so,
  because nobody can ask about an unattributed claim;
- a still-open pull request under `closedByPullRequestsReferences` that you did
  not write. That list carries the number and no author, and it keeps merged
  and closed pull requests too, so read the one it names:

  ```sh
  gh pr view <pr> --json state,author -q '.state + " " + .author.login'
  ```

Anything else and it is free. **An issue where every signal points at you is
yours to resume**: no re-claim, no comment, just carry on. An issue that is
merely old is free too: there is no expiry here, and nothing takes a claim over
on its own.

A **reopened** issue keeps the assignee who closed it but not the label. The
assignee records who did the work and does not claim what is left. The first bullet still
applies, so ask them first; if they are not back on it, claim it afresh.

## It is taken

Do not work it. Say so, in this order, to whoever asked you:

1. **Who holds it**: the assignee's login, or the number of the open pull
   request that closes it.
2. **Ask them.** Questions about scope, and anything urgent, go to the one
   already holding it; that costs a message where a second diff costs a day.
3. **An alternative**, if there is one. Offer a nearby issue that is free
   rather than handing back an empty answer.

```sh
gh issue list --state open --label "area: <x>" \
  --search 'no:assignee -label:"status: in progress"' --limit 10
```

Take the `area:` from the issue you were pointed at, so what you offer is work
of the same shape. Add `--label "priority: high"` to narrow it further.

## Claim it

```sh
gh issue edit <n> --add-assignee @me --add-label "status: in progress"
```

Then read it again:

```sh
gh issue view <n> --json assignees -q '[.assignees[].login]'
```

**If somebody else appeared as assignee** while you were writing, you are the
later claimant and you back off. Remove yourself, say who got there first, and
pick something else. Checking and then claiming is not one atomic step, so two
sessions can both pass the check. After a takeover it reads one name higher: the original assignee is
expected to still be there, and only a *third* login means somebody beat you.

## Taking one over anyway

A claim informs; it does not lock. If the one who asked you knows the issue is
held and still wants it worked, that is their call, and you work it.

**Comment first, reassign second.** The comment says who is taking it over and
why, and it goes on before the assignee changes, so the session that loses the
issue finds an explanation rather than a silent reassignment:

```sh
gh issue comment <n> --body "Taking this over at <who asked>'s request: <why>."
gh issue edit <n> --add-assignee @me --add-label "status: in progress"
```

Leave the original assignee in place unless they have clearly stopped. Two
assignees who have talked to each other is a better state than one who was
replaced without being told.

## Release it when you stop

Finishing is nothing extra. GitHub closes the issue when a pull request whose
body says `Closes #N` merges, but it keeps the labels, so
[`issue-closed.yml`](../../.github/workflows/issue-closed.yml) strips
`status: in progress` from an issue the moment it closes. A close no event
reports (one a workflow makes with its own token) or a failed run is caught
instead by the next daily sweep, best-effort, since GitHub can delay or drop a
scheduled run. The assignee stays: it is the record of who did the
work. Closing one by hand needs nothing extra either; the workflow owns the
label.

**A `status: in progress` on a closed issue** is always stale; take it off. On
an open issue the rules under [Before you start](#before-you-start) apply
unchanged.

Stopping **without** finishing is the case that needs you. Unassign yourself and
say where you stopped, always:

```sh
gh issue edit <n> --remove-assignee @me
gh issue comment <n> --body "Stopped here: <done, not done, what I learned>."
```

**The label comes off** only once nobody holds the issue. `--remove-assignee
@me` drops your login alone, while `--remove-label` drops a signal the whole
issue shares. After a takeover left two names on an issue, taking both off in
one command strips the claim of an original assignee who is still working, and
the issue reads as free. Look first, remove on an empty list:

```sh
gh issue view <n> --json assignees -q '[.assignees[].login]'
gh issue edit <n> --remove-label "status: in progress"
```

The comment matters most: whoever picks the issue up next starts from your
evidence instead of rediscovering it. A claim left behind by a dead session
reads as active work forever, and no timer clears it.

## Claim the sub-issue, not the tracker

A tracker gathers children and is worked by nobody directly. Assigning yourself
to it says every child is taken, which turns away colleagues who would have
worked its siblings. Claim the child you are actually writing, and leave
the parent alone.

## Re-derive the ruling before you execute it

The claim is settled and the issue is yours. **Open the files it names before
you build any of it**, and check the prescription against what is there now,
as well as the premise, which is usually still true.

Recent, specific rulings go stale too.

Ask these of each item, because they fail differently:

- **Is it already done?** Somebody may have built it since, under another
  ticket or in passing. Grep for the thing before you write it.
- **Would it still be right here?** A fix copied from a neighbouring rule can be
  wrong in a way the diff cannot show. `company`'s `linkedin_url` CHECK matches
  `linkedin.com/company/…`, while a lead's URL is an `/in/…` profile, so
  copying that CHECK to leads would refuse the whole column. In review it reads
  as a correct copy of the rule beside it.
- **What else touches this?** An observation can be right and its scope wrong.
  Renaming one table's `source_system` to end a collision with `source` leaves
  every other table that carries both, and invents a second vocabulary.

**Fix what is there**, and say on the issue what you did not build and why. A
ruling that has moved is no reason to hand the work back: most of it is usually
still worth doing, in a different shape. The write-up is often worth more than
the diff, because it stops the next session paying to rediscover it.
