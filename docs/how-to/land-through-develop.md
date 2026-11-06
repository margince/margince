<!-- describes: .github/workflows/ci.yml Makefile -->
# Land a change through `develop`

`develop` is a second branch, where a pull request lands after a shorter CI run. Each night a scheduled
session brings `develop` level with `main`, runs the full gate on it, fixes what is red, and merges
`develop` into `main`. `main` keeps its full gate, so it stays deployable.

`main` is still the default branch, and every rule in `AGENTS.md` holds as written. A change goes to
`develop` only when its author chooses that route.

## What CI runs on a pull request into `develop`

The `changes` job sets `full_lane` to false when the base is `develop`. These jobs then skip:

- `integration` (the Postgres shards);
- `uat` and `live-boot`;
- `images`;
- `sonarcloud`.

`backend-lint` also skips the test-input census, which guards the cache warm-up on `main`.

CI sets `CHECK_BASE_REF` to the base branch. So strict lint, `comment-budget`, the citation check, the contract check and
the migration check judge only what the pull request adds to `develop`. Into `main` they judge what it adds
to `main`, so the nightly pull request judges all of `develop`'s new code.

Everything else runs as it does for `main`: the Go build and gates, lint, craftsmanship, the license and
secret scans, the extension lane and the frontend lane. `ci` stays the required check.

CodeRabbit reviews pull requests into the default branch only. A pull request into `develop` gets no
CodeRabbit review; the nightly `develop` into `main` pull request does.

## Steps

```sh
git switch -c <type>/<slug> origin/develop
# work, then the smallest lane, then:
make check CHECK_BASE_REF=origin/develop
FE_UAT_BASE_REF=origin/develop make fe-uat # for a frontend change
git push -u origin <type>/<slug>
gh pr create --base develop
gh pr merge <n> --squash
```

A squash is fine inside `develop`. A red that only the skipped lanes can see may land on `develop`. The
nightly session fixes it, so it does not need a `claim: main-red` pull request.

## The nightly sync

1. On a branch off `origin/develop`, run `git merge origin/main` and resolve every conflict.
   Keep the migration stamps `main` has, and give anything only `develop` has a stamp above them.
2. Run `make db-up` and `make check-all CHECK_BASE_REF=origin/develop`, and fix every red on the same
   branch. With the default base, the run never asks whether the branch kept what `develop` has.
3. Open a pull request into `develop` and merge it with `gh pr merge <n> --merge`.
4. Open or update the pull request from `develop` into `main`. Merge it with `gh pr merge <n> --merge`
   once `ci` and `cla` are green and every review thread is resolved.

A branch that carries `main` commits `develop` lacks is judged by the migration check against the `main`
it merged, since `main` sets the order an installation applies. It must also keep every migration on
`develop`, once, under any stamp. After a sync that re-stamps a migration, a database that ran `develop`
needs `make dev-fresh`.

The pull request into `main` runs every lane under the usual path filters, and CodeRabbit reviews it.
Answer its findings with a fix on `develop`, through a pull request of its own.

Both sync merges use `--merge`. A squash leaves the commits of `develop` out of the history of `main`.
The next sync then merges them a second time, and conflicts wherever later work changed the same lines.

## When `develop` needs a `main` change before the night

A pull request merged straight into `main` reaches `develop` at the next sync. To bring it in sooner,
open a pull request into `develop` from a branch that merges `origin/main`, and merge it with `--merge`.
