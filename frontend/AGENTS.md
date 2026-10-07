# AGENTS.md: working in `frontend/`

Scoped to this directory. The root [AGENTS.md](../AGENTS.md) still governs
everything else: the branch/PR loop, the license header, the commit rules. The
rules below are frontend-only. Gates hold parts of some, and each section says
which; whatever it does not name, no gate catches for you.

`craft static` does not scan `*.test.tsx`, so P3 (*tests prove behaviour or
they are noise*, no real-clock flakiness) holds here only because the author
holds it. One part is enforced: `make fe-clock-drift` runs the whole suite at
+200 days and requires the same verdict. It runs daily on `main`
(`scheduled.yml`), not on your PR, because the calendar breaks those tests, not
a diff; a fixture you add today can red that lane weeks from now. Run it
locally when a test you touch reads a date.

## Read the design system before you build anything you can see

Read [`src/design-system/README.md`](src/design-system/README.md) before you
build a control, every time. It is the catalog: what already exists, what each
primitive is *for*, which file holds it, and whether it has a story. A
duplicate looks fine in review and drifts from the original afterwards.

The rule it states: every interactive control comes from `src/design-system/`.
A native `<select>`, a hand-rolled dropdown, one more "just this once" chip or a
second modal is a defect. Cards, buttons, inputs, fields, badges, tables, empty
states, menus and dialogs are all already there.

Duplicates here come from authors who did not know the catalog file existed,
wrote a reasonable-looking component, and passed review. So:

- **Grep the catalog for the noun first**, before writing a control. `Card`, `Panel`,
  `Button`, `Select`, `Field`, `Badge`, `ListTable`, `EmptyState`, `Callout`,
  `SurfaceState`, `Eyebrow`: if the thing you are about to build has a name,
  that name is probably already in the table.
- **Before writing a box-drawing `<div className="...">`**, check whether it is
  a `Panel` (the house card: header band, `PanelBody`, `PanelRow`,
  `panel-foot`). Two card primitives make settings and the record page look
  like two products.
- **Screens do not keep primitives.** If a screen exports
  something another screen imports, it belongs in `src/design-system/`.
- **If it is not there, add it there**, with a story, and update the README
  table in the same commit. `catalog.test.ts` fails a shipped component the
  table never names.

`make native-controls`, `catalog.test.ts`, `type-source.test.ts` and the
`check-ds-*.sh` and `check-space-tokens.sh` script gates hold part of this. None
of them can tell that the component you just wrote already exists under another
name. The catalog gate keeps it findable; the grep is still yours.

**A label in a pill is one `Badge`**: `soft` or `primary`, seven tones, icon left.
Soft is a tint plus tone hairline, primary a solid fill; `ai` draws Sparkles.
No caps, no click; `badge-spelling.test.ts` fails a hand-rolled pill or restyle.

### Indigo is a claim about provenance

Indigo (`--ai*`) means an agent proposed it. Never use it as decoration.
`--orbAmber` / `--orbRed` / `--orbGrey` mean an outcome, not provenance. Tokens,
the staged dashed edge, the text-contrast rule and `ProvenanceTag`:
[the catalog](src/design-system/README.md#indigo-says-a-machine-did-it).
`check-ds-purity.sh` holds that colours come from tokens; nothing can tell you
the token you picked means the wrong thing.

## Copy follows the style pages

English catalog text follows [`docs/reference/ui-copy-style.md`](../docs/reference/ui-copy-style.md);
German adds [`ui-copy-style-de.md`](../docs/reference/ui-copy-style-de.md). `copy-style.test.ts`
and `copy-style-de.test.ts` hold only the mechanical rules; tone, vocabulary,
length and message shape are yours to check before you add or change a value.

## A test may not depend on how busy the machine is

### A wait that expires is usually waiting for something that never arrives

A test that times out under load but passes alone is usually waiting for
something that never arrives. Before you raise a timeout, compare the file's
run time alone and in the full suite. A slow runner stretches a file by a small
factor; exhausting a 10s waiter needs a factor of about 40.

One such case was a product bug. React Query re-arms `useMutation`'s options in
a *passive* effect, so between the commit that renders an enabled control and
that effect running, the observer still holds the *previous* render's closure.
A click landing in that window runs against stale state, the mutation refuses
it, and the test waits out its full budget for a render that never comes. Under
load the window is wider.

A wait that dies close to its full budget means the thing never arrived.
Raising the timeout hides it. Capture the assertion's error text and the
rendered DOM on the failing run; that separates a refusal from a slow render.

The rule, and the gate that holds it:

- **A `mutationFn` takes variables.** It takes what it needs as a variable and never closes over
  render state. The click handler belongs to the committed render, so a
  variable it passes cannot be older than the control that carried it.
- A falsy guard at the top of a `mutationFn` (`if (!form) return`) offers no
  protection. It is what *fires* when a stale closure happens, and it refuses a
  form the user has filled in. A stale closure can also submit choices nobody
  made.
- `src/screens/mutation-variable-coverage.test.ts` walks the TSX with the
  TypeScript compiler API and fails on the pattern. Do not work around it.

### Drive the UI in a way that does not cost wall-clock time

`userEvent` advances on real timers. Its `delay` defaults to `0`, which is still
a number, so `wait()` schedules a real `setTimeout` and every simulated keystroke
and click yields a macrotask (`user-event` 14.6.3, `utils/misc/wait.js:9`). A
test's cost therefore scales with its interaction count, on a queue it shares
with every other jsdom suite. That pushes the interaction-heavy screen suites
past vitest's 5s default under contention.

The cost is per event, not per `setup()`, so constructing an instance per
interaction is not itself the expense. Do it once per test anyway: one instance
carries the shared input-device state, and a second one forgets which keys and
buttons the first left held.

When writing or touching a screen test:

- Call `userEvent.setup()` once per test, never per interaction.
- Wait on a condition, not on a duration. `findBy*` / `waitFor` over the settled
  state, not a `SETTLE_MS` ceiling that a slow scheduler can starve.
- No `setTimeout`, no sleep, no real clock. Inject fake timers, and drive
  interactions through `userEvent.setup({ advanceTimers })` when the component
  is timer-driven.
- A test that passes only when it has the machine to itself is not finished.
  Run the file alone and inside `make fe-unit`, and compare.
- Test files split at 1000 lines, the same ceiling the Go test trees hold.
  `make fe-file-length` enforces it with a ratchet: one already over carries its
  frozen count in `scripts/fe-file-length-waivers.txt` and may only shrink.

## Storybook is documentation, and it goes stale silently

Stories live beside their component as `<name>.stories.tsx`. That co-location is
what `frontend/scripts/fe-uat.mjs` keys on, and `pnpm storybook` serves the
catalog on :6006 with a Theme control that flips `data-theme` the way the
shell does.

What the gates cover, and what they do not:

- `make fe-bundle` builds the catalog in CI and is a required check, so a
  story that fails to compile or fails to register is caught deterministically.
- `make fe-uat` is the change-scoped render gate. It fails on a render error,
  on a changed component with no story, and on a changed story the build
  does not register. Two things weaken it: it is a coordinator lane and not
  required, and `ARGS="--allow-missing"` turns the missing-story failure off.
  So nothing stops a component from shipping without a story.

That second gap is the rule you keep by hand. When a change adds or alters a
component in `src/design-system/` or a screen surface:

- Add or update its `.stories.tsx` in the same commit, covering the states the
  change introduces, not just the happy one.
- Check it in both themes before calling the surface done. Every derived
  value is a `color-mix()` of a canonical token and follows the dark accent lift,
  so a surface can be correct in light and wrong in dark.
- Run `make fe-uat` locally on a frontend change. Being unrequired is a fact
  about CI, not permission to skip it.
- `src/design-system/README.md` is the catalog and the prose that goes with it.
  A new control, a new variant, or a changed prop contract updates that file too.
  It is what the next developer reads instead of hand-rolling a second dropdown.
