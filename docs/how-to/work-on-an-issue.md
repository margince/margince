# Work on an issue without colliding

Several sessions read the same tracker, and the interesting issues look
interesting to all of them at once. Two of them start the same work, both write
a diff, and one of those diffs is thrown away along with everything it cost —
which is most of a context window, and a colleague's afternoon.

A comment is not how that gets avoided. "Taking this one" sits in a thread
nobody opens, while the issue list keeps showing the row as free. **The claim is
an assignee plus `status: in progress`**, because those are the two things the
list shows and a search can filter on.

## Before you start

```sh
gh issue view <n> --json assignees,labels,closedByPullRequestsReferences
```

The issue is **taken** if any of these is true:

- an assignee other than you — `gh api user -q .login` is who you are, which is
  not always who ran the last session on this machine;
- the label `status: in progress`;
- a still-open pull request under `closedByPullRequestsReferences`. A branch
  already exists, and whoever opened it is working from the same issue.

Anything else and it is free. An issue that is merely old is still free: there
is no expiry here, and nothing takes a claim over on its own.

## It is taken

Do not work it. Say so, in this order, to whoever asked you:

1. **Who holds it** — the assignee's login, or the number of the open pull
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

**If a second assignee appeared while you were writing, you are the later
claimant and you back off.** Remove yourself, say who got there first, and pick
something else. Checking and then claiming is not one atomic step, so the window
is real — it is the same race the check exists to close, arriving through the
check.

## Taking one over anyway

The point of all of this is awareness, not a lock. If the one who asked you
knows the issue is held and still wants it worked, that is their call, and you
work it.

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

Finishing is nothing extra: a merged pull request whose body says `Closes #N`
closes the issue and retires the claim with it.

Stopping **without** finishing is the case that needs you:

```sh
gh issue edit <n> --remove-assignee @me --remove-label "status: in progress"
gh issue comment <n> --body "Stopped here: <done, not done, what I learned>."
```

All three — label off, assignment off, comment — and the comment is the
valuable one, because whoever picks it up next starts from your evidence
instead of rediscovering it. A claim left behind by a dead session is worse
than no claim: it reads as active work forever, and no timer clears it.

## Claim the sub-issue, not the tracker

A tracker gathers children and is worked by nobody directly. Assigning yourself
to it says every child is taken, which stands the colleagues who would have
worked its siblings down. Claim the child you are actually writing, and leave
the parent alone.
