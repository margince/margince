<!-- prose:plain -->
# Cut a release

## Plan the work by month

Each milestone is a month, named `YYYY-MM` and due on its last day, such as `2026-11`. To plan an
issue, set the milestone of the month the work should land in. A milestone names no release: a
release is what `main` holds on the day it is tagged. At the end of a month, move its open issues to
the next one, then close it. Agreed work that is not for this month goes in the next month's
milestone. No label parks work for later.

## Clear the blockers

An issue the next release cannot ship without carries `release: blocker`
([issue-labels.md](../reference/issue-labels.md#release)). List what is left:

```sh
gh issue list --label "release: blocker"
```

`release-tag.yml` refuses a plain tag while that list is not empty. Close each issue, or take the label
off one the release can ship without. Then run all jobs of the run again; the tag can stay. A tag with
an added part, such as `v1.1.0-rc.1`, passes with blockers still open.

## Write the notes

The notes of a release are its GitHub release page; the repository keeps no changelog file. Before you
tag, create a draft release for the tag, by hand, with the part no tool can write:

- **Highlights**: what a user gets from this release, in a few lines.
- **Upgrade notes**: what someone who runs Margince must do or know before the upgrade. That covers a
  setting that changed, a feature taken out, and an API change that breaks a client.

```sh
gh release create v1.1.0 --draft --title "Margince v1.1.0" --notes-file notes.md
```

You can also write it on the releases page: **Draft a new release**, type the tag name, and
**Save draft**. Only those who can push see a draft.

`release-tag.yml` refuses a plain tag that has no draft with notes, before it builds anything. It
adds the generated half under your text. That half lists each `feat`, `fix` and `perf` pull request
merged since the last plain release, grouped by the start of its title. It ends with a link that
compares every commit.

A title such as `feat(api)!: …` lists under **Breaking**, and that section is never cut for length.
Other titles (`refactor`, `test`, `docs`, `ci`, dependency updates) are left to the compare link.
[`scripts/release-notes.sh`](../../scripts/release-notes.sh) builds that half. A tag with an added
part needs no draft, and its page holds only the generated half.

## Prepare the release commit

Update the `Parameters` block of `LICENSE` and nothing else in the file:

1. Set **Change Date** to the date the release goes public plus 2 years, as `YYYY-MM-DD`.
2. Add the version name to the **Licensed Work** line, for example `Licensed Work: Margince CRM v1.3.0`.

Leave every line after the `Parameters` block alone. What each value must be, and why, is in
[license-release-rule.md](../reference/license-release-rule.md). `release-tag.yml` refuses a tag
whose `LICENSE` does not carry both values, so tag on the day the Change Date counts from.

## Tag it

Then tag a commit that is on `main`, with `git tag -a`. A tag without `-a` records no date, and the
lane refuses it:

```sh
git tag -a v1.0.0 -m "First stable release"
git push origin v1.0.0
```

`release-tag.yml` then checks the tag, the draft and the blockers, and refuses a commit that
`merge-attest` judged bad. It then creates the release: your notes, the list of changes, and a link to
[deploy-margince.md](deploy-margince.md) for the install.

A plain `v1.0.0` ships. A tag with an added part, such as `v1.0.0-rc.1`, goes out as a **pre-release**.
So a build for tests does not become the default file to get on the release page. To tag a candidate
deploys nothing, because a tag moves no branch.

A run that fails before the release job leaves nothing behind. The lane runs one run at a time, and does
not cancel a run, so no run is cut off while it writes the release. Still, check the releases page before
you decide nothing happened. Either way, fix the
tree, delete the tag, and tag again:

```sh
git push --delete origin v1.0.0 && git tag -d v1.0.0
git tag -a v1.0.0 -m "..." && git push origin v1.0.0
```

It is safe to run a failed run again after the release was created. The publish step replaces the notes
on the release. Run **all** jobs again. To run only the failed jobs again leaves the job
that read the tag not run. GitHub drops the values of a job that did not run, so the publish has no
version to name anything after. It refuses, and says so.

## Two release kinds

Two kinds of release live here, and they carry different versions:

| | `release-tag.yml` | `release.yml` |
|---|---|---|
| Trigger | a `v*` tag | started by hand |
| Version | `v1.0.0` (semver) | `YYYY.edition.bugfix`: today `1970.<run>`, a stand-in pinned to the year 1970 |
| Publishes to | the GitHub release page | the dist service at `dist.test.margince.com` |
| Carries | the notes and a link to the install steps | the patch of changes, SBOMs, role images |

The version form of the dist service does not accept a `v` prefix, so the two lanes use different names.

### Where the patch of changes starts

The patch of `release.yml` starts at the patch base. The patch base is the newest published release on
the release's line, as the dist service records it. The line is `main` or one `lts/YYYY` branch. A withdrawn
release was published, so it can be the base: an installation that runs it still gets the next patch.

The release CLI reads the base from the dist service with `resolve-patch-base --version`, given the version of
the new release. It names the version and the git revision of one release. The lane cuts the patch from that
revision, and `draft-release` names that version as `--patch-base`. So the patch and its base come from one
record and cannot disagree.

A lane that was cancelled or failed published nothing. So the next patch starts at the newest published release
on the line, and it also carries the files of the lane that was skipped.

If no release was ever published on `main`, the release is the first one. Its patch is the whole tree, and
`draft-release` marks it with `--first-release`.

`scripts/release-patch-base.sh` reads the record of the release CLI. It accepts a base or the first-release mark,
and it fails on any other record. `make test-release-patch-base` runs these cases.

