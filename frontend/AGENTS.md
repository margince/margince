<!-- prose:plain -->
# `AGENTS.md`: working in `frontend/`

These rules hold only in this folder. The repository's own [AGENTS.md](../AGENTS.md) still holds for
all the rest: the branch and PR loop, the license header, the commit rules. The rules below are for
the frontend alone. Gates hold parts of some, and each section says which; what it does not name, no
gate checks for you.

`craft static` does not scan `*.test.tsx`. So P3 (`tests prove behaviour or they are noise`, no test
that fails from a real clock) holds here only because the author holds it. One part has a gate:
`make fe-clock-drift` runs all the tests with the clock 200 days later and needs the same result. It
runs daily on `main` (`scheduled.yml`), not on your PR, because the calendar breaks these tests, not
a diff. A test file you add today can turn that lane red many days from now. Run it on your machine
when a test you touch reads a date.

## Read the design system before you build something a user can see

Read [`src/design-system/README.md`](src/design-system/README.md) before you build a control, every
time. It is the catalog: what already exists, what each building block is *for*, which file holds
it, and whether it has a story. A duplicate looks right in review and drifts from the first one
later.

The rule it states: every control a user can click or type into comes from `src/design-system/`. An
HTML `<select>`, a dropdown built by hand, one more "only this once" chip or a second modal is a bug.
Cards, buttons, inputs, fields, badges, tables, empty states, menus and dialogs are all already
there.

Duplicates here come from an author who does not know the catalog file exists, writes a component
that looks right, and passes review. So:

- **Grep the catalog for the name first**, before you write a control. Look for `Card`, `Panel`,
  `Button`, `Select`, `Field`, `Badge`, `ListTable`, `EmptyState`, `Callout`, `SurfaceState` and
  `Eyebrow`. If the thing you plan to build has a name, that name is most likely already in the
  table.
- **Before you write a card-like `<div className="...">`**, check whether it is a
  `Panel`. That is the card this app uses: header, `PanelBody`, `PanelRow`, `panel-foot`. Two card
  building blocks make the settings and the record page look like two products.
- **Screens do not keep building blocks.** If one screen imports something from a second screen, it
  goes in `src/design-system/`.
- **If it is not there, add it there**, with a story, and update the README table in the same
  commit. `catalog.test.ts` fails a shipped component that the table never names.

`make native-controls`, `catalog.test.ts`, `type-source.test.ts` and the `check-ds-*.sh` and
`check-space-tokens.sh` shell gates hold part of this. Not one of them can tell that the component
you wrote already exists under some other name. The catalog gate makes sure a search finds it; the
grep is still your job.

**A short label is one `Badge`**: `soft` or `primary`, 7 tones, its icon first. `soft` has a fill
with some of the tone, plus a line in the tone; `primary` has a fill with all of the tone. `ai` shows
Sparkles. No `uppercase`, no click; `badge-spelling.test.ts` fails a badge built by hand or a new
look for one.

### Indigo is a claim about the source

Indigo (`--ai*`) means an agent is its source. Never use it for looks. `--orbAmber` / `--orbRed` /
`--orbGrey` mean a result, not a source. The tokens, the edge and text rules, and `ProvenanceTag` are
in [the catalog](src/design-system/README.md#indigo-says-a-machine-did-it). `check-ds-purity.sh`
holds that these values come from tokens; nothing can tell you that the token you use means the wrong
thing.

## Text follows the writing guides

English catalog text follows [`docs/reference/ui-copy-style.md`](../docs/reference/ui-copy-style.md);
German adds [`ui-copy-style-de.md`](../docs/reference/ui-copy-style-de.md). `copy-style.test.ts` and
`copy-style-de.test.ts` hold only the rules a program can check. The tone, the words, the length and
the shape of a message are yours to check before you add or change a value.

## A test must pass on a slow machine

### A wait that runs out is most often waiting for something that never happens

A test that times out on a slow machine but passes by itself is most often waiting for something that
never happens. Before you make a timeout longer, check the run time of the file on its own against its
run time with all the tests. A slow runner makes a file take some more time; to use up a 10-second
wait, it must take about 40 times as long.

One such case proved to be a product bug. React Query passes new `options` to `useMutation` in a
`useEffect`. So between the commit that renders a live control and that `useEffect` running, the
`MutationObserver` still holds the closure of the render before. A click in that gap runs against old
state, and the mutation refuses it. The test then waits out its whole time limit for a render that
never comes. On a slow machine the gap is longer.

A wait that runs out close to its whole time limit means the thing never happened. A longer timeout
hides it. Capture the error text of the check and the rendered DOM on the failing run; that tells a
refused click from a slow render.

The rule, and the gate that holds it:

- **A `mutationFn` takes variables.** It takes what it needs as a variable, and never closes over
  render state. The click handler is part of the committed render, so a variable it passes cannot be
  older than the control that holds it.
- A check for an empty value at the start of a `mutationFn` (`if (!form) return`) does not keep you
  safe. It is what *happens* when an old closure runs, and it refuses a form the user has filled in.
  An old closure can also send a form state the user never set.
- `src/screens/mutation-variable-coverage.test.ts` walks the TSX with the TypeScript compiler API
  and fails on the pattern. Do not find a way past it.

### Use the app in tests in a way that does not cost clock time

`userEvent` moves on real timers. Its `delay` is `0` by default, which is still a number. So `wait()`
schedules a real `setTimeout`, and every typed key and click gives up a macrotask (`user-event`
14.6.3, `utils/misc/wait.js:9`). The cost of a test goes up with its count of clicks and keys, on a
queue it shares with every other jsdom test file. That pushes the screen test files with many clicks
past the 5-second default of vitest when the machine is slow.

The cost is per event, not per `setup()`, so a new `userEvent` per click is not itself the cost.
Still, make one per test. One `userEvent` holds the shared state of the input. A second one does
not know which keys and buttons the first one still holds down.

When you write or touch a screen test:

- Call `userEvent.setup()` once per test, never per click or key.
- Wait on a state, not on a length of time. Use `findBy*` / `waitFor` over the state you expect, not
  a `SETTLE_MS` limit that a slow scheduler can miss.
- No `setTimeout`, no sleep, no real clock. Use `vi.useFakeTimers()`, and send clicks and keys
  through `userEvent.setup({ advanceTimers })` when the component runs on timers.
- A test that passes only when it has the machine to itself is not ready. Run the file by itself and
  inside `make fe-unit`, and check the two against each other.
- Test files split at 1000 lines, the same limit the Go test trees hold. `make fe-file-length` holds
  it: a file already over the limit keeps its fixed count in `scripts/fe-file-length-waivers.txt`,
  and that count may only go down.

## Storybook is documentation, and it goes out of date without a sign

Stories are next to their component as `<name>.stories.tsx`. `frontend/scripts/fe-uat.mjs` finds them
by that place, and `pnpm storybook` serves the catalog on :6006. It has a `Theme` control that
switches `data-theme` the way the app shell does.

What the gates cover, and what they do not:

- `make fe-bundle` builds the catalog in CI and is a check that must pass. So a story that fails to
  compile, or fails to register, is found every time.
- `make fe-uat` is the render gate for what a change touches. It fails on a render error, on a
  changed component with no story, and on a changed story the build does not register. Two things
  limit it. It is a lane that need not pass, and `ARGS="--allow-missing"` turns the missing-story
  failure off. So nothing stops a component from shipping without a story.

That second gap is the rule you keep by hand. When a change adds or changes a component in
`src/design-system/` or a screen surface:

- Add or update its `.stories.tsx` in the same commit. Cover the states the change adds, not only the
  normal one.
- Check it in both themes before you call the surface ready. Every derived value is a `color-mix()`
  of a core token, and the `dark` theme changes that token. So a surface can be right in `light` and
  wrong in `dark`.
- Run `make fe-uat` on your machine for a frontend change. That it need not pass in CI is about CI,
  not a reason to skip it.
- `src/design-system/README.md` is the catalog and the text that goes with it. A new control, a new
  kind of a component, or a changed prop contract updates that file too. It is what the next
  developer reads instead of building a second dropdown by hand.
