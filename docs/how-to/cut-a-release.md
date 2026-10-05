# Cut a release

First, set the date on `CHANGELOG.md`'s top release heading to today and merge
that change. The heading is written when the section fills up, so its date is a
guess until you correct it. The date sits in the released commit, so it goes in
before the tag.

Then tag a commit that is on `main`:

```sh
git tag -a v0.0.1 -m "Zalo OA inbound, desktop build info"
git push origin v0.0.1
```

`release-tag.yml` then validates the tag, refuses a commit `merge-attest` judged
adverse, builds the macOS and Windows bundles, and creates the release with both
attached.

A plain `v0.0.1` ships. A suffixed `v0.0.1-rc.1` publishes as a **pre-release**,
so a build meant for testing does not become the release page's default
download. Tagging a candidate deploys nothing, because a tag moves no branch.

A run that fails before the release job leaves nothing behind. A run killed
during it can leave a release holding only some of its assets (the lane
serialises runs instead of cancelling them for that reason), so check the
releases page before assuming nothing happened. Either way, fix the tree,
delete the tag, and tag again:

```sh
git push --delete origin v0.0.1 && git tag -d v0.0.1
git tag -a v0.0.1 -m "..." && git push origin v0.0.1
```

Re-running a failed run after the release was created is safe: the publish step
replaces the assets and the notes on the existing release. Re-run **all** jobs.
A re-run of the failed jobs alone leaves the job that read the tag un-executed.
GitHub drops the outputs of a job that did not run, so the publish has no
version to name anything after; it refuses and says so.

## Two release kinds

Two release kinds live here, and they carry different versions:

| | `release-tag.yml` | `release.yml` |
|---|---|---|
| Trigger | a `v*` tag | manual dispatch |
| Version | `v0.0.1` (semver) | `YYYY.edition.bugfix`: today `1970.<run>`, the epoch-pinned placeholder |
| Publishes to | the GitHub release page | the dist service at `dist.test.margince.com` |
| Carries | both desktop bundles | the incremental patch, SBOMs, role images |

The dist service's version scheme does not accept a `v` prefix, so the two lanes
use different names.

### What the incremental patch is cut from

`release.yml`'s patch starts at the last revision that **published**. The lane
records it as the moving `released` tag once `publish-release` succeeds. The
ref's previous tip is not used.

The two agree only while every lane publishes. A run can be cancelled (the
release group holds one queued slot, so a merge evicts the run behind it) or can
fail. Either leaves a commit published by nobody. Basing the next patch on the
previous tip would drop that commit's files from every patch a consumer applies,
with no sign that anything went wrong.

Two consequences:

- the first patch after a skipped lane is wider than usual, covering what that
  lane dropped;
- a `released` tag that failed to move makes the next patch wider still, which a
  consumer applies without harm. The tag points at the dist service's own
  record and is written only after a success, so it fails in the safe direction.

`scripts/release-patch-base.sh` states the rule and its fallbacks (the first
release of a repository has nothing behind it and draws no patch at all).
`make test-release-patch-base` walks the publish/skip/publish sequence.

## To build a bundle without releasing anything

Dispatch a desktop lane directly. Each takes a `ref` and uploads its bundle as a
run artifact: the Windows lane uploads the folder itself, the macOS lane a
tarball, because an artifact upload drops the executable bit and tar preserves
it:

```sh
gh workflow run desktop-macos.yml --ref main
```

A dispatched build names itself after the commit, because no lane gave it a
version.
