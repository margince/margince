<!-- prose:plain -->
# Cut a release

First, set the date on the first release heading of `CHANGELOG.md` to today, and merge that change. The
heading is written when the section fills up, so its date is a guess until you correct it. The date is
part of the released commit, so it goes in before the tag.

Then tag a commit that is on `main`:

```sh
git tag -a v0.0.1 -m "Zalo OA inbound, desktop build info"
git push origin v0.0.1
```

`release-tag.yml` then checks the tag, and refuses a commit that `merge-attest` judged bad. It builds the
macOS and Windows bundles, and creates the release with both bundles on it.

A plain `v0.0.1` ships. A tag with an added part, such as `v0.0.1-rc.1`, goes out as a **pre-release**.
So a build for tests does not become the default file to get on the release page. To tag a candidate
deploys nothing, because a tag moves no branch.

A run that fails before the release job leaves nothing behind. A run that ends early in that job can
leave a release with only some of its files. For that reason the lane runs one run at a time, and does
not cancel a run. So check the releases page before you decide nothing happened. Either way, fix the
tree, delete the tag, and tag again:

```sh
git push --delete origin v0.0.1 && git tag -d v0.0.1
git tag -a v0.0.1 -m "..." && git push origin v0.0.1
```

It is safe to run a failed run again after the release was created. The publish step replaces the files
and the notes on the release. Run **all** jobs again. To run only the failed jobs again leaves the job
that read the tag not run. GitHub drops the values of a job that did not run, so the publish has no
version to name anything after. It refuses, and says so.

## Two release kinds

Two kinds of release live here, and they carry different versions:

| | `release-tag.yml` | `release.yml` |
|---|---|---|
| Trigger | a `v*` tag | started by hand |
| Version | `v0.0.1` (semver) | `YYYY.edition.bugfix`: today `1970.<run>`, a stand-in pinned to the year 1970 |
| Publishes to | the GitHub release page | the dist service at `dist.test.margince.com` |
| Carries | both desktop bundles | the patch of changes, SBOMs, role images |

The version form of the dist service does not accept a `v` prefix, so the two lanes use different names.

### Where the patch of changes starts

The patch of `release.yml` starts at the last commit that **was published**. The lane records it as the
moving `released` tag once `publish-release` succeeds. The lane does not use the old tip of the branch.

The two agree only while every lane publishes. A run can be cancelled, or it can fail. (The release group
holds one place in the queue, so a merge pushes out the run behind it.) Either way, no one publishes that
commit. Say the next patch started at the old tip. Then it would drop the files of that commit from every
patch a user applies, with no sign that anything failed.

That has two results:

- the first patch after a skipped lane is wider than usual, and covers what that lane dropped;
- a `released` tag that failed to move makes the next patch wider still, which a user applies with no
  harm. The tag points at the dist service's own record, and is written only after a publish succeeds. So
  when it fails, it fails on the safe side.

`scripts/release-patch-base.sh` states the rule, and what it falls back to. With no `released` tag, the
base is the old tip of the push. With no old tip either (a run started by hand, or a new branch), it is
`HEAD~1`. Then the patch of that release covers one commit. Only a checkout where none of these exists
makes no patch. `make test-release-patch-base` runs the steps publish, skip, publish.

## To build a bundle without releasing anything

Start a desktop lane directly. Each one takes a `ref`, and uploads its bundle as a file of the run. The
Windows lane uploads the folder itself, and the macOS lane uploads a `tar` file. That is because an upload
of a run file drops the mark that lets a file run, and `tar` keeps it:

```sh
gh workflow run desktop-macos.yml --ref main
```

A build you start this way names itself after the commit, because no lane gave it a version.
