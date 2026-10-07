<!-- prose:plain -->
# Work on an issue

Before you work on an issue, check that no one holds it, and that its decision still
matches the tree. The last section covers the decision.

A comment such as "Taking this one" does not claim an issue. It sits in a thread
that no one opens, while the issue list still shows the row as free. The claim is **an assignee and
`status: in progress`**, because the list shows these, and a search can
find them.

## Before you start

```sh
gh issue view <n> --json assignees,labels,closedByPullRequestsReferences
```

Every sign below is about someone **else**, so first learn who you are:
`gh api user -q .login`. That is not always the user of the last session on this
machine. The issue is **taken** if one of these is true:

- an assignee who is not you;
- the label `status: in progress` while the assignee is not you. With no
  assignee at all, it is still taken, and there is no one to name. Say so,
  because no one can ask about a claim with no name on it;
- an open pull request under `closedByPullRequestsReferences` that someone
  else wrote. That list has the number, but not the name of who wrote it. It also keeps merged
  and closed pull requests, so read the one it names:

  ```sh
  gh pr view <pr> --json state,author -q '.state + " " + .author.login'
  ```

If no item is true, the issue is free. **An issue where every sign points at you is
yours to go on with**: no new claim, no comment; go on with the work. An issue that is
only old is free too. A claim has no end date here, and nothing takes a claim over
on its own.

A **reopened** issue keeps the assignee who closed it, but not the label. The
assignee is the record of who worked on it, and does not claim what is still to do. The first item
still applies, so ask them first. If they are not back on it, claim it again.

## It is taken

Do not work on it. Say so, in this order, to the user who asked you:

1. **Name who holds it**: the login of the assignee, or the number of the open pull
   request that closes it.
2. **Send questions to them.** Questions about scope, and questions that cannot wait, go to the one
   who already holds it. That costs a message, where a second diff costs a day.
3. **Offer another issue, if there is one.** Offer a free issue close by,
   and do not hand back an empty answer.

```sh
gh issue list --state open --label "area: <x>" \
  --search 'no:assignee -label:"status: in progress"' --limit 10
```

Take the `area:` from the issue you started from, so what you offer is work
of the same shape. Add `--label "priority: high"` for a shorter list.

## Claim it

```sh
gh issue edit <n> --add-assignee @me --add-label "status: in progress"
```

Then read it again:

```sh
gh issue view <n> --json assignees -q '[.assignees[].login]'
```

**If someone else is now an assignee** after you wrote, your claim is
second and you step back. Remove yourself, say who claimed it first, and
choose other work. A check and then a claim are not one step, so two
sessions can both pass the check. After a take over, the count is one name more. The first
assignee stays there, and only a *third* login means someone claimed it before you.

## Take one over when someone holds it

A claim tells; it does not stop other users. The user who asked you may know that someone holds the issue,
and still want you to work on it. That is their call, and you work on it.

**Comment first, change the assignee second.** The comment says who takes it over and
why. It goes on before the assignee changes, so the session you take it from
finds a reason, and not a change with no word:

```sh
gh issue comment <n> --body "Taking this over at <who asked>'s request: <why>."
gh issue edit <n> --add-assignee @me --add-label "status: in progress"
```

Leave the first assignee in place, unless you know they stopped. Two
assignees who know about each other are a good state. One who learns of it later
is not.

## Release it when you stop

To finish takes no step from you. GitHub closes the issue when a pull request whose
body says `Closes #N` merges, but it keeps the labels. So
[`issue-closed.yml`](../../.github/workflows/issue-closed.yml) takes
`status: in progress` off an issue as soon as it closes.

Some closes send no event,
such as one a workflow makes with its own token, and a run can fail. The next daily
sweep finds these, but not always, because GitHub can start a planned run
later, or drop it. The assignee stays, as the record of who worked on the
issue. To close an issue by hand also takes no step from you; the workflow owns the
label.

**A `status: in progress` on a closed issue** is always out of date, so take it off. On
an open issue, the rules under [Before you start](#before-you-start) apply
with no change.

To stop **without** finishing is the case that needs you. Take yourself off as assignee, and
always say where you stopped:

```sh
gh issue edit <n> --remove-assignee @me
gh issue comment <n> --body "Stopped here: <done, not done, what I learned>."
```

**The label comes off** only when no one holds the issue. `--remove-assignee @me`
drops only your login, while `--remove-label` drops a sign that the whole
issue shares. A take over can leave two names on an issue. If you take both off in
one command, you remove the claim of the first assignee. That assignee may still work on it, and
the issue looks free. Look first, and remove the label only when the list is empty:

```sh
gh issue view <n> --json assignees -q '[.assignees[].login]'
gh issue edit <n> --remove-label "status: in progress"
```

The comment counts most. The next user to take the issue starts from your
evidence, and does not have to find it again. A claim from a session that ended
looks like open work for all time, and no clock takes it off.

## Claim the sub-issue, not the tracker

A tracker holds sub-issues, and no one works on the tracker itself. When you put yourself
on it as assignee, it says that every sub-issue is taken. That keeps colleagues off the
other sub-issues. Claim the sub-issue you write, and leave
the tracker alone.

## Check the decision again before you build it

The claim is in place and the issue is yours. **Open the files it names before
you build any of it.** Check what it tells you to do against what is there now,
and check its reason too, which is still true in most cases.

New and exact decisions go out of date too.

Ask these of each item, because each one fails in a different way:

- **Is it already built?** Someone may have built it already, under another
  issue or as part of other work. Search for the thing before you write it.
- **Would it still be right here?** A fix copied from a rule close by can be
  wrong in a way the diff cannot show. The `linkedin_url` CHECK of `company` matches
  `linkedin.com/company/…`, while the URL of a lead is an `/in/…` profile. So
  a copy of that CHECK on leads would refuse the whole column. In review, it looks
  like a right copy of the rule next to it.
- **What else touches this?** A finding can be right and its scope wrong.
  Say you rename `source_system` in one table, so that it does not share a name with `source`.
  That leaves every other table that has both, and makes a second vocabulary.

**Fix what is there**, and say on the issue what you leave out, and why. A
decision that has changed is no reason to hand the work back. Most of it is still
worth doing, in many cases in a different shape. In many cases the note is worth more than
the diff, because it saves the next session the cost of finding it all again.
