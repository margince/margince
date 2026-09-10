# Cut a release

First, set the date on `CHANGELOG.md`'s top release heading to the day you are
cutting, and land that: the heading is written when the section fills up, not
when the tag is pushed, so its date is a guess until you correct it — and it
sits in the commit the release describes, which is why it goes in before the
tag rather than after.

Then tag a commit that is on `main`:

```sh
git tag -a v0.0.1 -m "Zalo OA inbound, desktop build info"
git push origin v0.0.1
```

That is the whole gesture. `release-tag.yml` then validates the tag, refuses a
commit `merge-attest` judged adverse, builds the macOS and Windows bundles, and
creates the release with both attached.

A plain `v0.0.1` ships. A suffixed `v0.0.1-rc.1` publishes as a **pre-release**,
so a build meant for testing does not become the download the release page
offers by default. Tagging a candidate deploys nothing in any case — a tag moves
no branch — and the D13 deployment's production promotion is meant to select
non-suffixed tags on that same answer once it is built.

A run that fails before the release job leaves nothing behind. A run killed
during it can leave a release holding only some of its assets — the lane
serialises rather than cancels for exactly that reason — so check the
releases page rather than assuming nothing happened. Either way: fix the
tree, delete the tag, and tag again:

```sh
git push --delete origin v0.0.1 && git tag -d v0.0.1
```

Re-running a failed run after the release was created is safe: the publish step
replaces the assets and the notes on the existing release rather than failing.
Re-run **all** jobs, not only the failed ones. A re-run of the failed jobs alone
leaves the job that read the tag un-executed, and GitHub drops the outputs of a
job that did not run, so the publish has no version to name anything after — it
refuses, and says so.

## This is not the constellation release

Two release kinds live here, and they carry different versions:

| | `release-tag.yml` | `release.yml` |
|---|---|---|
| Trigger | a `v*` tag | manual dispatch |
| Version | `v0.0.1` (semver) | `YYYY.edition.bugfix` — today `1970.<run>`, the epoch-pinned placeholder |
| Publishes to | the GitHub release page | the dist service at `dist.test.margince.com` |
| Carries | both desktop bundles | the incremental patch, SBOMs, role images |

The dist service's version grammar (`pkg/version` in
`gradionhq/margince-constellation`) rejects a `v`-prefixed string by
construction, and its scheme is a product commitment — evergreen editions, LTS
always `YYYY.0` from 2028, a 24-month support window. So one build cannot be
named both ways, and neither lane tries.

## To build a bundle without releasing anything

Dispatch a desktop lane directly. Each takes a `ref` and uploads its bundle
as a run artifact — the Windows lane the folder itself, the macOS lane a
tarball, because an artifact upload drops the executable bit and tar
preserves it:

```sh
gh workflow run desktop-macos.yml --ref main
```

A dispatched build names itself after the commit, because no lane gave it a
version.
