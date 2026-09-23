# Work on an issue

Two things go wrong between picking an issue up and writing the diff, and they
are unrelated. Somebody else is already on it — most of this page. Or it is
yours and its ruling describes a tree that has moved, which is the last section.

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

Every signal below is about somebody **else**, so start by learning who you are
— `gh api user -q .login`, which is not always who ran the last session on this
machine. The issue is **taken** if any of these is true:

- an assignee who is not you;
- the label `status: in progress` while the assignee is not you. With no
  assignee at all it is still taken and there is nobody to name: say exactly
  that, because an unattributable claim is the one nobody can ask about;
- a still-open pull request under `closedByPullRequestsReferences` that you did
  not write. That list carries the number and no author, and it keeps merged
  and closed pull requests too, so read the one it names:

  ```sh
  gh pr view <pr> --json state,author -q '.state + " " + .author.login'
  ```

Anything else and it is free. **An issue where every signal points at you is
yours to resume** — no re-claim, no comment, just carry on. An issue that is
merely old is free too: there is no expiry here, and nothing takes a claim over
on its own.

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

**If somebody else appeared as assignee while you were writing, you are the
later claimant and you back off.** Remove yourself, say who got there first, and
pick something else. Checking and then claiming is not one atomic step, so the
window is real — it is the same race the check exists to close, arriving through
the check. After a takeover it reads one name higher: the original assignee is
expected to still be there, and only a *third* login means somebody beat you.

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

Stopping **without** finishing is the case that needs you. Unassign yourself and
say where you stopped, always:

```sh
gh issue edit <n> --remove-assignee @me
gh issue comment <n> --body "Stopped here: <done, not done, what I learned>."
```

**The label comes off only once nobody holds the issue.** `--remove-assignee
@me` drops your login alone, while `--remove-label` drops a signal the whole
issue shares — so taking both off in one command strips the claim of an original
assignee who is still working, after a takeover left two names on it, and the
issue goes back to reading free. Look first, remove on an empty list:

```sh
gh issue view <n> --json assignees -q '[.assignees[].login]'
gh issue edit <n> --remove-label "status: in progress"
```

Of the three, the comment is the valuable one: whoever picks the issue up next
starts from your evidence instead of rediscovering it. A claim left behind by a
dead session is worse than no claim — it reads as active work forever, and no
timer clears it.

## Claim the sub-issue, not the tracker

A tracker gathers children and is worked by nobody directly. Assigning yourself
to it says every child is taken, which stands the colleagues who would have
worked its siblings down. Claim the child you are actually writing, and leave
the parent alone.

## Re-derive the ruling before you execute it

The claim is settled and the issue is yours. **Open the files it names before
you build any of it**, and check the prescription against what is there now —
not just the premise, which is usually still true.

This is not caution about old issues. A ruling that is recent, specific and
confidently written is the one that costs most, because there is nothing about
it to distrust. The rulebook's *"do not refuse or narrow ordinary product
evolution because an older document disagrees"* fires on a disagreement you can
SEE, and a stale ruling does not look like one: it reads as perfectly consistent
with a tree it has not been checked against.

Three things worth asking of each item, because they fail differently:

- **Is it already done?** Somebody may have built it since, under another
  ticket or in passing. Grep for the thing before you write it.
- **Would it still be right here?** A fix copied from a neighbouring rule can be
  wrong in a way the diff cannot show. One lead-schema ruling said to copy
  `company`'s `linkedin_url` CHECK across; `company`'s matches
  `linkedin.com/company/…` while a lead's URL is an `/in/…` profile, so
  the constraint would have refused the whole column — and in review it reads as
  an obviously-correct copy of the rule beside it.
- **What else touches this?** An observation can be right and its scope wrong.
  The same ruling asked to rename one table's `source_system` to end a collision
  with `source`; six tables carry both, so renaming one leaves five and invents
  a second vocabulary.

**Fix what is actually there, and say on the issue what you did not build and
why.** A ruling that has moved is not a reason to hand the work back — most of
it is usually still worth doing, just not in the shape asked for. The write-up
is often worth more than the diff: it is what stops the next session paying to
rediscover the same four things.
