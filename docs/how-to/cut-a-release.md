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

## Refresh the policy packs

A messaging policy pack compiles into the build. So the commit you tag decides which legal text an
installation names beside a warning. Compare the packs with their sources before you tag:

```sh
make policy-refresh
```

The command fetches the source of each act a pack names, and takes the text out of the page. It
compares the hash of that text with the one the layer records, and prints one line for each act:

| Line | What to do |
|---|---|
| `unchanged` | Nothing. |
| `first digest` | Nothing. The layer records its first hash in this run. |
| `changed` | Read the source. Where the act changed, update the layer as [Change a rule](update-a-messaging-policy-pack.md#change-a-rule) says. |
| `not read` | The host refused, or the page loads its text with a script. Give the act a `text-url`, or read the source and set `checked-by-hand`. |
| `by hand` | Read the source again, and set `checked-by-hand` to the day you did. |

The command then writes each hash and `source-checked` into the layers, and builds each unit's
`messagingrules_gen.go` again. Open one pull request with those files and merge it.
`make policy-refresh REPORT=1` prints the same lines and writes nothing. The keys are in
[Sources and the refresh](update-a-messaging-policy-pack.md#sources-and-the-refresh).

`release-tag.yml` refuses a plain tag when the `source-checked` of a layer is more than 45 days
before the day of the tag. The check is
[`scripts/release-policy-sources-fresh.sh`](../../scripts/release-policy-sources-fresh.sh)
(`make test-release-policy-sources-fresh`). It reads the dates in the tree and fetches nothing, so
one commit gets one answer. A layer with no `source-checked` is not checked, so the rule holds a
layer from its first refresh on. A tag with an added part reports an old layer and passes.

## Tag it

Then tag a commit that is on `main`, with `git tag -a`. A tag without `-a` records no date, and the
lane refuses it:

```sh
git tag -a v1.0.0 -m "First stable release"
git push origin v1.0.0
```

`release-tag.yml` then checks the tag, the draft and the blockers, and refuses a commit that
`merge-attest` judged bad. Then it builds the release and signs it, and only then publishes the page:

1. It pushes the three images to `ghcr.io/margince`, tagged with the version without its `v`, such as
   `1.0.0`. The programs report that version too, as `VERSION` does.
2. It packs one server bundle for each Linux platform, with
   [`scripts/release-bundle.sh`](../../scripts/release-bundle.sh). Each bundle and the api and worker images carry
   `THIRD-PARTY-NOTICES.txt`, the license texts of the Go modules the programs link. It is written by
   [`scripts/backend-notices.sh`](../../scripts/backend-notices.sh). The pack refuses a bundle without it.
3. It writes the SBOMs, `images.txt` (each image pinned by its hash) and `SHA256SUMS` over the bundles, the SBOMs and `images.txt`.
   One SBOM job runs beside the build, and the dist release takes the same files.
4. It signs each image and `SHA256SUMS` with cosign.
5. It uploads those files with your notes, the changes and the install steps, then publishes the release.

The install steps link [deploy-margince.md](deploy-margince.md) for the images and
[install-on-a-server.md](install-on-a-server.md) for a bundle.

A plain `v1.0.0` ships. A tag with an added part, such as `v1.0.0-rc.1`, goes out as a **pre-release**.
So a build for tests does not become the default file to get on the release page. To tag a candidate
deploys nothing, because a tag moves no branch.

### When a run fails

A run that fails before the build job leaves nothing new: a plain tag's draft is still only yours. Once the build job has run, the images
are on `ghcr.io` under the tag, even when a later job fails. The next run pushes over them. The lane runs one run at a time, and does
not cancel a run, so no run is cut off while it writes the release. Still, check the releases page before
you decide nothing happened. Either way, fix the
tree, delete the tag, and tag again:

```sh
git push --delete origin v1.0.0 && git tag -d v1.0.0
git tag -a v1.0.0 -m "..." && git push origin v1.0.0
```

A tag can move only while Go has not recorded it. The tag is also a version of the Go module
`github.com/margince/margince`. When a client first asks the Go module proxy for that version, the proxy
records the commit of the tag. The Go checksum database records its files. Neither changes again.

After that, a moved tag makes a release with other source than the Go version of the same name. The proxy serves the old
commit, and a download from git fails Go's checksum check.

So both release lanes ask the proxy first, with
[`scripts/release-tag-go-proxy.sh`](../../scripts/release-tag-go-proxy.sh) (`make test-release-tag-go-proxy`).
The question reads only what the proxy already holds, so the check does not record the version itself. If Go
recorded the version at another commit, the run refuses. Leave the moved tag alone in that case, and tag the next
version, such as `v1.0.1`, with `VERSION` and `LICENSE` set to it.

It is safe to run a failed run again after the release was created. The publish step replaces the notes
on the release. Run **all** jobs again. To run only the failed jobs again leaves the job
that read the tag not run. GitHub drops the values of a job that did not run, so the publish has no
version to name anything after. It refuses, and says so.

All jobs include the test dist release. If the first run published it, the service refuses that version a
second time, and the dist jobs fail. The GitHub release does not wait for them, so it still goes out.

## Two release kinds

Two kinds of release live here, and they carry different versions:

| | GitHub release (`release-tag.yml`) | dist release (`_release-dist.yml`) |
|---|---|---|
| Trigger | a `v*` tag | the same plain `vX.Y.Z` tag, or `release.yml` started by hand with that tag |
| Version | `v1.0.0` (semver) | `1.0.0`: the tag without its `v` |
| Publishes to | the GitHub release page | `dist.test.margince.com` on a tag, or by hand with the default `target=test`; `dist.margince.com` only by hand with `target=production` |
| Carries | the notes, the images on `ghcr.io`, a server bundle for each Linux platform, SBOMs, a signed `SHA256SUMS` | the patch of changes, SBOMs, a changelog, role images, an archive of the programs for each Linux platform |

One tag cuts both releases in one run of `release-tag.yml`, from one set of SBOMs. Neither release waits for
the checks of the other: an open blocker holds only the GitHub release. The dist service accepts no `v` prefix, so the tag `v1.0.0` makes the dist
release `1.0.0`. Never push a tag `1.0.0` next to `v1.0.0`: the release CLI refuses that pair. A tag
with an added part, such as `v1.0.0-rc.1`, makes no dist release.

### Start a dist release by hand

A tag publishes to the test service only. Once the test release works, publish the same tag to production:

```sh
gh workflow run release.yml --ref main -f tag=v1.0.0 -f target=production
```

If the run of a tag failed before its draft, start it again with `-f target=test`. Each target has its own
publisher tokens. Test uses the repository secrets `MARGINCE_DIST_PUBLISHER_TOKEN` and
`MARGINCE_AUTH_PUBLISHER_TOKEN`. Production uses `MARGINCE_PRODUCTION_DIST_PUBLISHER_TOKEN` and
`MARGINCE_PRODUCTION_AUTH_PUBLISHER_TOKEN`, set only in the `dist-production` environment, so no test run gets
them. A run whose tokens are missing stops before it drafts, and never uses the tokens of the other target.

The run builds the commit the tag names, never the tip of `main`, so the dist release describes the
same source as the GitHub release. `--ref main` only picks the copy of the workflow that runs.

Before the lane drafts, it refuses a tag that is not on `main`, as the GitHub release does. The check is
[`scripts/release-tag-on-main.sh`](../../scripts/release-tag-on-main.sh) (`make test-release-tag-on-main`).
It also refuses a tag that Go recorded at another commit, as the GitHub release does
([When a run fails](#when-a-run-fails)).
Then [`scripts/release-dist-version.sh`](../../scripts/release-dist-version.sh) reads the version from the tag
(`make test-release-dist-version`). The tag is `v` and three numbers,
with no ending. A number may be `0`, but no number has a leading zero, such as `01`.

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

The patch of a dist release starts at the patch base. The patch base is the newest published release,
as the dist service records it. A withdrawn release was published, so it can be the base: an installation that
runs it still gets the next patch.

The release CLI reads the base from the dist service with `resolve-patch-base --release`, given the version of
the new release. It names the version and the git revision of one release. The lane cuts the patch from that
revision, and `draft-release` names that version as `--patch-base`. So the patch and its base come from one
record and cannot disagree. The publisher uses `--release <version>` for
`draft-release`, `add-artifacts` and `publish-release`.

A lane that was cancelled or failed published nothing. So the next patch starts at the newest published
release. It carries the changes of the skipped release that are still in the next one. A file that the skipped
release added and a later commit removed is in neither tree, so no patch carries it.

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

