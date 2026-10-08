<!-- prose:plain -->
# Claim a red `main`

`main` goes red here often, and that is by design. To merge past a red required check is a standing
decision. So a break reaches the newest commit, and every open pull request gets it through its merge commit. With
many sessions running at once, several of them see the same failure within minutes, and all of them
reach for the same fix.

A claim stops that waste. To find the cause of a red lane costs a session all of its context. The
second and third sessions to do it make nothing but copies of the same pull request. All the while,
the first session lands its fix in any case.

The claim is a **draft pull request** with the label `claim: main-red`. One command finds it, and it
exists before the fix does.

## Before you look into a failure that is not yours

```sh
gh pr list --state open --label "claim: main-red" \
  --json number,title,body,updatedAt,author
```

Read the bodies, not only the titles. Each claim lists the lanes and tests it covers, and you match your
failure against that list. The answer to "is `main` red?" is almost always that it is, so it tells you nothing.

**If a claim covers your failure**, stop. Do not look into it, and do not open a second fix. Do not keep
checking it either: to wait spends the tokens a claim saves. Your own pull request is red for a reason
that is not yours. So carry on with your own work, and look again when you are ready to merge.

**If no claim covers it**, you are first for this cause, even when another claim is open for a different
one. `main` is often red for two or more causes at once that have nothing to do with each other. A claim
that names one of them says nothing about the rest.

## Opening a claim

Open the claim **before** you start to look for the cause. Its value is in the minutes it saves the next
session.

```sh
git switch -c red/<lane-or-test-slug> origin/main
git commit --allow-empty -m "claim: <lane> is red on main"
git push -u origin red/<lane-or-test-slug>
gh pr create --draft --label "claim: main-red" \
  --title "main is red: <lane>" \
  --body "$(printf 'Claiming:\n- TestOne\n- TestTwo\n\nCause: unknown so far.\n')"
```

**A pull request with no diff is allowed.** GitHub asks only that the branch has a commit the base
does not have. The empty commit gives it one, so the claim opens before you write a line of the fix. Do
not wait until you have a fix to show: by then the second session has already started.

A draft runs almost no CI, and gets no CodeRabbit review. `ci.yml` runs the `changes` job only on
`draft == false`, so every lane that waits on its answer skips. And `.coderabbit.yaml` sets `drafts: false`.
The draft state keeps it cheap whatever the diff holds, so push your work to it as you go. It costs CI
only once you mark it ready.

If `main-health` has already filed a `main is red:` issue, assign it to you at the same time. An
issue with no one assigned reads as not claimed. And the issue is where a reader who does not watch pull
requests will look.

## Keep the body current

**The body is what others read.** Another session reads your list to decide whether to stand down, so keep
it current. Add a test when the failure covers more tests than you expected, and say so if it covers
fewer. A claim that covers less than it lists leaves a real failure that no one owns.

**Say what you learn.** A comment that names the cause, or names what you ruled out, saves the next
session from finding it again. It helps more than the diff.

## Releasing it

- **Fixed**: mark the pull request ready and merge it. The claim goes with it.
- **Dropped**: close the draft, and say in a comment what you learned, so the next session starts from your
  evidence.
- **Not a real failure**: close it, and say why the verdict was wrong. A claim still open over a green
  `main` stops someone from looking at a real failure later.
- **Someone else fixed the cause**: close it, and name the pull request that fixed it. A merge releases
  only a claim that its own change fixed. A claim that another change fixed is still open. Then a fixed
  problem sits in the list that the next session reads first.

## When two sessions claim the same thing

To list the open claims and then create one is not one step. Two sessions that reach the same red
within the same few seconds both find nothing, and both open a claim.

**The smaller pull request number wins.** It existed first, every session can see it, and no clock has to
agree. If yours is not the smaller number, and it covers some of the same tests:

- close yours, with a comment that points at the winner;
- write what you learned on the claim of the winner before you close yours;
- carry on with your own work.

Count it per test. Your claim may cover three tests, and only one of them is also on the list of the
winner. Then drop that one from your body and keep going. The other two are still not claimed, and to
drop them would leave them with no owner.

## Taking over a stale claim

A session can stop and never come back, and a claim it leaves behind would block the repository for
good. When a claim has no commit and no comment for **90 minutes**, another session may take it over.
Say so first in a comment on it, so the first session sees what happened if it comes back. Then carry on in your own branch.

The limit is 90 minutes, because to find the cause of a red integration lane takes that long. A limit
short enough to find a session that stopped soon would also take work from a live one.

## When several causes are open at once

Each cause gets its own claim, and they land in whatever order they are ready. Required checks run
against the merge commit. So two pull requests that each fix part of a red `main` are **both red**.
Neither can go green while the other is not merged, and the branch rules refuse both.

The way out is one branch that merges both fixes. Its merge commit holds the whole repair, so it is green,
and no admin has to skip the rules.
