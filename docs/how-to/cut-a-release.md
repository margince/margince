<!-- prose:plain -->
# Cut a release

## Plan the work by month

Each milestone is a month, named `YYYY-MM` and due on its last day, such as `2026-11`. To plan an
issue, set the milestone of the month the work should land in. A milestone names no release: a
release is what `main` holds on the day it is tagged. At the end of a month, review its open issues.
Move planned work to the next month, then close the old milestone.

Work with no planned month carries `status: backlog` and has no milestone.
Remove that label when setting a monthly milestone. Keep its `priority:` label.

## Clear the blockers

Keep the old API working. [The API change rules](../reference/api-compatibility.md) say when one may be
removed. The current CI lane does not let an operation be removed.

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
[`scripts/release_notes.py`](../../scripts/release_notes.py) builds that half. A tag with an added
part needs no draft, and its page holds only the generated half.

## Prepare the release commit

Set `VERSION` at the repository root to the version, without the tag's `v`: one line, such as `1.3.0`
for `v1.3.0`. `release-tag.yml` refuses a tag that `VERSION` does not name.

Leave `REVISION` alone. It holds `$Format:%H$`, and `git archive` writes the commit there, so a
GitHub source download names its own commit. A build reads it through `scripts/build-revision.sh`
when it has no `.git`.

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
`merge-attest` judged bad. Then it builds the release and signs it, and only then publishes the page:

1. It pushes the three images to `ghcr.io/margince`, tagged with the version.
2. It packs one server bundle for each Linux platform, with
   [`scripts/release-bundle.sh`](../../scripts/release-bundle.sh). Each bundle and the api and worker images carry
   `THIRD-PARTY-NOTICES.txt`, the license texts of the Go modules the programs link. It is written by
   [`scripts/backend-notices.sh`](../../scripts/backend-notices.sh). The pack refuses a bundle without it.
3. It writes the SBOMs, `images.txt` (each image pinned by its hash) and `SHA256SUMS` over the bundles, the SBOMs and `images.txt`.
4. It signs each image and `SHA256SUMS` with cosign.
5. It uploads those files with your notes, the changes and the install steps, then publishes the release.

The install steps link [deploy-margince.md](deploy-margince.md) for the images and
[install-on-a-server.md](install-on-a-server.md) for a bundle.

A plain `v1.0.0` ships. A tag with an added part, such as `v1.0.0-rc.1`, goes out as a **pre-release**.
So a build for tests does not become the default file to get on the release page. To tag a candidate
deploys nothing, because a tag moves no branch.

A run that fails before the build job leaves nothing new: a plain tag's draft is still only yours. Once the build job has run, the images
are on `ghcr.io` under the tag, even when a later job fails. The next run pushes over them. The lane runs one run at a time, and does
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
| Trigger | a `v*` tag | the same `v*` tag with no added part, or started by hand from `main` |
| Version | `v1.0.0` (semver) | `1.0.0`: the tag without its `v`, or typed for a run by hand |
| Publishes to | the GitHub release page | the dist service at `dist.test.margince.com` |
| Carries | the notes, the images on `ghcr.io`, a server bundle for each Linux platform, SBOMs, a signed `SHA256SUMS` | the patch of changes, SBOMs, a changelog, role images, an archive of the programs for each Linux platform |

One tag cuts both releases. The dist service accepts no `v` prefix, so the tag `v1.0.0` makes the dist
release `1.0.0`. Never push a tag `1.0.0` next to `v1.0.0`: the release CLI refuses that pair. A tag
with an added part, such as `v1.0.0-rc.1`, makes no dist release.

### Start a dist release by hand

If the run of a tag failed before its draft, start `release.yml` from `main` and type the version:

```sh
gh workflow run release.yml --ref main -f version=1.0.0
```

The version is three numbers, `x.y.z`, with no prefix, no ending and no part that starts with `0`.
Before the lane drafts, [`scripts/release-dist-version.sh`](../../scripts/release-dist-version.sh)
reads the tag through `scripts/release-tag-version.sh`, or checks the typed version and the branch
(`make test-release-dist-version`).

The dist service has one line of releases. A new version must be above every published release,
withdrawn ones too. The release CLI refuses a lower version before the draft.

### What the archive of a platform holds

For `linux-amd64` and `linux-arm64` the archive holds the server bundle of that platform, without its
top folder. The lane packs it with [`scripts/release-bundle.sh`](../../scripts/release-bundle.sh) from
the same `bundle` bake as `release-tag.yml`, so every path matches the bundle. The programs are in
`bin/`, and `VERSION` names the dist version.

Before the upload, the lane runs `bin/margince-<role> --version` for the `linux-arm64` tree. Each program
must print `margince-<role> <version> (revision <commit>)`, with the version of the dist release.

`build-archive` of the release CLI turns each folder into an archive and three SBOM files.
`add-artifacts` uploads both platforms, with the role images, in one call. The dist service refuses a
platform that comes in parts.

The role images are pushed with no version tag. The release names each image by its hash, as
`<role>=<repository>@sha256:<hash>`.

### Where the patch of changes starts

The patch of `release.yml` starts at the patch base. The patch base is the newest published release,
as the dist service records it. A withdrawn release was published, so it can be the base: an installation that
runs it still gets the next patch.

The release CLI reads the base from the dist service with `resolve-patch-base --release`, given the version of
the new release. It names the version and the git revision of one release. The lane cuts the patch from that
revision, and `draft-release` names that version as `--patch-base`. So the patch and its base come from one
record and cannot disagree. The publisher uses `--release <version>` for
`draft-release`, `add-artifacts` and `publish-release`.

A lane that was cancelled or failed published nothing. So the next patch starts at the newest published
release, and it also carries the files of the lane that was skipped.

If no release was ever published, the release is the first one. Its patch is the whole tree, and
`draft-release` marks it with `--first-release`.

`scripts/release-patch-base.sh` reads the record of the release CLI. It accepts a base or the first-release mark,
and it fails on any other record. `make test-release-patch-base` runs these cases.

### The changelog of a dist release

The dist service publishes no release without a changelog. The lane writes it from the subject of every commit
since the patch base, so the changelog lists what the patch carries. Merge commits are left out. A change that
reached `main` through another branch keeps its own subject. If a lane was skipped, the next changelog lists its
changes too.

[`scripts/release_changelog.py`](../../scripts/release_changelog.py) writes the file. Under the heading
**Changes** come the sections of [`scripts/release_notes.py`](../../scripts/release_notes.py), and then a link
that compares every commit since the base. The dist page shows the version above the file, so the file does not
name it. A release with no subject a user notices gets only the link. The first release gets one sentence: its
patch is the whole tree.

The script drops a byte that is not valid UTF-8 from a subject. The history of `main` does not change, so one
bad subject would otherwise stop every later release. The lane fails before the draft when no commit is new since
the base, or when the **Breaking** subjects alone are too long. The release CLI also refuses a file that is empty
or over its size limit.
`make test-release-changelog` runs these cases.

