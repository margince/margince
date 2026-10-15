<!-- prose:plain max-words=1000 -->
# Move a frontend feature

A move PR takes the files of one feature out of `frontend/src/screens/` and puts them in
`frontend/src/features/<name>/`. It changes where code lives and nothing else. The layers and the
import rules are in [frontend-architecture.md](../explanation/frontend-architecture.md#the-layers).

## Rules for a move PR

- **Move, never change.** Each file keeps its name and its code. Only import paths change. A file
  that needs a fix, a split or a new name gets its own PR, before or after the move. A split also
  fails here: code that leaves its file counts as new lines, so every comment in it must then pass
  the comment checks. Only a gate that reads moved code as new may ask for more, as the
  [section on those gates](#when-a-gate-reads-moved-code-as-new) lists.
- **One feature per PR.** The issue for the move lists the files. If you find a file that it
  missed, add it to the PR and say so in the PR body.
- **A size limit grows only by import lines.** A file over the size limit has its line count in
  `scripts/fe-file-length-waivers.txt`. A move may add the import lines it added to that count, and
  nothing else. The rule is at the top of that file.

## Steps

1. Branch off `main`:

   ```sh
   git switch -c refactor/fe-<name> origin/main
   ```

2. Move the files with the move script. It runs `git mv`, makes the folders it needs, and changes
   every import that names a moved file, `vi.mock` paths included. It stops if the tree has changes
   you have not committed:

   ```sh
   cd frontend
   node scripts/move-modules.ts \
     src/screens/<name>.tsx src/features/<name>/<name>.tsx \
     src/screens/<name>.test.tsx src/features/<name>/<name>.test.tsx
   ```

   Pass one pair for each file, or one pair for a whole folder, such as
   `src/screens/record360 src/features/records/record360`. Write the new path in full. Move a file's
   test, stories and CSS with it.

3. Read the list the script prints at the end. It names text outside imports that still holds an old
   path. That may be a list of size limits, a gate or a doc. Change each path by hand. If you
   change an import by hand, run `pnpm exec biome check --write` on that file to sort it.

4. Search for the bare name too:

   ```sh
   git grep -nw '<name>'
   ```

   Some guards name a file by its name alone. The script searches for `screens/<name>`, so it misses
   them.

5. Search the moved tests for a path they build from their own folder, such as
   `join(__dirname, "../..")`. The folder is now one level deeper, so the path needs one more
   `..`. A test that builds a path to a moved file from another folder needs the new path too.

6. If a path changed inside a comment, read the whole comment again. `make craft-static` judges
   every sentence of a line you touch by the
   [comment rules](../reference/docs-prose-style.md#comments-in-code).

7. Check `scripts/fe-file-length-waivers.txt`. For each moved file it lists, change the path. Then
   add the import lines the move added to its count, and nothing else.

8. Run the checks:

   ```sh
   make craft-static
   make check
   ```

   `make craft-static` takes a short time, so run it first. Do not change the tree while `make check` runs. If
   you changed a file under `backend/`, also run `make test-it DIR=<pkg>` for that package.

9. Push and open the PR. The PR body lists the files that moved and the paths you changed by
   hand. It also lists each line count you changed there.

## Guards that scan the screens

Some tests scan every screen file. They check the list sort, the page size, the shared styles, the
card zones, the AI caches, the AI activity lines and the settings imports. They read their folders from
`SCREEN_ROOTS` in `frontend/src/testing/screenfiles.ts`, and `features` is already on that list. A
moved file stays in their scan, and you change nothing there.

A guard with its own list of folders would miss the moved files and still pass. If you find one, make
it read `SCREEN_ROOTS` in the same PR.

## When `main` moves under you

A move touches many import lines, so it often conflicts with work that lands first.

1. Run `git rebase origin/main`.
2. In an import conflict, keep the lines from `main`, then point each one at the new path.
3. Delete an import that is now there twice, once by the old path and once by the new.
4. Search the files that landed on `main` since your branch started. They may import a moved file
   by its old path, also as `../` from a folder below.
5. Search `vi.mock` paths by hand. `tsc` does not check them.
6. Run `make check` again before you push.

## When a gate reads moved code as new

`make fe-ds-gates` refuses spacing written in `px` on changed lines, and a moved stylesheet counts
as changed. Swap each such value for the `--space-*` token of the same size in `tokens.css`, such
as `var(--space-2)` for `8px`. The page then draws the same.

SonarCloud reads a moved file as new code too. So a move takes on the old test gaps of its files, and
the gate can fail below 80% coverage of new code. Find the moved files with the most lines that no
test runs. Write a real test for one or two of them in the same PR. Do not lower the gate.

## After the merge

Remove your worktree, delete the branch on your machine and on GitHub, and comment on the issue that
the feature has moved.
