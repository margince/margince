<!-- prose:plain -->
<!-- describes: .github/ci-image .github/actions/ci-image .github/workflows/lint-cache.yml scripts/ci-image-context.sh scripts/ci-image-digest.sh scripts/gate-tools-recipe.sh scripts/check-image-pins.sh -->
# The CI image

Both parts of `deterministic-gates` and of `backend-lint`, the jobs of the frontend lane and `uat` run
in one container image, `ghcr.io/margince/ci`. It holds Go, the gate binaries, Node with pnpm, the Chromium build
Playwright starts, and the Ubuntu packages Chromium needs. So these jobs install no toolchain, and
`uat` does not reach a package mirror. A mirror that stopped answering during a font fetch has
stopped a UAT shard before a test ran.

The rest of the merge gate is in [ci-pipeline.md](ci-pipeline.md).

## Every byte comes from a pin

[`.github/ci-image/Dockerfile`](../../.github/ci-image/Dockerfile) builds the image, and each input
has a pin in this tree:

| Input | Pin | What moves it |
|---|---|---|
| Ubuntu, Go and Node base images | tag and `@sha256` digest | Renovate, through its `dockerfile` manager. The `golang` image moves in the `go toolchain` group, with every other Go pin |
| Ubuntu packages | one time on `snapshot.ubuntu.com`, `ARG UBUNTU_SNAPSHOT`, and each package as `name=version` | the `Ubuntu` group in Renovate, once a week: the time and every package in one pull request |
| Gate binaries | the recipe `make -n tools` prints, so the versions stay in `backend/Makefile` | a change to that recipe |
| pnpm | `packageManager` in the root `package.json`, with its hash | the same change that moves pnpm for every other reader |
| Chromium | `PLAYWRIGHT_VERSION`, the browser build and its sha256, as `ARG` lines | a maintainer, when Playwright moves (below) |

The image is Ubuntu 24.04, read at one point in the history of the archive. So the same time and
pins give the same packages on every build, and `apt-get upgrade` patches the base image the same way
each time. The image replaces its own sources, which name the live archive, with the snapshot.

`snapshot.ubuntu.com` answers only over HTTPS, and the base image has no CA bundle until
`ca-certificates` is in. So `apt` starts on the bundle from the pinned Go image. It checks each index
against the Ubuntu archive key either way. A fetch tries three times with a 30 second limit, so a
dead mirror fails the build in minutes. `apt-get update --error-on=any` fails on an index that never
came. Without it, `apt` only warns, and every pin then fails as a package it cannot find.

Renovate reads two sources from Launchpad, which records what Ubuntu publishes. One is the newest
time the 24.04 archive changed, which is the snapshot time. The other is the versions of each
package published for 24.04, with their dates. The time and the versions move in one branch, because
`apt` cannot find a version newer than the time. The versions keep the wait of seven days. The time
has no date, so it never waits, and it is never older than the versions beside it.

The gate binaries are built with no C toolchain, so they do not need the newer C library of the Go
image. The Go and Node builds the image copies in are the upstream ones, which run on 24.04.

[`backend/gates/ciimagepins_test.go`](../../backend/gates/ciimagepins_test.go) holds the file to
these rules:

- every base image and every `COPY --from` image has a digest;
- `apt` reads only the snapshot, and every package has a version;
- every fetch over the network has a checksum that the build checks;
- nothing turns off the checksum database of Go or the signature check of Corepack;
- the managers in `renovate.json` find every pin.

Each rule has a case that breaks the file, and the case must fail.

Playwright gives no checksum for its browser. When the lockfile moves Playwright, the same gate fails.
It names the four `ARG` lines to set from the `browsers.json` of that release, and the sha256 of the
`chrome-headless-shell` archive.

## How a job finds the image

The tag is the sha256 of the build context, which
[`scripts/ci-image-context.sh`](../../scripts/ci-image-context.sh) writes: the Dockerfile, the tools
recipe and the pnpm pin. So the tag changes when an input does, and on no other change. There is no
rebuild on a schedule.

The `ci-image` job runs beside `changes`.
[`scripts/ci-image-digest.sh`](../../scripts/ci-image-digest.sh) asks the registry for the digest of
that tag, with no login. The `container:` of a job pulls with no login too, so the job asks the same
way.

- **Found.** The job hands on `ghcr.io/margince/ci@sha256:<digest>`, and each job above sets that as
  its `container:`.
- **Missing, and the run can push.** The job builds the image, pushes it under the tag, and hands on
  its digest. So a Renovate pull request runs on the image it brings.
- **Missing, and the run cannot push.** A pull request from a fork gets a token that can only read.
  The job hands on an empty name, with a warning.

These runs can push: a pull request from a branch of this repository, a merge queue run, and a run
started by hand. The `ci-image` jobs of `cache-warm.yml` and `lint-cache.yml` push on `main`.
While the package needs a login to pull, every such run builds again.

An empty name runs each job on the runner. The steps marked
`if: needs.ci-image.outputs.pinned-image == ''` then install what the image would hold: Go, the gate
binaries, pnpm and Chromium. In the image, setup-go and setup-node get no version, use the tools in
the image, and only restore their caches. An image that needs a login to pull gives the same empty
name. So `ci` is correct with or without the image, and only slower without it.

`scripts/check-image-pins.sh` admits that one job output as a `container:`, read as it is or behind a
condition that falls back to the empty string. A lane may take it as a `ci-image` input. That input is
declared for `workflow_call` with no default but the empty one. A caller passes the job output, or
nothing, as `main-health.yml` does, and then the lane runs on the runner. Each `pinned-image` output
must come from the `ci-image` action.
The action refuses to hand on a name that does not end in a digest.

The script also scans `.github/actions/`. Its `./path` allowance lets a local action through, because
the repo versions its own code. That holds for the action's own ref, but not for the outside actions
it calls, which would come in with nobody reading them.

## Its own caches

Go keys a build on the folder it read, and golangci keys its cache on the path of the checkout. In
the image both differ from a bare runner, so an entry from one misses in the other. The image sets
`MARGINCE_CI_IMAGE`, and `go-build-cache` and `golangci-cache-key` add `in-image-` to their keys when
it is set. `cache-warm.yml` warms both: its `plain` job runs on the runner and, when the image can be
pulled, again in it. `lint-cache.yml` lints in the image when it can, so its golangci entry is the one
`backend-lint` reads.

## Which jobs stay on the runner

- The integration shards and `live-boot` start Docker Compose, which needs the Docker of the runner.
- `craftsmanship`, `craft-residue`, `secret-scan`, `license gate` and `sonarcloud` install no
  toolchain, or only Go for one small module. A pull would cost more than they save.
- `vuln` reads the `plain` cache of the runner. So do the integration build and unit coverage jobs,
  which stay on the runner with the rest of their lane.
- `test-budget` only reads two logs, so a pull would cost more than the job.
