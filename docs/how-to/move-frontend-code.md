<!-- prose:plain max-words=1000 -->
# Move frontend code

A move PR puts a file in the folder it belongs in and changes nothing else. You need one when
`scripts/feature-boundaries.test.ts` fails. It fails in three cases:

- a file outside the shell imports a feature;
- a feature imports the shell;
- one feature alone uses a file in `components/`, `lib/` or `format/`.

The rules and the folders are in
[frontend-architecture.md](../explanation/frontend-architecture.md#the-layers).

## Where the file goes

| The file is used by… | It goes to |
|---|---|
| tests or stories of two or more features (a fixture, a testkit) | `src/testing/` |
| two or more features, and it draws | `src/components/` |
| two or more features, and it draws nothing | `src/lib/` |
| one feature only | that feature's folder |
| a feature that shows another feature's screen | that screen's own feature; `app/` passes it in |

A file can move down only if it imports no feature itself. If it does, move what it imports first,
or leave the import where it is.

## Rules for a move PR

- **A name clash is the one rename.** If the name is taken in the new folder, add a part to it, as
  in `agentusage.counters.ts`.
- **Move, never change.** Each file keeps its name and its code. Only import paths change. A file
  that needs a fix, a split or a new name gets its own PR, before or after the move. A split also
  fails here: code that leaves its file counts as new lines, so every comment in it must then pass
  the comment checks. Only a gate that reads moved code as new may ask for more, as the
  [section on those gates](#when-a-gate-reads-moved-code-as-new) lists.
- **Never add a line to `scripts/feature-boundaries-waivers.txt`.** The list holds the imports between
  features that were there when the gate landed, and it only gets shorter. When your move removes one,
  the gate asks you to delete its line.

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
     src/features/<name>/<file>.ts src/lib/<file>.ts \
     src/features/<name>/<file>.test.ts src/lib/<file>.test.ts
   ```

   Pass one pair for each file, or one pair for a whole folder. Write the new path in full. Move a
   file's test, stories and CSS with it.

3. Read the list the script prints at the end. It names text outside imports that still holds an old
   path. That may be a gate, a ledger or a doc. Change each path by hand. If you change an import by
   hand, run `pnpm exec biome check --write` on that file to sort it.

4. Search for the bare name too:

   ```sh
   git grep -nw '<name>'
   ```

   Some guards name a file by its name alone, so the script misses them.

5. Search the moved tests for a path they build from their own folder, such as
   `join(__dirname, "../..")`. A move from `features/<name>/` to `lib/` is one level up, so the path
   needs one `..` less.

6. If a path changed inside a comment, read the whole comment again. `make craft-static` judges
   every sentence of a line you touch by the
   [comment rules](../reference/docs-prose-style.md#comments-in-code).

7. Push and open a draft PR. The PR body lists the files that moved and the paths you changed by
   hand.

8. Before you mark the PR ready, run the checks:

   ```sh
   make craft-static
   make check
   ```

   `make craft-static` takes a short time, so run it first. Do not change the tree while `make check`
   runs. If you changed a file under `backend/`, also run `make test-it DIR=<pkg>` for that package.

## Guards that scan the screens

Some tests scan every screen file. They check the list sort, the page size, the shared styles, the
card zones, the AI caches, the AI activity lines and the settings imports. They read their folders from
`SCREEN_ROOTS` in `frontend/src/testing/screenfiles.ts`, so a file that moves between `features/`
and `components/` stays in their scan.

A guard with its own list of folders would miss a moved file and still pass. If you find one, make it
read `SCREEN_ROOTS` in the same PR.

## When `main` moves under you

A move touches many import lines, so it often conflicts with work that lands first.

1. Run `git rebase origin/main`.
2. In an import conflict, keep the lines from `main`, then point each one at the new path.
3. Delete an import that is now there twice, once by the old path and once by the new.
4. Search the files that landed on `main` since your branch started. They may import a moved file
   by its old path.
5. Search `vi.mock` paths by hand. `tsc` does not check them.
6. Run `make check` again before you mark the PR ready, or before you push if it is ready already.

## When a gate reads moved code as new

`make fe-ds-gates` refuses spacing written in `px` on changed lines, and a moved stylesheet counts
as changed. Swap each such value for the `--space-*` token of the same size in `tokens.css`, such
as `var(--space-2)` for `8px`. The page then draws the same.

SonarCloud reads a moved file as new code too. So a move takes on the old test gaps of its files, and
the gate can fail below 80% coverage of new code. Find the moved files with the most lines that no
test runs. Write a real test for one or two of them in the same PR. Do not lower the gate.
