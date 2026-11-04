<!-- describes: .github/workflows/ci.yml -->
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

Everything else runs as it does for `main`: the Go build and gates, lint, craftsmanship, the license and
secret scans, the extension lane and the frontend lane. `ci` stays the required check.

CodeRabbit reviews pull requests into the default branch only. A pull request into `develop` gets no
CodeRabbit review; the nightly `develop` into `main` pull request does.

## Steps

```sh
git switch -c <type>/<slug> origin/develop
# work, then the smallest lane, then:
make check
git push -u origin <type>/<slug>
gh pr create --base develop
gh pr merge <n> --squash
```

A squash is fine inside `develop`. A red that only the skipped lanes can see may land on `develop`. The
nightly session fixes it, so it does not need a `claim: main-red` pull request.

## The nightly sync

1. On a branch off `origin/develop`, run `git merge origin/main` and resolve every conflict.
2. Run `make db-up` and `make check-all`, and fix every red on the same branch.
3. Open a pull request into `develop` and merge it with `gh pr merge <n> --merge`.
4. Open or update the pull request from `develop` into `main`. Merge it with `gh pr merge <n> --merge`
   once `ci` and `cla` are green and every review thread is resolved.

Both sync merges use `--merge`. A squash would give `main` commits that `develop` lacks, and the next
sync would conflict on every line the two copies share.

## When `develop` needs a `main` change before the night

A pull request merged straight into `main` reaches `develop` at the next sync. To bring it in sooner,
open a pull request into `develop` from a branch that merges `origin/main`, and merge it with `--merge`.
