<!-- describes: scripts/check-go-work-sum.sh renovate.json -->
# Finish a Renovate Go update

Renovate updates the Go modules in one pull request, `Update go dependencies (non-major)`, on the
branch `renovate/go-dependencies-(non-major)`. It runs `go mod tidy` in each module, so `go.mod` and
`go.sum` are right. It cannot run `make install`, so it leaves `go.work.sum` without the workspace
checksums that `make install` writes. The `go-work-sum` gate fails such a pull request, and it is right
to: a missing checksum breaks a project that pins this repository as a submodule.

So a person or the nightly maintenance session finishes each Go update. No CI job pushes to a Renovate
branch.

## Steps

1. Find the open Go update. It carries the `dependencies` label and the title above.
2. Check out its branch and fill in the checksums:

   ```sh
   git fetch origin 'renovate/go-dependencies-(non-major)'
   git switch -c finish-go-update FETCH_HEAD
   make install
   make go-work-sum
   ```

   `make go-work-sum` prints `OK` once `go.work.sum` holds what `make install` leaves behind.
3. Commit `go.work.sum` alone. `make install` touches nothing else that belongs in this pull request;
   leave any other change out.
4. Push it to the Renovate branch:

   ```sh
   git push origin 'HEAD:renovate/go-dependencies-(non-major)'
   ```

5. Merge the pull request the same day, once `ci` and `cla` are green. Use a squash merge, as for any
   other pull request.

## Why the same day

Renovate merges its own pull requests, but it stops rebasing a branch that someone else pushed to. A
finished Go update that waits falls behind `main` and gets no help from Renovate. If it falls behind
anyway, merge `main` into the branch, run `make install` again, and commit `go.work.sum` once more.
