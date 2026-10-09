<!-- prose:plain -->
# `src/design-system/`: the Margince design system

Read this before you build a control by hand.

This file is the catalog: what exists, and what each thing is for. How a screen should look (grounds,
chrome, type, depth, the parts of a record page) is in [`DESIGN.md`](../../../DESIGN.md) at the
root of the repository. Read that first when the question is about the look, not about the parts.

## The rule

**Every control comes from this directory.** That means each one a user can press or fill. Take a native `<select>`, a
dropdown built by hand, a menu made for one screen, or a second modal. Each is a defect, and so is
one more "just this once" chip. There are two reasons:

1. The browser draws a native control. `<select>` takes our tokens on its closed face, and none of
   them on the list behind it. No browser engine we ship to lets `option` take a style. So it
   reads as a hole in a product built from these files.
2. A copy drifts, and nobody sees it. Each copy grows its own gap, its own focus ring and its own
   idea of what a label is. Each one looks fine in review on its own.

If what you need is not here, add it here, with a story and a spec, and it becomes the one way to
spell it. UI text never lives in a primitive: words come in through props, and the caller
translates them with `t()`.

## Type comes from the tokens

One rule sets the type for the whole product. `body` in `app.css` reads `--fontBody`, and `html`
beside it reads `--textPrimary`. A reset hands both to the elements a browser would otherwise size
itself. The root declares no font, so `1rem` stays the browser's own 16px. A `font` on the root would
read its own `rem` against the size it was setting, and shrink the whole page. 

Everything inherits, so there is no size, line height, weight or neutral ink to pick where a component is used.
`src/mcp-apps/view.css` carries the same shape for the views that stand alone.

Every type token in `tokens.css` is one `font` shorthand in rem. It holds weight, size, line height
and face in one value, so nobody can use half of a level. The weight in it is itself a token
(`var(--fontWeightBold)`). A `font` shorthand takes it, because the browser puts in the `var()`
value before it checks the grammar of the shorthand.

Size, line height, weight and letter space exist only in `tokens.css`. Every other sheet, and every
style object in the tree, uses a `--font*` token or inherits. `design-system/type-source.test.ts`
fails a second way to spell any of the four, wherever it is written.

Body text has three levels, each with its own paragraph space. The gap between two blocks of prose
is that space and nothing else: one sibling rule in `app.css`, at zero specificity, puts
`--paragraphSpacing` between them. So a level that is not the default hands its own space down
beside its `font`, or the prose keeps the rhythm of the default.

| Level | Token | Spacing | For |
|---|---|---|---|
| L | `--fontBodyLarge` | `--paragraphSpacingLarge` | Reading at ease: sales pages and blog prose. Rare in the product. |
| M | `--fontBody` | `--paragraphSpacing` | The platform default, and what `body` already uses. |
| S | `--fontBodySmall` | `--paragraphSpacingSmall` | Use with care: second-level content and messages that carry meaning. |

**A heading's size comes from its context.** There are seven tokens, all in the heading face at
`--fontWeightBold`:

| Token | For |
|---|---|
| `--fontHeadingXXLarge` | Brand and sales content. |
| `--fontHeadingXLarge` | Brand and sales content, and the largest page title in the product. |
| `--fontHeadingLarge` | A page title in the product: the title of a view or a form. |
| `--fontHeadingMedium` | A large component with room, set against Body M: the title of a modal. |
| `--fontHeadingSmall` | A title in a small component where space is tight. |
| `--fontHeadingXSmall` | The same, tighter still: the title of a flag. |
| `--fontHeadingXXSmall` | Use with care; it goes with Body S, as small print does. |

**A heading's level comes from the structure.** A user of a screen reader moves through a page by
`<h1>`–`<h6>`, and everyone else sees the page group by them. They run in order from the top down.
There is one `<h1>` (the page title), and no level is ever skipped (no `<h2>` followed by an
`<h4>`). Size and level do not depend on each other. An `<h3>` inside a modal with room uses
`--fontHeadingMedium`, while an `<h2>` on a full card uses `--fontHeadingXSmall`.

A heading is written as `Heading`, which makes both choices: `size` for the token, `as` for the
level. A gate keeps the raw `<h1>`–`<h6>` elements out of the tree, so the two stay together. There
is no global CSS rule for `h1`–`h6`. `Heading` maps each size to a default element, and `as`
replaces it.

A new section gets a heading: the element and a heading token. Bold body text, or a size made larger
by hand, is not a heading. A screen reader reads out the element, so a section that starts that way
has no title for the reader who needs one most.

**Weight says what kind of text it is**, and it is named, not spelled out.
`--fontWeightRegular`, `--fontWeightMedium` and `--fontWeightBold` are the three roles, and a rule
reads the role.

- 400 regular is for plain paragraphs, so they read as prose next to a heading and next to the text
  inside components.
- 500 medium is for text beside a line icon, where the line of the glyph and the line of the letter
  should match. That covers most text inside a component.
- 700 bold is for emphasis, for telling one thing from another, and for the heading tokens. Used in
  any other place, it stops meaning anything.

Both text families ship 400, 500 and 700, and `index.html` loads those three. A fourth weight would
draw as a face the browser made up, not one the designer drew. `design-system/weights.test.ts`
reads the font request and the stylesheets against each other. It reads each weight token through
`tokens.css`, and fails on any difference.

The `.t-*` names in `base.css` are role hooks, not sizes. An element that plays a role uses one.
The rule for each role sits on top of the root, with its own gate.

- `t-caption`: the line that supports, Body S in `--textSecondary`. Size and ink come together,
  because either one alone reads as a mistake.
- `t-label`: the name of a control, Body S at `--fontWeightMedium`, one step under the value it
  names.
- `t-name`: the name of a group (a settings row, the legend of a fieldset), at medium weight and at
  the size of what it heads. The label of a field is quieter than its input. The name of a row is
  not quieter than its row.
- `.t-danger` is declared after the roles. It weighs what a role weighs, so it has to be the later
  rule to win when used on top of one.

**Mono is for code only.** An element that is code (`code`, `pre`, `samp`, `.code-block`) uses
Geist `Mono` through one rule in `base.css`. An id, a key, a URL and an amount are read, not run.

 A
figure that must line up uses `.t-num` (`font-variant-numeric: tabular-nums`). `--fontFamilyMono`
is the one token that spells it. `design-system/mono.test.ts` with `check-font-lock.sh` fails the
`t-mono` class and a mono family on any other selector. It also fails a second token that carries
one, or one in an inline style.

**Two neutral inks.** `--textPrimary` carries a heading, a record's name and body prose.
`--textSecondary` supports: a placeholder first, and each other role is decided at the element. Dark
lifts both. `--textPrimary` goes to `#fff`, and `--textSecondary` goes to `lch(63.304% 1.425 272)`
from the light `lch(40% 1 282)`. Anything quieter than the second uses the same ink at a smaller
size or a lighter weight; there is no third neutral.

`Foundations/Typography` in Storybook draws the body levels, the heading ladder and the pairs.

## Corners come from the ladder, and they are smooth

Every radius in the tree reads one of six tokens: `--r-xs` 4, `--r-sm` 8, `--r-control` 12,
`--r-md` 16, `--r-lg` 20, `--r-full` the pill. A raw px corner is a seventh value that nobody chose.

`tokens.css` also declares `corner-shape: squircle` on `:where(*)` inside an `@supports` query, and
doubles the ladder inside it. A superellipse of radius R looks about as round as a plain round
corner of R/2. So a corner keeps the size it seems to have, and gives up its two points. Nothing
where a component is used has to ask for it.

**A round thing opts out**, beside its own radius:

```css
.badge {
  border-radius: var(--r-full);
  corner-shape: round;
}
```

`:where(*)` carries zero specificity, so that one line always wins. Write it wherever you write
`--r-full` or `50%`. A superellipse at full radius is a long rounded shape, and on an avatar it is
a squircle tile.

A box that CSS adds itself takes the shape too. `*` matches elements and not pseudo-elements, so
`::before`, `::after` and `::backdrop` are named beside it in that rule. The doubled tokens reach a
pseudo-element anyway, because a custom property inherits and `corner-shape` does not. So an
unlisted one would draw a round corner at twice the radius its author asked for. `corners.test.ts`
holds the list against the pseudo-elements the tree gives a corner to.

## State colours

Five states, and a screen may not make up a new one.

| State | What it says | Base (light / dark) |
|---|---|---|
| information | The neutral report, and work still running: an `info` mark, the ring of a spinner, a row a job is still writing. | `#0485f7` / `#0485f7` |
| success | A good end: the thing finished the way the reader hoped. | `#17c964` / `#17c964` |
| warning | A note before the fact: the sentence that stops an error while it can still be stopped. | `#f5a524` / `#f7b750` |
| danger | The serious one, or the one nobody can take back: the write that failed, the record that cannot come back. | `#ff383c` / `#db3b3e` |
| discovery | What is new to this reader: onboarding, a feature they do not know yet. The one state that is not a verdict about the record. | `#964ac0` / `#b76be3` |

There is one base per state, and it is the only value someone picks. Every other member comes from
it, in `tokens.css` and nowhere else, so a new base moves the whole family at once:

| Token | How it comes from the base | What it is for |
|---|---|---|
| `--<state>` | the base | A fill, a bar, a dot, a border: everything seen with nothing read on it. |
| `--<state>Text` | the base moved down in OKLCh lightness (light) or up (dark), with chroma kept, until it clears 4.5:1 | Ink: the letters on a badge, an error line, a caption, a stat card's figure. Also the ground of a filled control. |
| `--<state>Surface` | `color-mix(in srgb, var(--<state>) N%, var(--bgElevated))` | The solid tint, for a badge. A see-through one takes its contrast from whatever it lands on, and a badge lands on everything. |
| `--<state>Bg` | the same `N%` over `transparent` | A wash on a ground the token cannot know: a tint on a whole page, a row, a stop in a gradient. |
| `--<state>Border` | the base at `45%` over `transparent` | The hairline. |

The ground of a filled control is the tone's `--<state>Text`, never its base. A base is set to be
seen at the size of a bar or a dot, and no one ink sits on all five. White gives 2.04:1 on
`--warning`, an almost black ink gives 3.19:1 on `--discovery`, and the dark danger base takes
neither. The Text token carries ink, so it fills a button that deletes, a done disc and a `primary`
badge. Their letters are in `--textOnStatusControl`. It is the one token in the palette that changes
with the theme, because the ground under it does.

The light ink, surface and border of discovery are stated as hex values (`#48245d`, `#eed7fc`,
`#d8a0f7`). The design source gives those values directly. No share of `#964ac0` over
`--bgElevated` reaches `#eed7fc`, which is bluer than the ground it would mix into. The same hue
and contrast gates measure them.

**Each tone has one spelling.** Use `warning`, not `warn`, in the prop, the end of the CSS class, the
`data-` attribute and the test: the state is a noun. Each component that carries tones exports its
words as one array. The type, the story and the gate all read it: `BADGE_TONES` (`atoms.tsx`),
`CALLOUT_TONES` (`callout.tsx`), `PANEL_TONES` (`panel.tsx`), `TOAST_TONES` (`toast.tsx`),
`SECTION_STATE_TONES` (`surfacestate.tsx`), `STAT_CARD_TONES` (`statcard.tsx`).

Two colours are not states. `--accent` is the brand and the primary action. Think of a check mark, a
"Connected", a done disc or a "Strong" reading in that colour. Each says *press me* about something
that already happened, so those read `--success*`. 

`--ai` is provenance. A progress bar or a running step
in that colour claims a model decided the figure. So work still running reads `--info*`: the ring
of a spinner, a delivery in the queue, a stage under way.

**The gate is `tokens.test.ts`.** It reads the five states off `tokens.css`. In both themes it
measures every `--<state>Text` on every ground it can land on, and on its own `Surface`. It also
measures it on its own `Bg` over every ground.

It holds every stated member to its base's OKLCh hue, and holds `Surface` and `Bg` to one share. It
fails when a state is missing from one of the dark arms.
`state-ink.test.ts` holds the other side, in every sheet the app ships: no text has its letters in a
base, SVG `<text>` included. Only SVG shapes may use one.

A colour that does not mean one of these five belongs to another family. `--accent` is the brand
and the primary action, and `--ai` is agent provenance. `--orbAmber` / `--orbRed` are the outcome of
an agent run, at a size where chrome colours go muddy. The `--tag*` row means only "not that other
tag".

## Indigo says a machine did it

`--ai` is the AI hue, and it carries meaning. An indigo tint on a surface is a claim about
provenance. It says the information, or the action it proposes, came from an agent and not a human.
Paint it where that is true and nowhere else. A card tinted indigo for looks tells every reader of
that screen something false about who decided.

It is a family of its own for the same reason the five states are: one meaning per colour.
`--accent` (deep emerald) is the brand and the primary action. The state hues report how something
went. None of them means "written by a model". The top of `tokens.css` states the split, and
`tokens.test.ts` pins it.

This family splits base from ink the same way a state does; see [State colours](#state-colours) for
the rule and the gate.

| Token | Value | Role |
|---|---|---|
| `--ai` | `#5b61d6` | The line and the mark: a solid border on a staged card the reader has focused or accepted, a focus ring, a hairline. The strongest of the four, so the rarest. |
| `--aiLight` | `rgba(91, 97, 214, 0.08)` | The ground under text an agent wrote: staged cards, the wash of the onboarding stage, a `badge-ai` fill. |
| `--aiMed` | `rgba(91, 97, 214, 0.3)` | The border of something staged. Most often `1.5px dashed`, which is half the signal (see below). |
| `--aiText` | `#3f45b0` light, `#9ba0f0` dark | Text and glyphs on `--aiLight`. Never `--ai` for text over the tint: the two are so close that it fails AA on the ground its own family paints. |

**Dashed means proposed; solid means real.** `1.5px dashed var(--aiMed)` marks a thing an agent has
staged and a human has not accepted yet. It marks `.staging-card`, `.deal-card.staged`,
`.filter-clause[data-proposed]` (a filter condition a model proposed), and the empty and staged
slots of the deck. When the dashes turn solid, that is what accepting looks like. The tint says
*who*, and the line says *whether it counts yet*.

One family, one place to declare it. `Panel tone="ai"` and `.staging-card` are the same claim on two
shapes, so `panel.css` declares both. `.panel-ai` is for the panel a machine wrote. `.staging-card`
(and `.deal-card.staged`) is the staged kind, and only the dash is different. Use the class; do not
spell the tint and the edge again. A box that drew its own indigo would tell a reader the two were
different claims.

The tone says who wrote it; the badge says what is on offer. `Panel tone="ai"` goes on a panel whose
body a model wrote. A `Badge tone="ai"` sits in the header band, in the panel's `titleAction`,
carrying `co.assistant.aiTag`. It goes on a panel that offers an AI verb: ask, draft, write again,
assess again.
They are two facts, so they combine, and a panel that is both uses both.

**`ProvenanceTag` is the rule at its smallest**: one `Badge` per provenance, and only its `agent`
arm takes `tone="ai"`, with the Sparkles that tone draws. Every other kind is the default soft badge, told
apart by its words. Those kinds are a connector, a system job, a human, a buyer, and an unknown
source.
A connector copies what a mailbox already held, and a system job runs a rule, so neither one is a
model that decides. There is no provenance class to use; the tone is the claim.

### The orb is the same claim, made loudest

The agent Core and the lit window edge draw in the Core palette, and its working tones are the AI
indigo. `--orbBody` is declared as `var(--ai)` so the two cannot drift, with `--orbGlow`, `--orbMid`
and `--orbBright` around it. It is a family because a lit glass body needs tones a flat UI accent
cannot give. It needs a light end to glow, a bright end for the one state with energy, and a dark
end to be seen against.

The tones are named by role (`--orbBody`, `--orbGlow`, `--orbMid`, `--orbBright`), not by hue, so a
new colour never makes a name wrong.

The two states that are not work keep their own hues. `--orbAmber` asks a user for something,
and `--orbRed` failed. Neither turns indigo. Provenance and
outcome are different questions. An orb that turned indigo when a run failed would say who was
working, and stop saying how it went. `--orbRed` is not `--danger`, because `--danger` is UI chrome
and goes muddy at 34px.

**Colour is never the only signal.** As with `Callout`, the words carry the meaning. A surface an
agent wrote says so in text (an `ai` `ProvenanceTag`, a "suggested" label, an "Approve" verb). The
tint makes it easy to find at a glance for readers who can see it.

## What this directory already gives you

### Foundations

| Primitive | For | File | Story |
|---|---|---|---|
| `Logomark` | The product's own "M". One mark, every fill on `currentColor`, so the shell chip and the onboarding speaker draw the same mark | `logomark.tsx` | ✅ |
| `.arrive` / `.arrive-stack` | The arrival motion: content fades in over `--dur-enter` and moves up over `--dur-move`, with `--stagger-enter` between each, four deep and then all together. A container whose children arrive says `.arrive-stack`, and its children need no class. The rule names `.wrap`, the page gutter of the shell, so every screen gets it. A stack container does not arrive itself (two fades inside each other make a weaker fade), so mark each level where a screen puts its blocks deeper. It plays when an element is added, which gives a tab panel its change while the tab strip stays still | `enter.css` | in every screen |
| `--focus-ring` / `--focus-glow` / `--focus-ring-forced` | The focus ring, in three shapes. An outline for a control on a surface, drawn outside the box so it never changes the control's size. A glow for a field, which already has an edge. A `transparent` outline with the glow, for a control that is neither, so `forced-colors` mode still has something to paint. The offset stays per component: a control its container cuts off draws the ring inside itself. A fitness function refuses another spelling | `tokens.css` | — |
| `--gapCards` / `--padCard` / `--padPanel` / `--gapActions` | The spacing roles: what a space is for, where `--space-N` says only how big it is. `--gapCards` goes between sibling surfaces, `--padCard` inside a bare card, `--padPanel` inside a panel (one step above the card, because the header band, rows and footer share that inset), and `--gapActions` between two buttons side by side. Change a role here and every screen moves with it. `check-ds-spacing-roles.sh` holds screen CSS to them. It also holds that a screen which spaces a primitive again says so in these words | `tokens.css` | — |

No caller declares these tokens and sheets again:

- `tokens.css`, the Ledger Green canon, pinned by `tokens.test.ts`;
- `brand.css`, the layer built on top: `color-mix()` over a canon token, never a new hex value;
- `base.css`;
- the sheet each component imports for itself.

 Literal colours
appear only in `tokens.css`, and in `provider-mark.tsx`, which draws the sign-in marks of other
companies; `check-ds-purity.sh` holds this. `interaction.stories.tsx` lists the colours the browser
owns (caret, checkbox tick, scrollbar thumb, selection). They are set once at the root of the page,
and belong to no component.
The other Foundations pages show tokens and add none: `colors.stories.tsx` (each colour token by
role, light beside dark), `elevation.stories.tsx`, `spacing.stories.tsx`, `motion.stories.tsx` and
`iconography.stories.tsx`. Each value on them is read from the running sheet through
`tokenspecimen.ts`, so a change in `tokens.css` moves the page, and nothing is typed twice.

### Forms and input

| Primitive | For | File | Story |
|---|---|---|---|
| `Button` | The one button. Variants: `primary`, `ghost`, `danger`, `federated`, `ai`, `aiQuiet`, `link`; `iconOnly` makes a square glyph button that still needs a name a screen reader can read. Height is always `--controlHeight`, and there is no size prop; a button with a label has a smallest width. `reason` / `reasonId` refuse the press and print why in `t-caption`, and a caller's `disabled` or `aria-describedby` cannot get past them. `pending` marks a write still running: `aria-disabled`, full ink and a turning mark, with `busyLabel` for a sentence about the wait. `federated` is the full-width sign-in door, led by a `ProviderMark`; `unavailable` is how it refuses at rest. `ai` (filled, one per surface) and `aiQuiet` (tinted, for an AI verb next to other verbs) mean a model does the work behind the click. `link` uses `.link-button`, for a verb that needs the refusal and `pending` rules of `Button` but should read as a link; `button.test.tsx` holds its class pair | `atoms.tsx` | ✅ |
| `.iconbtn` | A bare icon control that is not a `Button`: no fill, no border, no label. For the verb a row offers where a full button would shout over the row. It carries the `--controlHeight` press target, the hover and the focus ring | `base.css` | in `Button` |
| `.link-button` | A second-rank text control (in the accent colour, no fill, no border) for an action that must not fight the primary button beside it: a download, a "view existing", a verb inside a row. A class a caller puts on its own `<a>` or `<button>`. Use `Button variant="link"` when the verb also needs the refusal and `pending` rules of `Button`. Not `.co-rowlink`, which is for a row title that shows its underline only on hover. A lucide glyph may lead the label; the class sizes it | `atoms.css` | ✅ |
| `TextInput` | The one text field: `--inputHeight` on `--inputPaddingY`/`--inputPaddingX`, in root type and ink, with the placeholder in `--textSecondary`. `Textarea`, the `Field` shell, the `TokenInput` frame and the `Select` trigger take the same box; buttons stay on `--controlHeight` | `atoms.tsx` | ✅ |
| `SearchField` | A text field with the search control. `flush` is for a field whose container draws the chrome (the bar of the `⌘K` palette). It takes a `ref` for a caller that has to move focus to it | `atoms.tsx` | ✅ |
| `Textarea` | The one field with many lines | `atoms.tsx` | ✅ |
| `JsonField` | A JSON value that an expert reader edits by hand: a `.code-block` shaped like a `Textarea`, with a gutter of line numbers that marks the lines a problem points at. No colour for syntax and no auto-complete; the server checks the value. `lineOfPath` / `parseProblem` turn its key path, or the error from the parser, into the marked line. Tab indents | `jsonfield.tsx` | ✅ |
| `RichText` | The light formatting a business email needs: bold, italic, links, lists. The marks sit in a footer under the place you write, beside the caller's `hint`; `actions` adds the caller's own verbs there (the paperclip of the composer). `disabled` refuses typing. It is built on `contentEditable`, because the server's outbound allowlist (`activities.SanitizeOutboundHTML`) decides what a recipient gets. Every change reports both `html` and `text`, and both go on the wire. No image button: a remote image is a read receipt. `format="markdown"` is the same editor for a body stored as markdown (an activity's): `value` and the reported `markdown` are markdown, read through the parser `Markdown` uses, the toolbar adds a heading, and a paste of a document's markup or of markdown source ends formatted. `grow` makes `rows` a floor, for a host whose body is the field. `richtext-markdown.ts` holds both directions | `richtext.tsx` | ✅ |
| `Select` | The one dropdown: a button trigger plus a portalled listbox. Never a `<select>`. The trigger has the size of a field (`--inputHeight`). An option whose label is not in the page's language carries `lang` (WCAG 2.2 AA 3.1.2). An `adornment` draws a mark before an option's label and on the closed face; it is `aria-hidden`, and the `label` stays a plain string. `appearance="button"` draws the same trigger through `Button` (`primary`, `--controlHeight`, `pending` while a write is out). It is for the one value a reader sets beside a record's name: the lifecycle stage in the company header. A value inside a line of text stays `InlineChoice` | `select.tsx` | ✅ |
| `MultiSelect` | The sibling of `Select` for many values: the same trigger and listbox, where a pick turns a value on or off and the list stays open. For a set of words someone can list in full, such as the options of a custom field; `TokenInput` is the control for a set nobody can list. `values`/`onChange` speak `string[]` | `select.tsx` | ✅ |
| `ComboBox` | A text box that takes any value and offers suggestions from a portalled listbox. Use it over `Select` when the known answers are only a start, and the real list of words belongs to someone else (a model id). Typing is never replaced (`aria-autocomplete="list"`), Escape keeps the text, and with nothing to suggest it is a plain input. `type="email"` keeps the browser's address check on an address box. It shares `suggestlist.tsx` with `TokenInput` and `anchoredpopup.ts` with `Select` | `combobox.tsx` | ✅ |
| `useSuggestList` / `SuggestPopup` / `SuggestOption` | The portalled listbox a text box offers, and its key rules: the half that `ComboBox` and `TokenInput` share. It decides which rows match (a part of the label or value), which row is active, what Enter and Escape do, and where the popup sits. What a pick means stays with each host. `SuggestOption` is one row of that shape, which `ListPopover` draws too. `walkedTo` is where an arrow, Home or End lands, and `useActiveRow` holds that row, and lets go when the rows shrink under it. `anchoredpopup.ts` is the layer below, shared with `Select` | `suggestlist.tsx` | ✅ |
| `TimezoneSelect` | A time zone dropdown, city first, built on `Select`, with current UTC offsets and typing to jump. It keeps saved aliases and sends IANA names | `timezoneselect.tsx` | ✅ |
| `ProjectPicker` / `ScopeLine` | Which project a surface is about, and the line that says which project its output was cut down to. `ProjectPicker` is a `Select` over the shared `projects` section (`liveProjects` drops closed ones; `useSoleProjectDefault` picks the only live one). `ScopeLine` prints the server's report ("Scoped to KEY · N of M activities"). Every AI surface draws this pair | `projectpicker.tsx` | ✅ |
| `DateInput` | The one date field: a native `type="date"`. It breaks the `Select` rule, because its closed face takes our tokens, and the platform picker shows only when asked. `value` is always `YYYY-MM-DD`, the contract's `format: date` shape | `dateinput.tsx` | ✅ (`Value inputs`) |
| `TimelineFilterBar` | The row of dials above a record timeline: a kind `Select`, a `SearchField`, and a from/to pair of `DateInput` items. Every dial is a server parameter, spelled once in `recordtimeline.ts` (`timelineQueryParams`). The state is `TimelineFilters`, held by `useTimelineFilters` and handed to `useRecordTimeline`. Search commits on Enter or blur. While it is active, the bar says it leaves out conversations with limited access. `kinds` sets which kinds the dial offers | `timelinefilterbar.tsx` | ✅ |
| `TokenInput` | A set of short values built one at a time, for the `in` operator of the filter engine, where nobody can list the possible values. Enter or comma commits, Backspace on an empty box removes the last one, a copy of a value is dropped, and blur commits. `suggestions` help if given, but do not limit; a row is found by label or value. With no list of words it keeps the plain `textbox` role | `tokeninput.tsx` | ✅ (`Value inputs`) |
| `TokenList` | The `TokenInput` token without the text box, for a set built in another place. It takes `{id, label}`; `removeLabel` is a function, so each remove button names its token. Leave out `onRemove`, and the set is read-only | `tokeninput.tsx` | ✅ (`Value inputs`) |
| `OptionCount` | The count beside an option's name in `SegmentedControl` and `RecordTabs`: tabular digits, the reader's number format, its own chip, and a comma a screen reader hears but nobody sees, so it says "Contacts, 2". It draws inside the option's button and inherits the host's ink. Use it wherever a number says how much sits behind an option, tab, filter, menu entry or disclosure | `atoms.tsx` | in `Segmented control` and `Record tabs` |
| `Checkbox` / `Radio` | A tick with its label as the other half of the click target, and the product's only tick. Many of many is `Checkbox`; one of many is `Radio` with a shared `name` (`ChoiceList` where the whole question needs a name). `tick-spelling.test.ts` fails any other spelling | `atoms.tsx` | ✅ |
| `Field` | The row with a label above a control that every form is built from. It owns the id and hands the control `{ id, required, aria-describedby, aria-invalid }`. Slots: `hint` (the rule that always holds), `error` (a refusal), and `icon` / `trailing` (controls inside the control's outline). `labelEnd` puts a "Forgot?" beside the label, outside the name a screen reader reads; `labelHidden` keeps the label for screen readers only | `atoms.tsx` | ✅ |
| `usePasswordReveal` | A password field's control to show the text, and the matching input `type`, returned together so they cannot get out of step. Pass it as the `trailing` of `Field` | `passwordreveal.tsx` | ✅ (in `Field → Affordances and refusal`) |
| `useClipboardCopy` | Copies one string to the clipboard, and returns the button label, the copied state, the copy action and the failure notice. It checks for `navigator.clipboard`, which is not there outside a secure context. It takes the text, so `copied` clears when the text changes. Labels and `remedy` are the caller's; `clipboard-spelling.test.ts` fails a copy built by hand | `clipboardcopy.tsx` | ✅ |
| `CopyableText` | The text a copy control copies, as a wrapping `.code-block` that a keyboard reader can still take by hand: it is a named read-only textbox in the tab order, and focus selects all of it. For a one-time secret or link, shown once beside the button of `useClipboardCopy` | `clipboardcopy.tsx` | ✅ |
| `FieldGrid` / `FieldRow` | The grid of labels and values around a record's fields, with a label column of fixed width, so values line up on every record and in every language. A read-only row takes a plain node; one you can edit wraps `InlineText`/`InlineChoice`. `valueRef` gives access to the value cell; `stacked` gives a group editor the full width. Pass `InlineChoice` `hideLabel` so the row's label is the only one. `FieldGrid icons` adds a column of glyphs, for looks only, for every row | `fieldgrid.tsx` | ✅ |
| `MoneyInput` | An amount with its currency, put into its display form at the edge where it is shown | `moneyinput.tsx` | ✅ |
| `FileDropzone` / `FileDropzoneControl` | Choosing one file by drop or click. The `<input type="file">` is the control, and the zone is chrome over it, so the keyboard path works. `emptyLabel` is the caller's. An empty selection never fires `onPick`, and the input clears after each pick. `accept` filters the picker only, not a drop. `FileDropzone` brings its own `Field`; use `FileDropzoneControl` inside a `Field` you already draw | `filedropzone.tsx` | ✅ |
| `ServiceAccountKeyField` | A Google service-account key, pasted or chosen as its `.json` file, in one box. Only the latest read fills the box, and an edit by hand counts as newer. `serviceAccountProblem` names what is wrong before anything is sent; the key is never shown back | `serviceaccountkeyfield.tsx` | ✅ |
| `SegmentedControl` | A small closed set of options, all in view at once. `counts` puts a count beside each name (leave out a count you cannot state; a missing count is not zero). `marks` puts an `aria-hidden` dot on options with something waiting, and is never the only thing that carries that fact | `atoms.tsx` | ✅ |
| `ChoiceList` | One question with every answer readable at rest: a native `fieldset` and `legend` of radios, a legend that can be hidden and still keep its name, and a `description` per answer if needed, which is part of its label. Use it over `Select` for a decision a reader must think about, and over `SegmentedControl` when an option is a sentence. `disabled` on one choice refuses it; its `description` says why | `choicelist.tsx` | ✅ |
| `Switch` | A setting that writes when you turn it (`role="switch"`), where a `Checkbox` states an intent that a later submit sends. `pending` follows the rules of `Button`. `reason` refuses the switch by itself. `describedBy` adds an element the caller owns, such as the description of a `SettingRow`. A filter over a list is a pressed button, not a switch | `switch.tsx` | ✅ |
| `MeetingSlots` | Free meeting times you can pick, with states for selected, empty and disabled; labels come in the viewer's time zone. A pick returns only the start and end time | `meetingslots.tsx` | ✅ |
| `MeetingWeek` | A week of free times as one column per day, for a host who sets days side by side. Each slot has a full `label` for screen readers and a short `time` face; `selected` takes many starts. Without `onSelect` it is a read-only preview. An empty day says so in words | `meetingslots.tsx` | ✅ |
| `SaveBar` | The unsaved draft of a whole settings page, pinned to the foot of the view: one Save for many sections. Draw it only while something is unsaved; its content is the caller's sentence and verbs | `savebar.tsx` | ✅ |
| `RecordPicker` | Search → candidates → pick, for choosing a record that already exists. `id`, `aria-describedby` and `aria-invalid` connect a `Field` around it to the search input. `useCandidateSearch` is its search, which waits for typing to stop, and which it shares with `ListPopover` | `recordpicker.tsx` | ✅ |
| `PassportSelect` / `ScopeChips` | Which agent passport, and the scopes it carries | `passportselect.tsx` | ✅ |
| `Calendar` | A month of days with one marked, for a date chosen inside a larger decision (when to send something). Use `DateInput` everywhere else. It only draws: the month, the chosen day and `today` come from the caller. It always shows six weeks. `refusal(day)` draws a line through a day that cannot be offered, and adds the reason to its name for screen readers | `calendar.tsx` | ✅ |
| `FilterPills` | Cuts through a list, one in view (All / Conversations / Changes), each its own pill with an outline. Use `SegmentedControl` for a setting instead. `count` is optional per pill, and a missing count is not zero. `layout="list"` stacks the cuts as full-width rows | `filterpills.tsx` | ✅ |
| `SwipeRow` | A list row you answer with the thumb, where there is no width for its verbs. Stage, then confirm: a drag to the side past 56px shows the action, and a press runs it. It moves only to the side, so a scroll up or down is never read as an answer. It wraps the whole row. More quick moves step through more actions; `onStage` fires on the move. The move is extra: callers keep their buttons where there is room, and owe another path to these actions | `swiperow.tsx` | ✅ |
| `StageLadder` | The stages a record climbs, the one it stands on, and the move to any other, as one control. A `role="group"` that holds an `<ol>` of chevrons; `current` is a marker, not a button; `done` is the trail behind it; `terminal` steps (won, lost) sit in their own `<ul>`. The run stays on one row and scrolls. `reason`/`reasonId` pass to the refusal of `Button`; `hint` is the line under it | `stageladder.tsx` | ✅ |
| `SortableList` | Rows a reader puts in order by hand: a grip per row, ↑ and ↓ from the keyboard, and the new place read out in a `polite` live region. `renderItem` draws the row; `onReorder` gets the whole new order once, on release. It uses pointer events, so it works on phones. With no `onReorder` it draws no grip; `busy` keeps grips in the focus order but refusing; `selectedKey` marks the open row. `orderAtPointer` / `moveKey` are exported | `sortablelist.tsx` | ✅ |
| `IconAction` | A verb whose glyph is its whole label: square, named through `aria-label` and on hover through `useTooltip`, both from one `label` prop. Use it only where the glyph is the verb (mail, phone, calendar, pencil, link, the three dots of the overflow menu). Keep words for a verb whose result must be read first. `reason` / `reasonId`, `disabled` and `pending` pass to `Button`. `pressed` draws `aria-pressed` for a glyph that sets a state; `disclosure` draws `aria-expanded` and `aria-controls`; `inline` fits the square into a line of text | `iconaction.tsx` | ✅ |
| `ChipValueList` | The value step of one filter: a fixed list of radios, or a search box over a large set (the companies of a workspace) with its own empty, loading, error and result lines. One value per filter. The chip's own `search` picks the shape. `filterable` adds a box that narrows a fixed list by name (every colleague) and still draws every option before typing | `listfiltervalues.tsx` | ✅ |
| `InlineChoice` | Edit on hover on a record page: a chooser that commits on pick. A reader who may not edit sees the plain value. A refused save keeps the answer and shows as the field's `error`; picking the stored value again is no edit. `onEditingChange` pins the record version; `onDirtyChange` reports unsaved drafts. `FieldRow` puts it in the grid | `inlinechoice.tsx` | in `Field grid` |
| `InlineText` / `InlineEditVerb` | Edit on hover on a record page: a text field that commits on Enter or blur, with the same refusal, version and unsaved rules as `InlineChoice`. `multiline` is for a paragraph (Cmd/Ctrl+Enter commits). `type` checks email, date and number input; `step` sets how fine a number may be. `display` is the text at rest when it is not the same as the edited value. `verb` makes the trigger `InlineEditVerb` (`Change {field}`); `InlineEditVerb` alone serves an edit that opens in another place | `inlinetext.tsx` | in `Field grid` |

### Images and icons

| Primitive | For | File | Story |
|---|---|---|---|
| `Avatar` | A record's round chip (contact, company, a colleague's seat): monogram, an optional logo, and a soft mesh ground under every monogram. The mesh comes from an `FNV-1a` hash of `identity` in `avatarmesh.ts`, and skips the `--ai` and `--danger` hues; `avatarmesh.test.ts` holds its contrast in both themes. `identity` is required, and is the record's own id, so one record draws one mesh everywhere (`recordmark.test.tsx`). With a logo, no mesh is drawn; a logo that fails to load falls back to the monogram. `size`: `sm` (default), `md`, `lg`, `xl`. The monogram is `--fontHeadingXXSmall` at every size | `atoms.tsx` | ✅ |
| `CompanyLogo` | A company's full logo in a slot the caller sizes, keeping the shape of a wide logo. The caller's fallback stays in view until the image paints, and comes back if it fails | `companylogo.tsx` | ✅ |
| `AvatarStack` | A group of contacts or users as monograms on top of each other, folding past `max` into a "+N" chip. It needs a list that is not empty. The ring between them is a `box-shadow`, so a face in the stack is the same size as a chip alone | `avatarstack.tsx` | ✅ |
| `ProviderMark` | The sign-in mark of a federated provider, drawn in that company's colours. It leads a `Button variant="federated"` and keeps its own size there | `provider-mark.tsx` | in `Button` |
| `AmbientWaves` | A WebGL2 canvas of slow indigo ribbons for the ground of a place a reader arrives in (the sign-in surface and `OnboardingStage`), never behind a surface where work happens. It is `aria-hidden`, there only for the feel of the page; it is not provenance colour. It sits at `z-index: -1`, so the host must open a stacking context (`position: relative; isolation: isolate;`), as the `.auth-surface` of `auth.css` and the `.ob-page` of `onboarding-stage.css` do. Without WebGL2 it draws an empty canvas over the caller's own ground. Its only prop is `className` | `ambient-waves.tsx` | ✅ |

### Labels

| Primitive | For | File | Story |
|---|---|---|---|
| `Badge` | A label a reader reads and never presses: a 20px pill in `--fontBodySmall`, sentence case, no dot by default. `variant`: `soft` (default, a tint with a hairline of the same tone) or `primary` (solid fill, for a count or the one status a reader must not miss). `tone` comes from `BADGE_TONES`: `default`, `accent`, `info` (also work still running), `success`, `warning`, `danger`, `ai` (an agent proposed it; it always draws `Sparkles`, see [Indigo says a machine did it](#indigo-says-a-machine-did-it)), `discovery` (something new). `icon` leads the label; `live` puts a dot that pulses there instead. Long labels are cut short; `wrap` wraps them instead. A small title over a section is `Eyebrow`; a chip you can click is a control, not a badge. `badge-spelling.test.ts` fails a pill built by hand or a `.badge` with a new style | `atoms.tsx` | ✅ |
| `TagPill` | One tag as its word with a tone dot, built on the neutral soft `Badge`: a tag's colour carries no meaning, so it goes on the dot. An archived tag drops its dot and shows its archived words | `tagpill.tsx` | ✅ |
| `VisibilityBadge` / `VisibilityLine` | Who may read a thing, as one mark: an icon for the shape of the readers and a word, in six states: `team`, `workspace`, `participants`, `selected`, `private`, `withheld`. A message cut down to its record's reach says "Team"; a shared company or contact says "Shared". `private` reads "Private". Only `withheld` takes a tone. `opens` draws it as a `Popover` trigger (the access chip in the record header in `screens/recordaccess.tsx`). `VisibilityLine` puts the verb that changes the state beside it | `visibility.tsx` | ✅ |
| `RowTags` | A record row's tags as a strip of chips: two words then "+N", with the rest named in the title. It takes no input, stays on one line, and draws nothing for a row with no tags | `rowtags.tsx` | ✅ |
| `RoleBadge` / `FieldGuard` | The role of a principal, and a withheld value that reads as withheld and not as missing | `rbac.tsx` | ✅ |

### Layout and structure

| Primitive | For | File | Story |
|---|---|---|---|
| `ActionRow` | A row of verbs in two groups: second-rank verbs on the start edge and one call to action on the end edge. The two groups are real elements, so when the row wraps on a phone, the primary stays on the end edge. No `role`, no label. Not for the submit row of a form (`.form-actions`) or the footer of a modal (`.modal .actions`). `DecisionCard` is the model | `actionrow.tsx` | ✅ |
| `.card-actions` | The action row at the end of a card: the verbs under a card's body, with the space above them and the gap between them. Use `.form-actions` for the submit row of a form instead | `atoms.css` | in `Attention` |
| `.cell-actions` | A table row's verbs in its last cell: aligned right, one line, no margin, no wrap. A small cell scrolls with the table inside `TableScroll`. Use a word for each verb | `atoms.css` | in `Senders` |
| `SettingList` / `SettingRow` | One settings decision per row: what it is and does on the left, its value on the right, at one `x` down the page. Use it in every settings card. `control` takes a node, or a function that gets `{ id, aria-labelledby, aria-describedby }`. A `Switch` keeps its own label but takes the row's description through `describedBy`. `value` shows the current answer when the control does not. `layout="stack"` puts a control that is the subject (a table, a grid of choices, a log) below the name. How complex it is decides this, not its size. A control that needs two inputs goes behind a verb in a `Modal`. The list draws the hairlines | `settingrow.tsx` | ✅ |
| `NumberSetting` / `NumberSettingRow` | A whole number an admin sets inside a range, in the right column of a `SettingRow` (`control` takes the row's function form). `NumberSettingRow` is the whole row, when a label and a description are all it needs. It commits on Enter or blur, like a rename, and only a whole number in range reaches `onCommit`. Anything else stays in the box, and the caller's `refusal` is added to the row's description, so the reader hears the rule and how they broke it. `refusal` comes from the caller, because only the caller knows the unit. Typing the stored value back is no edit. Pass `min`/`max` as `{ min: N, max: M }` keyed by the wire property: `backend/gates/settingbounds_test.go` holds that shape to the contract. | `numbersetting.tsx` | ✅ |
| `Disclosure` | A section the reader opens when they want it | `atoms.tsx` | ✅ |
| `Card` | The one card surface. `as` picks the element (`section` by default, also `div` / `article` / `form` / `li`), `inset` is the kind set into the page, and `title` / `actions` draw its `SectionHeader`. A `<div className="card">` built by hand is a second card the moment one of the five chrome values moves. A `Card` with a title is for a card inside something else (a list row, an inset, an auth card). A surface with a title in a page's own column is a `Panel`, and `cardzones.test.ts` fails a `Card` with a title under `src/screens/` outside its allowlist | `atoms.tsx` | ✅ |
| `SectionHeader` | A block's heading: its title and, if needed, the verbs that act on it, never a description. Text that describes goes in the body. The title is `--fontHeadingMedium`; `level` `2` or `3` picks the element only, and `1` is the page's own name at `--fontHeadingLarge`. `panelhead.test.tsx` fails a description prop or class, and a title at another size | `atoms.tsx` | ✅ |
| `Panel` / `PanelBody` / `PanelIntro` / `PanelRow` / `PanelPlate` / `PanelGroupHead` | The zone pane: one zone of a record (or any page column) on a see-through ground with a hairline edge. The header band is fixed at `--panel-head-h` (56px) and holds a title and verbs if needed, never a description; a long title is cut short. `PanelBody` is content with padding, `PanelRow` a row to the edges (`interactive` makes the whole row one press target), and `PanelBody` items in a stack get a line between them. `PanelIntro` is the one line that describes, first in a `PanelBody`. `tone` from `PANEL_TONES` marks the one lead card on a page: `accent`, the five states, or `ai` for a body a machine wrote. `actions` is a band for verbs that change the panel. `PanelPlate` is a plate set into the page, for context. `titleLevel` sets the element. The title labels the `<section>` as a landmark. `PanelGroupHead` names a group inside the pane. `panel.test.tsx` and `panelhead.test.tsx` hold the shape and type of the head | `panel.tsx` | ✅ |
| `RailPanel` | A `Panel` that tells an empty section from a withheld one. A section the reader's role cannot read is named in `sections_omitted`, so the card says it is hidden instead of drawing an empty list. Message states are `SurfaceState` inside a `PanelBody`; `ready` draws from edge to edge. The `footer` shows only on `ready` and `empty` | `panel.tsx` | ✅ |
| `PageZones` | The page's columns as one grid: a work column plus up to two rails, in four shapes (`single` / `rail` / `aside` / `both`). The work column stays first and largest, goes to full width at 1200px, and the rails stack under it at 720px. It carries only the grid; `RecordView` puts it together. Pass the shape that matches the slots you fill. `asideOpen` folds the details pane over `--dur-move`; the folded `<aside>` is `inert`, and then leaves the page | `pagezones.tsx` | ✅ |
| `OnboardingStage` | The card with a frame that every onboarding question is asked in, with the Core beside a large title. The card fits inside the window, and the board scrolls inside it, so the rail's Continue stays in reach. Mount it once per flow; a stage per step starts the Core again. `lit` says a model is bound (read from the server). `anchor="start"` is for a board that grows while read. `where` names the part of a stop that spans many screens; `hint` is the one line in the foot. The band shows the mark, the step's name, the progress marks and the Core's state in words. `coreScale` makes the orb smaller by transform; `coreHidden` fades it when the board draws its own. `useStageTitleFocus` moves focus to the title after a board swap | `onboarding-stage.tsx` | ✅ |
| `StageActions` | A step's way on, portalled from inside the board onto the `OnboardingStage` rail, so the step keeps its own state and the button stays in view. Without a rail the actions draw in place. The primary always presses, and pressed too early it names what is still needed | `onboarding-stage.tsx` | ✅ |
| `RecordView` | The record page shell: identity, readings, timeline, with `PageZones` under them. `tabs` is its own slot just under the identity. `band` holds what describes the whole record (readings, stepper, a sentence that refuses) and scrolls as one row. It owns the column rhythm (`.record-rail`, `.record-aside`, `.record-timeline`). `timelineAnchorId` puts an id on the timeline section. `back` takes a `RecordBack` (a link back to where the reader opened the record from), drawn over the head. `asideOpen` passes to `PageZones`. Its action bar on a phone uses the card inset, so the reasons why an action is disabled stay clear of its edges | `recordview.tsx` | ✅ |
| `.record-stack` | The stack of the work column: one column, top to bottom, of full-width panels. Wrap a tab body that draws more than one panel in it. The work column itself has no space between its children, so bare siblings would touch | `composed.css` | in `Record view` |
| `.meta-row` | A list row read as a face and a body on a `PanelRow`: a meta line (who, source, time), the content under it, and an action at the end at a fixed `x`. The cells have names (`.meta-row-line`, `.meta-row-who`, `.meta-row-time`, `.meta-row-action`, `.meta-row-entry`, or an `.emailentry`), so a caller can leave one out; a row without an `.avatar` drops the face column | `composed.css` | in `ContactMemory`, `CompanyFactsPanel` |

### Loading

| Primitive | For | File | Story |
|---|---|---|---|
| `Skeleton` | One placeholder bar. Use `PendingBody`, unless you draw a shape (a ring, a chart) that lines cannot stand in for | `atoms.tsx` | ✅ |
| `BusyMark` | The turning mark a control shows while its write is still running. It is for looks only (`aria-busy` on the control is the fact). Under reduced motion it stops turning and stays in view | `atoms.tsx` | ✅ |
| `PendingBody` | The one pending state. `label` is required: it is what a screen reader hears, so name what is loading. `lines` keeps height free, so the page does not jump. `visible` shows the label too. `delayMs` holds everything back until the wait is worth a report (for surfaces that read again as the reader types). `QueryGate` / `QueryStates` / `SurfaceState` draw it, and pass on `pendingLabel` / `pendingLines` (`loadingLabel` / `loadingLines` on `SurfaceState`). Fitness functions keep one home for the read-out and the pulse | `atoms.tsx` | ✅ |

### Messaging

| Primitive | For | File | Story |
|---|---|---|---|
| `ErrorLine` | The line that says a write or a read failed, under the control or submit row it belongs to: a `<p role="alert">` in `--dangerText`. Pass `error` (the thrown value; it draws nothing while null, with words from `problemMessageOf`) or `children` (a translated sentence), never both. `id` is for `aria-describedby`; `actions` adds a verb on the same line; `inline` draws a `<span>`; `standing` drops the alert for a state that is already true on load. Inside a dialog, a line with an alert scrolls itself into view when it arrives or its words change, so a refusal under a pinned foot is seen. It owns no margin. Use the `error` of `Field` for a field, a `danger` `Callout` for what a surface states, and a `Toast` for a short-lived message. `errorline-spelling.test.ts` fails other spellings | `errorline.tsx` | ✅ |
| `Callout` | What a surface says about itself, in seven tones from `CALLOUT_TONES`: `info`, `accent` (`info` said with weight), `warning`, `danger`, `success`, `discovery` (something new to this reader), `ai` (a machine made what it is about). `title` is required, and a title alone is the common shape. The tone's glyph (you can replace it through `icon`), a body if needed, the caller's verbs and a `dismiss` follow. The tone colours only the icon and the heading, through `--callout-ink`. `kind` (`outcome`, `standing`, `event`) decides how it is read out; `live` replaces that. No `className` | `callout.tsx` | ✅ |
| `EmptyState` | What a surface shows when it has nothing to show. Bare: the one-line "nothing here". With `title` and `plate`: an empty group inside a pane, as a dashed plate, with its verb in the group's `PanelGroupHead`. With `title` alone: the first-run state, with a paragraph and one `action` that adds the first record (the projects list is the model) | `atoms.tsx` | ✅ |
| `ToastProvider` / `ToastRegion` / `useToast` | The short-lived confirm message, fixed to the foot of the viewport. One provider and one region, mounted in `main.tsx`; a screen only calls `show`. It goes away after 3.5 seconds, unless the pointer or focus is on it (WCAG 2.2.1). `action` adds a verb: an `undo` stays 8 seconds and a newer undo takes its place, an `open` stays until dismissed, and a toast that only reports waits behind either. `sticky` keeps any toast until dismissed, for a refusal with no other place to land. `tone` from `TOAST_TONES`: `success` (default), `danger`, `warning`, `info`, `discovery`. The `<output>` is the live region, and it never takes focus. When the action is pressed from inside the toast, focus goes back to where it was before it went into the toast, unless the action moved focus somewhere else or that place is no longer on the page. The story sets out which writes get a toast | `toast.tsx` | ✅ |
| `SurfaceState` | The nine states a surface can be in (`ready \| empty \| withheld \| unavailable \| loading \| unsupported \| failed \| stale \| partial`) as one component. `sectionState()` sorts one section of a read made of many parts, and `omitted()` asks whether a grant withheld it. Only `empty` may say "there is none". `stale` puts its note above the rows, and `partial` its count below. `emptyLabel` and `loadingLabel` are the caller's words; `label` and `labelLevel` name one part of a card. `SECTION_STATE_TONES` colours only the verdicts (`loading`, `failed`, `stale`, `partial`). `detail.withheldReason` replaces the general withheld sentence | `surfacestate.tsx` | ✅ |
| `CardBoundary` | A render boundary around one card, so when one card throws, the shell and the menus stay in place. It says the card failed, and tries again with the query cache reset. It never shows the error's text | `cardboundary.tsx` | ✅ |

### Navigation

| Primitive | For | File | Story |
|---|---|---|---|
| `.entity-link` | One of our records, named in a line and reachable: a contact in a sentence, an account in a value, a sender in a header. In the accent colour, with an underline on hover, and no chrome. It is a class, because building a route is the job of a screen. A screen uses `screens/entityref` (`EntityRef` / `RecordRef`). A design-system component that names a record puts the class on its own anchor (as `EmailDetail` does). Use `OffsiteLink` for an address outside the product, and `Button variant="link"` for an action. `newTab` on `EntityRef` sets `target` and `rel="noopener noreferrer"` together | `atoms.css` | in `Email entry` |
| `OffsiteLink` | A place off our origin: a link when `webUrl` accepts the address, plain text when it does not. It sets `rel="noopener noreferrer"` and uses `.link-button`, unless the caller passes a class. It is named for the place it goes to, so it does not clash with the `ExternalLink` icon from lucide | `offsitelink.tsx` | ✅ |
| `ContactLink` | An email address or phone number the reader can use, kept as plain text when the value is not valid (`format/contacturi` decides). Inside the product, an address opens the composer on the record the caller names, hosted once in the shell by `screens/writeto`. Without a connected mailbox, or with no composer host mounted, it falls back to a `mailto:` link. A page with its own composer answers there. A number is a `tel:` link. `readOnly` keeps the address as text | `contactlink.tsx` | ✅ |
| `RecordTabs` | The strip that chooses which body of a record is open: quiet text tabs at `--fontWeightMedium`, with the accent under the current one, across the work column. `counts` may be partial, as in `FilterPills`; `trailing` holds the control that opens the details column. A strip with one body draws it as a label, not a button | `recordtabs.tsx` | ✅ |
| `Breadcrumb` | Where the reader is, and how they got here: a named `nav`, an `<ol>`, one `<li>` per stop. The last stop is never a link. Separators are `aria-hidden` spans inside the item they follow. The last stop is cut short and a middle stop has a cap, both with `useTruncationTooltip`; under 700px a middle stop folds away. No `className` or `style` | `breadcrumb.tsx` | ✅ |

### Overlays and layering

| Primitive | For | File | Story |
|---|---|---|---|
| `Modal` | The one dialog: portalled, closed by Escape, with Tab kept inside. `intent` picks its shape. Each shape is one width token in `tokens.css`, and the box is the token or the glass less its gutter, whichever is smaller. `confirm` (`--dialogConfirmWidth`, 440px) asks before an act, and carries at most two reason fields. `form` (`--dialogFormWidth`, 600px) is a short form you fill and leave, six fields or fewer (`intentForFieldCount` picks `form` or `drawer` for a form whose fields are data, at `FORM_DIALOG_MAX_FIELDS`). `drawer` (`--drawerWidth`, 560px) sits on the end edge, for work beside the record, a longer form included. `drawer-reading` (`--drawerReadingWidth`, 880px) is that drawer for something you read or write at length (a brief, a message, a model's settings). `full` (`--dialogFullWidth`, 1240px, as tall as the glass) is the lightbox for a stored file (`FilePreview`). Drawers float with a gap and `--shadow-pop`. On a phone (720px and under), a confirm stays a card over the scrim you can see, and every other intent is a full-screen sheet. On a desktop, a `form`, or a drawer whose body is one `.form-stack` followed by `.actions`, scrolls that stack, so the title and the action row stay put; a drawer with bands uses `DrawerHead` / `DrawerBody` / `DrawerFoot`. `intent` is required and is the only box prop, so a dialog that names no shape, or any other width, does not build. Every dialog draws one way out other than Escape: an `IconAction` X in the top corner at the end side, last in tab order, and never where focus lands. `closeReason` refuses that close with a sentence; `closeDisabled` holds it while a write the dialog started is out. It moves in and out (`usePresence`), and is `inert` while it leaves. A title is a `Heading` with `modal-title`, which owns the space under it; leave out the class inside a container with a gap | `modal.tsx` | ✅ (`Confirm`, `Form`, `Drawer`, `DrawerReading`, `Full`, and the two phone frames) |
| `DrawerHead` / `DrawerBody` / `DrawerFoot` | The bands of a `Modal` placed on the right. `DrawerHead` holds the title, a `Heading` whose id the `labelledBy` of the Modal names, and anything under it that describes the whole drawer. `DrawerBody` holds the content; `DrawerFoot` the verbs. In every drawer, the head and foot stay put while the body scrolls. In a reading drawer, each band also has its own inset to the drawer's edge; in the standard drawer, the drawer's padding frames all three. `className` adds a screen's layout to a band (`actions` on a foot of Cancel/Save) | `drawerbands.tsx` | ✅ |
| `ResolveSheet` | Answering a finding from the input check that runs each night, as a `Modal` placed on the right. Which fields are required turns on the outcome, the same as the server's rules: `value_correct` and `not_relevant` ask why and offer an end date, and `remind_later` asks when. It sends only the fields of the chosen outcome. `condition_cleared` is not offered. It holds no UI text. `error` shows a refused save as an `ErrorLine`; `returnFocusTo` passes to `Modal` | `resolvesheet.tsx` | ✅ |
| `RecordFormDialog` | The dialog to add or edit a record, around a record form. `fieldCount` picks the shape through `intentForFieldCount`: a `form` with the title, the form and an `.actions` row, or a `drawer` with bands past `FORM_DIALOG_MAX_FIELDS`. The shape is read once each time it opens, so a field that arrives while it is open does not mount the form again. `form` and `actions` are the caller's; the dialog owns the title and the box | `recordformdialog.tsx` | ✅ |
| `ConfirmModal` | A dialog that asks before something that cannot be taken back without loss. A write whose one reverse call brings back the prior state with nothing lost runs at once, and offers Undo through the toast instead (hiding a deal file, taking a tag off a record, taking a record off a Shortlist). A reverse that stamps the provenance again, drops a note, needs a reason, reaches an outside party or changes a user's access keeps the confirm. It owns the heading, the error line and the Cancel/Confirm pair. `confirmDisabled` refuses as a state; `confirmReason` refuses with a sentence, through the `reason` of `Button`. Pass one or the other. It is a `confirm` unless `intent` names another shape. While `pending`, Escape and the backdrop wait for the write, and the X in the corner is disabled, as Cancel is. The footer wraps only between `actionsLead` and the Cancel/confirm pair | `confirmmodal.tsx` | ✅ |
| `NamePrompt` | A write whose only input is a name: trigger, dialog, name box, save. `icon` leads the trigger. It refuses an empty name and clears the box once the write works, using the chrome of `ConfirmModal`. `onSave` gets a `done` callback, so a refused write keeps the dialog open with the text in it | `nameprompt.tsx` | ✅ |
| `OverflowMenu` | The verbs a record offers but a reader seldom wants. Items are words with no glyph; `overflowmenu-icons.test.ts` fails a glyph inside one. On a record page the order is Edit, Merge, Share, Full history, the record's own verbs, then Archive. Verbs that are only an icon stay outside, as `IconAction` | `atoms.tsx` | ✅ |
| `Popover` | A short note on click, portalled beside its trigger, for a paragraph or a small list that may carry a link. Use it where a `Disclosure` would push the page around; use `useTruncationTooltip` for one line on hover. It closes on Escape (and gives focus back once it has closed) and on a click outside it. `disabled` refuses to open, but never closes an open panel or disables an open trigger; `reasonId` refuses the same way and names why. `open` / `onOpenChange` hand the open state to the caller. `dialog` makes the panel a non-modal dialog of controls, as the one in `ListPopover` is. No `pending` | `popover.tsx` | ✅ |
| `ListPopover` | A pick that is the whole act (a tag, an owner, a project), as a search over a list tied to its trigger. The panel is a non-modal dialog: a `role="combobox"` search drives a listbox through `aria-activedescendant` (arrows, Home and End walk it past a disabled row, Enter picks, Escape closes and gives focus back). `options` is a list in hand, filtered by name and any `keywords`, and `undefined` while it loads. `search` asks the server once per finished search term that is not empty, and offers no row from a term since typed again. `onPick(option, done)` closes on `done`, so a refused write stays open over its `error`. A second pick is refused until `pending` falls or the panel closes. `selected` marks the current one; `empty` and `footer` carry the caller's words, and one live region that stays in place reads out searching, the result count and an empty answer. `open` / `onOpenChange` as on `Popover`. At or under the fold (720px, where `Modal` turns into a sheet), it is a `drawer` `Modal` sheet with `title` at its head. A field's value is a `Select`; a pick inside a form is a `RecordPicker` | `listpopover.tsx` | ✅ |
| `FilePreviewProvider` / `useFilePreview` | One stored file opened over the page, which goes dark, with save, print and close. `FileChip` opens it. The bytes are fetched once into a blob, and the media type comes from the file name, checked against the allowlist of `previewMediaType` in `filechip.tsx`. So no type that can run a script (HTML, SVG) runs under our origin. A PDF goes in a frame and prints itself. An image goes in an `<img>`, and the page prints it through the `@media print` rules on `.modal-full` and `.file-preview-*`. A kind nothing can draw, or a click with a key held down, keeps the download. One provider in `main.tsx`, held by `conformance.test.ts` | `filepreview.tsx` | ✅ |
| `useTooltip` | A control's own name on hover and on focus, for a control that cannot draw it, or one short fact about the control (who put a tag on a record, and when). It lands in `aria-describedby`. It works the same way as `useTruncationTooltip` | `tooltip.tsx` | in `Icon action` |
| `Menu` | The popover panel that `ListSurface` hangs its sort, filter and column sets in: a `fieldset` with a heading; `align` picks the edge it opens from | `listsurface.tsx` | in `List table` |
| The sort menu | Every field the server can sort the list by, hidden columns included, and the server's own default order. `ListTable` builds it from its columns, and the header and the menu both read `nextSortValue` | `listsurface.tsx` | ✅ (`List table → SortedByAMenu`) |
| `useTruncationTooltip` | The whole of a string that was cut short, on hover or focus, and nothing when it fits. Use it wherever user data of any length is drawn on one line | `tooltip.tsx` | ✅ |

### Status indicators

| Primitive | For | File | Story |
|---|---|---|---|
| `CommunicationStatus` | Whether a message can go, shown where the send is decided. Six states, told apart by shape: `context_only`, `ready` (`MailCheck`), `attention` (`MailWarning`), `restricted` (`MailMinus`), `checking` (`BusyMark`, no envelope), `external`. `scope` sets the meaning: `contact_preferences` (never reaches `ready`) or `current_message`. `label` and `name` are required and translated. `exception` never changes the colour of the mark. `CommunicationStatusLine` puts the mark beside the verbs, never inside one | `communicationstatus.tsx` | ✅ |
| `StageStrip` | The shape of a pipeline at a glance: open stages from left to right, shaded by the chance to win, with won and lost at the end. A reading, never a control (`StageLadder` moves records). `compact` is the bar for looks in a list row; `empty` is the label while there are no stages | `stagestrip.tsx` | ✅ |
| `StrengthMeter` | How strong a relationship is, as three bars that rise over its word: `strong`, `moderate`, `weak`, `none`. The word is the fact. The contact Routes panel shares `strengthmeter.css` | `strengthmeter.tsx` | ✅ |
| `PipelineLadder` | The path of one message through the ingress pipeline, with what each step did and why. It draws whatever stages the server sends, using the server's `label` / `reason_text` for one it does not know. Its statuses keep `not_applicable` apart from `unknown`, and `withheld` (always drawn for a reader who is not the owner) apart from a step that was left out | `pipelineladder.tsx` | ✅ |

### Text and data display

| Primitive | For | File | Story |
|---|---|---|---|
| `EmailEntry` | One kept email as a row, and the one reading of a message in the product. It takes the server's `EmailSummary` and a formatted time; there are no density or variant props. `onOpen` opens the reader; `onSelect` and `selected` let a composer pick a message to reply to. It owns its padding, type, cutting short and focus ring. A withheld row keeps its shape and loses its words | `emailentry.tsx` | ✅ |
| `EmailDetail` | One email read whole, in the drawer form of `Modal`, over the record. `open` is true by default. When closed, it stays mounted so it can move out, and opening it again fetches it again. It fetches on open only. Quoted history and the sign-off fold behind a control. A withheld message draws the `withheld` state of `SurfaceState`. Participants it knows and records the message is filed on are links that open in a new tab. Record names and the reply composer come in as render props | `emaildetail.tsx` | in `Email entry` |
| `OpenEmailDrawer` | The one email drawer of the record page: `EmailDetail`, plus the state that keeps the last message in place while the drawer closes. Mount it on the record, not inside a tab. It binds the access editor, filed links and reply verb, so every surface that opens a message offers the same three | `openemaildrawer.tsx` | in `Email entry` |
| `EmailText` | Full, cleaned email text with paragraphs, a sign-off in view, and quoted history that opens, shared by the email reader and the composer preview | `emailtext.tsx` | ✅ |
| `EmailWords` | A message's words alone (the server's preview line), for a thread card that already draws the sender and time. It follows the withheld rule of `EmailEntry` | `emailentry.tsx` | in `Email entry` |
| `EmailReference` | A short pointer to an email: subject, date and first words, with no preview and no access badge. For naming a message inside another layout (a list in time order, an evidence row of a brief, a receipt in the graph). `stacked` puts the date under the subject, for a small column | `emailreference.tsx` | ✅ |
| `CellStrip` | More than one pill in one table cell, on one line: text beside a badge is cut short first, then each badge. It never wraps. Use `RowTags` for a tag column | `listtable.tsx` | ✅ (`List table → Cell strips`) |
| `Markdown` | A document from the knowledge corpus, read-only, with the cited part marked. `markdown-parse.ts` returns data with a closed set of shapes, and this file builds every element, so raw HTML shows as text and never runs. A link is an `<a>` only for `http`, `https` or `mailto`; any other link shows its label. Tables go in `TableScroll`, named by their header row. It supports the syntax the shipped handbook uses; anything else shows as source text. `highlight` reports through `onHighlight`: the quote found and marked (with white space joined, as `claims.CollapseSpace` does), the `line` with a band, or `"none"`. `autolink` turns a bare `http(s)` address into a link with itself as the label. It shows a link with a label as that label, followed by the real address as the link. The timeline draws a note body this way, while a mail body stays plain text | `markdown.tsx` | ✅ |
| `InlineMarkdown` | One line of markdown, in the line: bold, italic, code and links, using the parser and element list of `Markdown`. For the answer a model wrote. `links={false}` keeps link labels and drops the link targets, for prose that nothing checks against a quote. Use `Markdown` for headings or tables | `markdown.tsx` | ✅ |
| `FactList` | Pairs of label → value that a reader scans. Rows come in as an array, so a caller drops missing facts instead of showing blanks | `factlist.tsx` | ✅ |
| `ProjectLinks` | The one section where a record shows its projects, and where a reader adds or removes one. The record page gives it an adapter (what is linked, how to link and unlink, whether more than one is allowed). The component owns the surface, verbs, and the empty, confirm and refusal states. `allowsMany: false` turns "Attach" into "Move". The project page uses it for its companies | `projectlinks.tsx` | ✅ |
| `Heading` | The one spelling of a heading. `size` (required) is one of the seven `--fontHeading*` steps; `as` replaces the default element (`xxlarge` / `xlarge` → `h1`, `large` → `h2`, `medium` → `h3`, `small` → `h4`, `xsmall` → `h5`, `xxsmall` → `h6`, plus `div` and `span`). No margin and no colour. Other attributes pass through. `heading-spelling.test.ts` fails a raw `<h1>`–`<h6>` under `src/`, and holds `heading()` in `mcp-apps/bridge.ts` to the same table | `heading.tsx` | ✅ |
| `Kbd` | A key cap, in the body face: a key someone presses is not code | `atoms.tsx` | ✅ |
| `StatCard` | One reading with its basis. `label` and `value` are required strings; an empty reading says "0" or "None yet". The figure is `--fontHeadingLarge` with tabular digits on one line; the label is one line in `--textSecondary`; `detail` is two lines in `t-caption`. `tone` from `STAT_CARD_TONES` (`info`, `success`, `warning`, `danger`) colours the figure in `--<state>Text` and the `meter` in the base. `alert` tints the tile. `basis` opens a `Popover` of source rows, from a trigger on the label line. `onOpen` adds the fixed "Open →" door; the whole card is its press target. `href` draws the same door as a link, for a reading that names one record. `narrow="row"` folds the tile into a line in a small `StatStrip`. `source` may hold an `IconAction inline`. `statcard.test.tsx` holds the ink and the door | `statcard.tsx` | ✅ |
| `StatStrip` | A record's readings as one plate of slots with rules between them, read across as one set to compare. It takes `StatCard` items and owns only the plate: the slot count from the children it draws, the rules between slots, and the fold (the last slot fills its row). A slot may be a `button` that wraps the card, for a strip of filters; a reading that opens something uses `StatCard onOpen`. `floor` carries a note for the whole row. Small readings fold into full-width rows in small containers, with the text and the action each on their own line | `statstrip.tsx` | ✅ |
| `ReadingsFloor` | The sentence a row of readings shows when its source was read up to a limit, so every figure is a floor. It belongs to the row; `StatStrip` and `ReadingsGrid` both draw it through `floor` | `readingsfloor.tsx` | in `Readings grid` (`TheFloorAlone`) |
| `ReadingsGrid` | A record's readings as cards that stand on their own, each used alone with its `basis`, `meter` and `onOpen`. It owns the row: `auto-fit` columns on a 9.6rem floor, and every card as tall as the tallest | `readingsgrid.tsx` | ✅ |
| `Meter` / `Sparkline` / `Chip` | A share as a bar (`value` and `max`, never a percent), a short run of values as a bare line, and one field of a record as an icon pill (a fact that may be a link). `Meter`: `tone` colours the fill; `restTone` gives the track its own value; `dense` is the thin label bar; `part` draws a stricter measure inside the fill as its solid head. `Chip dense` matches the 20px shape of a badge, for table rows. Size comes from the props, not from a screen sheet | `readings.tsx` | ✅ |
| `CumulativeChart` / `BulletChart` / `RangeChart` / `GroupedBars` | Report readings on shared axes that start at zero: event curves for this and the last period with a target line, an owner's number against the target, spreads from the middle value to `P75`, and pairs of event counts that do not depend on each other. A missing value leaves a gap. The caller gives the formatted readings and the evidence callbacks; each chart has a table of numbers you can open. Without a label to compare against, the legend and controls for comparing are left out. A bullet chart draws a target marker only for a set target; a missing target is not zero | `report-charts.tsx` | ✅ |
| `BarList` | Many readings with one value each, as a ranked list on one scale: label, bar, formatted amount per row. `max` names a whole the rows do not reach; a `max` below the largest row is ignored. Amounts come in formatted. Bars are `aria-hidden`, and an `sr-only` table carries the values. Long stage and team member labels wrap inside their column. With `onSelect`, each full bar becomes an evidence action a screen reader can use | `readings.tsx` | ✅ |
| `SegmentBar` | Parts of one total that do not overlap, on one track, with a `marker` if needed for a figure the parts are read against (won, evidence and best case, with the call marked). One to three parts, from most to least sure, in steps of one hue from `--accentText`. The legend names every part and the mark; the track is `aria-hidden`. Amounts come in formatted. With `onSelect`, each part becomes a real button | `readings.tsx` | ✅ |
| `Waterfall` | A bridge from one total to the next that adds up: the start and end totals start at zero, each signed move starts at the total before it, and the links between them show the flow. One scale covers every total in between and every value below zero. Amounts in view and a table for screen readers come with the shapes. A selection callback, if given, adds evidence actions you can reach by keyboard. The product checks that the bridge adds up; a drop uses amber, because leaving open pipeline is not always a bad outcome | `waterfall.tsx` | ✅ |
| `FileChip` | One stored file as a small card: a kind glyph, the file name, and a click. The click opens `FilePreview` for a kind the browser can draw, or downloads one it cannot. PDF and images have their own glyphs. The email attachment shelf and the account and contact file lists use it. It takes either `href` or `withheld`, never both. `withheld` is the sentence for a file known by name only, such as the attachment of a private message whose bytes were never kept. It draws the same card, dashed and quieter, as text and not as a link | `filechip.tsx` | ✅ |
| `DataTable` | A plain table of columns and rows, with a way to open a row if needed, inside a `TableScroll`. `label` is required, and matches the heading above the table. `align: "end"` puts a column of figures on the right, in tabular digits; `grow` names the column that takes the free width | `datatable.tsx` | ✅ |
| `CellStack` | Two facts in one table cell, one under the other: a value, and the fact that limits it (a failure marker under its time) | `cellstack.tsx` | ✅ |
| `TableScroll` | The box a table that is too wide scrolls inside, to the side, so the page does not. Use it for any `<table>` drawn by hand; `DataTable` already does. It takes a tab stop and becomes a named `region` only while its content does not fit. `label` is required | `atoms.tsx` | ✅ |
| `RelationshipMap` | The account's routes as a fixed drawing in three columns: our colleagues, the account and its deal, and their contacts in role lanes. It takes data only; strings come in translated. Selecting a contact lights the strongest route, and the panel names it and the other routes. A fixed layout, with no motion model. It gets a little smaller, then scrolls; the side panel moves under the drawing by container query. Strength is the width of the line (the weakest is dashed), kind is shape, and how engaged they are is a word. One tab stop; arrow keys walk lane by lane | `relationshipmap.tsx` (geometry and route arithmetic in `relationshipmap.layout.ts`) | ✅ |
| `PipelineBoard` | The pipeline surface: one column per stage, each with its money head and the `DealCard` items in it. A stage is 300px (`--board-col-w`) above the 700px phone breakpoint, so a 1440px screen shows part of a fourth stage. On a phone a stage takes four fifths of the screen, and the board moves by swipe. `mailAside` and `cardActions` (`DealCardHooks`) are what a caller hangs on every card | `composed.tsx` | ✅ |
| `DealCard` | One deal on the board. The company slot: `company` names it, `companyWithheld` draws the mask of `FieldGuard`, `companyUnreadable` says the lookup failed, and when none of them is set it draws nothing. The lines keep one order: company and owner as a caption, the name cut to two lines, the short figure with the close date, then the foot. The foot is how the deal is moving: the stall, the mail line (`lastEmail`) and single-threaded. `mailAside` makes the mail line a hover `Popover` trigger; the line is `DealMailChip`, exported for the deals table. `actions` (`DealCardActions`) adds up to three named `IconAction` items (summary, email, add task) at the end edge of the foot, as bare glyphs, and one left out is not there | `dealcard.tsx` | ✅ |
| `IdentityLine` | The line of facts under a record's name. `separator`: `dot` for parts of one sentence (account, deal), `space` for separate handles (a contact's address, number, profile). It drops missing children, so no dot is left alone. `IdentityFact` is one fact with a glyph if needed (`quiet` for a fact that limits another); `IdentityMeta` stacks two lines | `identityline.tsx` | ✅ |
| `RecordFacts` / `Fact` | The strip of facts in the head: caption over value, cells that wrap as the head gets smaller, as one `dl`. `Fact` sets its `label` in `Eyebrow` over a `dd`. Which facts, and in what order, stay the caller's | `recordfacts.tsx` | ✅ |
| `RecordCard` | A record listed in another place (an account's contacts, search results) as its mark, name, `position`, and handles a reader can use. `kind` (company or contact) and `identity` key the mark; `logo` passes to `Avatar`. The mark and name share one link, and each handle is its own control; the card is not one link. `aside` is what the surface knows (which colleagues reach this contact). `.record-card-list` is the stack. A name in a sentence is `EntityRef` | `recordcard.tsx` | ✅ |
| `GroupedTimelineList` / `TimelineList` / `TimelineRow` | The activity timeline, in groups. Every row carries its day and time in the gutter. A thread is one open card: kind, count, participants and whose move it is, over the subject. Then come the messages (the same `TimelineText` fold), with those past the first three behind a count. A send to many contacts folds behind its newest copy, drawn through `EmailEntry` | `composed.tsx` | ✅ |
| `CountUp` | A number that climbs while a count is still being made, from where it was and not from zero. Tabular figures. Under reduced motion it shows the number at once. Not for a figure that is already final | `countup.tsx` | ✅ |
| `CrawlCanvas` | The site drawn as it is read, on the screen where a reader waits for a crawl. Pages hang off the pages that linked to them; indigo marks current work, and a read page cools to grey. It draws only what the server sent. In effect it is `aria-hidden`; the ticker and the counters carry the words. The shapes live in `crawl-graph.ts` | `crawl-canvas.tsx` | ✅ |
| `Eyebrow` | The small title over a section: the element carries `.t-eyebrow` from `base.css`. `as` picks `h2` / `h3` / `h4` / `span` / `dt`. A place that has only a selector (`.firmo dt`) can reach the same rules | `eyebrow.tsx` | ✅ |
| `ListTable` | A record list as a table: columns, rows, query dials above, footer below. Header cells are sentence case at `--fontWeightMedium` (from `app.css`). The screen owns sort, filter and search. `selection` adds row checkboxes and a bar for acting on many rows; `SelectionBar` (`selectionbar.tsx`) is that bar for a list that is not a table (the tasks and commitments of the Worklist). A column declares its kind: `fixed` (identity), `numeric`, `verbs`. `initiallyHidden` starts a column with its tick off. Columns stop at their smallest width, and the body scrolls to the side. `page` with `onPage` lets the address own the page; a narrower list goes back to page one. `bodyRef` gives access to the scroll element (`app/scrollmemory.ts`). See [Building a list screen](#building-a-list-screen) | `listtable.tsx` | ✅ |
| `ListSurface` | The chrome `ListTable` draws into, which you can use alone: saved-view tabs, the count line, search, filter chips, sort, the archived switch, footer. The left half narrows the list (search, filters, Filter, archived); the right half changes how it is drawn (Sort, `displayMenu`, `tools`, Save view). Every verb is a `Button` | `listsurface.tsx` | ✅ |
| `CountLine` | The sentence under a list that says what is on screen out of what exists, and the sort. It spells "23 of 1,204"; `more` stops an unknown total from reading as known | `listsurface.tsx` | in `List table` |

### Primitives

| Primitive | For | File | Story |
|---|---|---|---|
| `Stack` / `Row` | Space between things, from the scale, and nothing else (no ground, border, padding or type). For extension units, which cannot import CSS and so cannot write a class. `gap` names a spacing step. `Row` adds `align` and `justify` (`between` puts a label left and a verb right), and wraps by default. A box you can see is a `Card` or a `Panel` | `stack.tsx` | ✅ |

### AI and provenance

| Primitive | For | File | Story |
|---|---|---|---|
| `SourceEvidence` | The thing a task was read out of. It finds the kind of the source activity through the plain activity read. Then it draws an email in place through `SourceEmailPanel`, or offers a button for a meeting transcript, which opens through the host's `onOpenTranscript`. Nothing is drawn until the kind is known | `sourceevidence.tsx` | ✅ |
| `SourceEmailPanel` | The message a task was read out of, drawn inside the task panel instead of in a second drawer. It makes its own read under `emailDetailKey`, so the server checks access again. Its envelope (From, To, Cc, the date and the note that Bcc is withheld) is `EmailEnvelope`, exported from `emaildetail.tsx` and drawn there too. So both readings name the parties to a message one way. Without it, a reader took the header lines of the quoted history for those of the reply. No editor for who may read it, no filed links, no reply verb. A withheld message shows nothing below the state, the subject and date included | `sourceemailpanel.tsx` | ✅ |
| `EvidenceReceipt` | What a number was drawn from: how many records could count, how many carried the values it needs, and what the figure does not cover. `state` is a one-word `Badge`, missing until the caller has one. `calculation` and `calculationSummary` come together or not at all. It only draws; forecast, pipeline and agent reports use it | `evidencereceipt.tsx` | ✅ |
| `AiPending` | The wait for a model's answer: an indigo tile that pulses, answer lines of uneven length, and a light that passes over them, with the sentence shown and read out. It is built on `PendingBody`; under reduced motion it rests. Only for an agent that fills a card; a plain fetch is `PendingBody` | `aipending.tsx` | ✅ |
| `EvidenceMark` | The provenance control: a dotted underline on a value a human did not type, which opens to where it came from. On a value that is already a control, it sits beside it with a word ("read", "bought"). `subject` names the value, and the name a screen reader reads starts with the word in view (WCAG 2.5.3) | `evidencemark.tsx` | ✅ |
| `AutonomyDot` | The meaning of the 🟢/🟡 autonomy marks as a token component, never an emoji glyph | `trust.tsx` | ✅ |
| `EvidenceChip` / `ConfidenceMeter` | How sure we are, and on what evidence. Inside `EvidenceMark` and on the staging surfaces, never stacked under a field. `evidence.lines` adds line numbers that start at 1: numbers that follow each other join into a range (`lines 12–14`), and gaps stay listed | `trust.tsx` | ✅ |
| `ProvenanceTag` / `provenanceLabel` | Where a value came from, and who captured it: a `Badge` with one label per kind of actor (agent, connector, system job, human, buyer, not recorded). Only `agent` takes `tone="ai"`. `agent`, `system` and `buyer` may come with no name, and then show the kind. `buyer` is a Deal Room participant from outside the company | `provenance.tsx` | ✅ |
| `confidenceLevel` | The confidence bands: a wire value from 0 to 1 mapped to the level `ConfidenceMeter` draws, high at 0.8 and `med` at 0.5, both included. Null in, null out: a confidence nobody recorded is not low | `trust.tsx` | ✅ |
| `StagingCard` | The staged, not yet real, state, on the staged kind of the `panel-ai` family in `panel.css`: dashed until a human accepts it. `DecisionCard` and the deal board use this class | `trust.tsx` | ✅ |
| `FieldDiff` | The old → new value change, in the line; a null side reads as a marker, never a blank | `trust.tsx` | ✅ |
| `PassportChip` | An agent's passport id on a soft `ai` `Badge` | `trust.tsx` | ✅ |
| `DecisionCard` | The card that asks a human to decide something an automation staged, in two layouts: `deck` (tall, the whole payload) and `row` (short list form). It shows the proposed content: a draft's `subject`/`body`, or `current_X`/`proposed_X` through `FieldDiff`. `expires_at` tints the edge in three bands from `decisionUrgency`, and past the deadline Accept is not drawn. Verbs split on `ActionRow`: Accept on the end edge, the rest as `IconAction` items. Verbs are callbacks and words are `labels`; `meta`, `aside`, `editor`, `detail` and `display` (decided per kind in `screens/approvalkind.ts`) give the rest. `compact` with `DecisionCompactWords` gives one line per decision, with the proposal in a `Popover`. Inner parts: `decisioncard.payload.ts` (`draftOf`, `diffsOf`, `restFields`) and `decisioncard.content.tsx` (`DecisionContent`, `DraftBody`, `DecisionEvidence`) | `decisioncard.tsx` | ✅ |
| `DecisionStatusChip` | The decision's header chip: the verdict once answered, or else the live count down. It reads `decisionExpiryMs` / `decisionLapsed` / `decisionUrgency`, so it steps up on the same bands as the card's edge. A pending proposal that has run out of time gets no chip. `labels.expiresIn` builds the sentence; an unknown status reads as out of time. It takes a `DecisionDeadline` (`status`, `expires_at`) | `decisioncard.tsx` | ✅ |
| `DecisionToolChip` | Names the tool that staged a proposal (`send_email`), apart from its kind (`advance_deal`). The verb and words come from the caller. No verb, no chip | `decisioncard.tsx` | ✅ |
| `decisionUrgency` / `decisionUrgencyTone` / `decisionExpiryMs` / `decisionLapsed` | The deadline words that the card, its chip and every count-down badge read. `decisionUrgency` owns the `1h`/`6h` bands; `decisionUrgencyTone` maps them to a `Badge` tone | `decisioncard.tsx` | ✅ |
| `DecisionDeck` | The queue of staged decisions, answered one at a time. Stage, then commit: a swipe or a key stages it on this device, the tray shows what is waiting, and nothing is sent until you commit, which makes the tray the undo. Drag and arrow keys go through `dragVerdict`/`keyVerdict`; `U` takes a stage back, `Enter` commits. A `bundle_id` group is one decision. `SegmentedControl` switches to the list form, the default under `prefers-reduced-motion`. `title` puts the heading on the switch's row; `frame` lets a `Panel` place the switch and content. `listCap` / `listRest` cap the list form. Parts: `DeckItemCard` (`decisiondeck.item.tsx`); `decisiondeck.verdicts.ts` (`dragVerdict`, `keyVerdict`, `sharedFacts`, `deckKeyHandler`, `verdictSends`); `decisiondeck.stack.tsx` (`DeckStack`, `useDeckDrag`); `decisiondeck.frame.tsx` (`DeckSurface`, `DeckQueue`, `StagingTray`, `useCommitTakesFocus`). In a panel, a queue whose every card is staged draws nothing | `decisiondeck.tsx` | ✅ |
| `MarginceCoreScene` | The product's AI identity: a glass ball with four ribbons, drawn on the GPU, in five lifecycle states (`idle` · `ingest` · `working` · `warning` · `error`). The state is motion first (`margince-core-motion.ts`) and colour second. `aria-hidden`, no click. Callers pass `state`; `size` picks hero or chrome, and `surface` dark or paper; it is sized through `--coreSize` / `--coreGlass` / `--coreHalo`. Without WebGL2 it draws a still image. `margince-core-shader.ts`, `margince-core-gl.ts` and `margince-core-engine.ts` are inner parts | `margince-core.tsx` | ✅ |
| `AiRuntimeChip` | Which model answered, how many calls, and what it cost. The cost always shows; hover shows the cost per model, and a press pins it. That list scrolls inside the space it has (`useScrollRegion` in `scrollregion.ts`), named by its heading, with Escape giving focus back | `airuntimechip.tsx` | ✅ |

### Libraries

| Primitive | For | File | Story |
|---|---|---|---|
| `useRecordTimeline` / `useTimelineFilters` | The query and filter state of the activity timeline. `timelineQueryParams` builds the request, `hasTimelineFilters` says whether anything is narrowed, and `dayStartIso` finds where a day starts in the workspace time zone. `enabled: false` holds the read until the record's own read allows it (the contact's Meetings tab reads `kind: "meeting"` this way) | `recordtimeline.ts` | — |
| `usePrefersReducedMotion` / `useTypeStream` / `useDocumentIntro` | Motion, with one rule: reduced motion jumps to the end state, never to nothing | `motion.ts` | — |
| `useDialogFocus` | What a dialog owes the keyboard: Escape closes from any place inside, Tab stays in, and focus goes in on open and back to the opener on close. `Modal` and the `⌘K` palette use it. `container` is the dialog's box, not its overlay. Only the dialog on top owns Escape and Tab. `initialFocusTo` picks a field; `returnFocusTo` names a new opener. A popover, menu or sheet asks `coveredByDialog(itsTrigger)` before it answers Escape | `dialogfocus.ts` | in `Modal` and `Shell → Command palette` |
| `useArrivalFocus` | A page opened by a press that took its control off the page: focus would fall to `<body>`, so the element of the `ref` (the page title, with `tabIndex={-1}`) takes it on mount. Focus the reader put in any other place stays there. The CSV import run page and the `vCard` import page use it | `arrivalfocus.ts` | in `Settings/Data/Data import/Import` and `Patterns/vCard import` |
| `useSettledValue` / `useDebouncedSearch` | Typing as a question asked once per pause (`SEARCH_DEBOUNCE_MS`). `useSettledValue` holds back a value to use as a `react-query` key, so the caller keeps its own query, cancel and error. `useDebouncedSearch` runs a `{ value, label }` search, and reports pending, failed and the last answer. The `⌘K` palette, the audit log filters and the consumer mail domain search use the first; `ListPopover` and the filter builder use the second | `debouncedsearch.ts` | — |
| `usePresence` | Keeps a closing surface mounted until its exit motion ends, by waiting on `getAnimations({ subtree: true })`, so it needs no time value. It returns `{ mounted, state }`; put `state` on the element as `data-state`. With nothing moving (reduced motion, `jsdom`) it leaves the page at once | `presence.ts` | in `Modal` |
| `useUnsavedGuard` / `UnsavedGuard` / `useGuardedLeave` | Unsaved edits, and leaving without saving them. Whatever holds a draft calls `useUnsavedGuard(dirty)`; `UnsavedGuard` wraps the surface, and keeps it on screen while it asks. A reload uses `beforeunload` (set only while there are unsaved edits); a move inside the app gets a `ConfirmModal`. Escape and the backdrop keep the edit. It covers only the surface, so Back and Forward are not stopped. A page whose save, or its own question, moves it on calls `useGuardedLeave(dirty)` instead. It claims the guard as `useUnsavedGuard` does, and returns the way off the page. That way drops the claim, and moves on only once the guard reads it as released, so a plain `navigate()` never runs ahead of the guard | `app/unsaved.tsx` | — |
| `useUrlParams` | The query half of the address, and the one way to read or change it. The path names what is on screen (`app/router.tsx`); this names which view, so `routeIdentity` does not read it. Writes replace the history entry, so Back from a record brings back the list. It returns a `ReadonlyMap`; `currentParams()` reads it outside a render. The list mapping is `listQueryFromParams` / `paramsFromListQuery` in `screens/listquery.tsx`, bound in `useListQuery`. A screen's own drawing dial goes in `screenDials`; the deals board passes the same names to `withoutScreenDials` / `mergeScreenDials` | `app/urlstate.ts` | — |
| `subscribeToWindowFocus` and the rest of its group | Whether this window has focus: one signal for the draw loop and the stylesheet | `window-focus.ts` | — |
| `readStored` / `writeStored` / `forgetSeat` | Web Storage, reached one way. Every key is declared in `STORAGE_KEYS` with how long it lives: `device` (kept by this browser), `seat` (dropped on sign-out), `account` (named by its account). A browser that refuses storage reads as empty. `storage-spelling.test.ts` fails any other file that names `localStorage` or `sessionStorage` | `app/storage.ts` | — |

## Building a list screen

A record list is `useListQuery` bound to `ListTable`, and a screen's job is to declare what its
records are. `screens/listquery.tsx` holds the binding. `screens/products.tsx` is the shortest full
example, and `ListTable → Default` / `Paged` / `SortedByAMenu` in `listtable.stories.tsx` show the
surface on its own.

Each of the four dials asks the server; none of them cuts up what is already on screen. The list is
a keyset cursor over a set larger than what is loaded. So a table that put its own page in a new
order would give a false picture of the other pages.

| Dial | Where it lives | The rule |
|---|---|---|
| Search | `q`, after a pause set by `SEARCH_DEBOUNCE_MS` | `searchable={false}` for a list whose GET has no `q`, and then no box is drawn |
| Sort | `sort`, one field plus the house tie-breaker | A column declares the server field it sorts by; that makes its header live, and puts it in the sort menu. A screen may not name an order its endpoint cannot give (`screens/list-sort-claim-coverage.test.ts`) |
| Filters | one wire parameter per chip | A chip's key is the parameter name, so the address and the endpoint use one set of words. `dataChips` is for options the server names at run time |
| Page | `page` + `per` in the address, `cursor` on the wire | The reader keeps their place when they open a record and press Back. A read fetches a whole multiple of the page size through `listFetchLimit`, so the pager can offer numbers without asking the server again (`screens/list-page-size-coverage.test.ts`) |

What the binding gives you:

- Every dial is in the address, so a narrowed list is a link someone can paste.
- Back from a record returns to the page and scroll place it was opened from.
- One `CountLine` sentence for what is on screen out of what exists. It says "loaded so far" instead
  of making up a total that a keyset cursor cannot know.
- The reader goes back to page one when a narrowing changes what page one means, and not when they
  only arrive.

Where a screen uses it:

- Do not draw a dial the endpoint does not answer. Leave it out; do not disable it. `/partners`
  offers a `sort` its handler never reads, and a screen that trusted it showed a "Newest" tab over
  rows in `uuid` order.
- **Two lists on one route need `paramScope`**, or they share one parameter space and send each
  other's fields to the wrong endpoint. A list that owns its address alone keeps bare names, so
  `#/companies?q=acme` stays a clean link.
- **Name a screen's own drawing dial in `screenDials`** (board or table, which pipeline's board). It
  belongs in the address and not on the wire. If not, it is sent as a filter, counted as a narrowing,
  and wiped by "clear filters".

`emptyNote` is the sentence a screen adds when it knows why its list is empty. It is drawn under the
narrowed line as well as the bare one, because the common case is narrowed. An example is a "Mine"
tab for a reader who owns nothing. The caller decides which empty list a note explains. `mineEmptyNote` in
`screens/recordlist.tsx` is that sentence for the lists cut down to the owner. A caller whose note
would blame the data source for what the reader's own dial did passes none.

## Absent, disabled, or withheld: the cause decides

A surface a reader cannot use is in one of three states, and the cause picks which. An absent card
and an empty card look the same on screen, and mean opposite things.

| Cause | State | What the reader gets |
|---|---|---|
| It does not apply here: a posture, a rollout flag, a feature this installation does not have | absent | Nothing. There is no fact to report. |
| A need the reader could meet is not met yet: nothing selected yet, a write still running, delivery not set up | disabled | The control, inert, and what would make it live. |
| A permission denies it | withheld | The surface keeps its place, and says that it is withheld. |

The third row is the one code breaks most, because to return `null` on a denial is the shortest code.
It makes a false statement. A retention card that is gone for an ops seat reads as "this
installation keeps nothing". An audit trail that is not there reads as "nothing has happened here".
Both are claims about the data, made in place of a claim about who may see it.

Four rules follow:

- **A withheld card asks the server for nothing.** The answer is already known, so keep
  `enabled: canRead` on the query.
- **Gate on the probe's answer.** A `/me` read that is still running is not a denial. To branch
  before it answers shows the notice to every reader for a moment.
- A write control inside a surface you can read may be absent, if the surface states once that it
  is read-only. `auto.readOnly` and `cf.noPermission` are the pattern. To withhold twelve buttons
  one by one is noise; to leave out the page's one explanation is the defect.
- A surface that is only an action may be absent on a denial. A card that holds no fact cannot be
  read wrongly as "zero" or "nothing happened". An absent Reset-data card says nothing about the
  installation, while "you may not reset this installation" is noise on every page. A surface that
  reports any fact at all does not count here.

**`SurfaceState` is the primitive for the third row**, and for the other states a surface can be in
that are not content. Use it before you build a message line by hand. It already words withheld,
unavailable and unsupported in different ways, and lets only `empty` say there is none.

Other primitives that carry this:

- The `reason` of `Switch` draws the explanation, points the control at it with `aria-describedby`,
  and refuses the switch by itself. The caller cannot undo it with `disabled={false}`.
- The `reason` of `Button` does the same for an action. It disables the button, draws the sentence
  beside it, and sets up `aria-describedby`. No screen reader reads out a `title` on a disabled
  button, and a disabled button cannot take focus. `reasonId` points more than one refused control at
  one sentence already on the page.
- `FieldGuard` covers a withheld value, not a withheld surface.
- In other cases, hand the sentence to `EmptyState` (`<EmptyState>{t(…)}</EmptyState>`) where
  `SurfaceState` does not fit.

The sentence carries no wrapper and no type class of its own. The plate declares both, and a
`t-caption` or `t-sub` inside it overrides the primitive from where it is used. A sentence shown
instead of content is card body text, and `SurfaceState`, `EmptyState` and the `.empty` plate all
draw it that way.

`Switch` and `Checkbox` follow from the same rule. A `Checkbox` states an intent that something
later submits, and a `Switch` is the action. A control that writes when you turn it, but reads out as
a checkbox, tells the reader something false about what their next click does.

That pair also answers a control with a state that a permission denies. There, the control is the
only place a reader can see the setting's value. Absent would hide a read the reader is allowed, and
to withhold the surface would hide the fact. A `Switch` that carries `reason` shows the state,
refuses the change, and says why, with the explanation tied to the control.

## Seeing them

```sh
cd frontend && pnpm storybook      # the catalog on :6006, light and dark
```

The Theme control in the toolbar switches `data-theme` the same way the shell does, so every token
reads its value again. Check both themes before you call a surface done. Stories live beside their
component as `<name>.stories.tsx`; the capture gate for changed files
(`frontend/scripts/fe-uat.mjs`) keys on that.

The roots of the Storybook sidebar stand in the order `.storybook/preview.tsx` sorts them. A story's
`title` is the only thing that files it under one. The capture gate keys on `importPath`, never on
the title, so a title may describe the surface instead of the file:

| Root | What is under it |
|---|---|
| `Get started/` | The introduction: what the catalog is, and how it is put in order. |
| `Foundations/` | The rules under every component, one node per topic: `Color`, `Typography`, `Spacing`, `Radius`, `Elevation`, `Motion`, `Iconography`, `Brand`. |
| `Components/` | One node per component in this directory, under the category below that says what it is for. |
| `Patterns/` | Parts of a screen that are not a page: the query gate, the add, edit, merge and share actions, the composer. |
| `Shell/` | The app frame and the Home page. |
| `Records/` | The pages a rep works in, and the cards on them. Each page a record opens is one node: `Contact 360/`, `Company 360/`, `Deal 360/`, `Project 360/`, `Leads/`, `Offers/` and `Deal room/`. `Record 360/` holds the parts that two or more of those pages mount. A list page, the table of every record of one kind (`Companies`, `Contacts`, `Deals/`, `Projects`, `Partners`), is a leaf or node of its own. The one case apart is the leads list, which sits in `Leads/` beside the lead page. `Worklist/` and `Reports/` are nodes of their own. |
| `Settings/` | `<Group>/<Page>/<Card>`, the same shape as the settings catalog: the groups of `SETTINGS_GROUPS` and the pages of `SETTINGS_PAGES`, under their own sidebar labels. `screens/settingsstories.test.ts` fails a story filed under a group or page the catalog does not declare. |
| `Onboarding/`, `Signed out/` | The first run, and the pages you can reach without a session. |
| `MCP Apps/` | The governed tool surfaces and their document forms. |

`Components/` has one level of category, named the way the Atlassian design system names them. AI
and provenance is the one category of our own:

| Category | What is under it |
|---|---|
| Forms and input | Controls a reader fills, picks or presses: `Button`, `Text input`, `Textarea`, `Search field`, `Field`, `Checkbox and radio`, `Segmented control`, `Select`, `Combobox`, `Value inputs`, `Switch`, `Choice list`, `Field grid`, the inline edits, the pickers, the filter controls, `Icon action` |
| Images and icons | Marks that stand for a contact or a company, and the ground a place is drawn on: `Avatar`, `Avatar stack`, `Company logo`, `Ambient waves` |
| Labels | A word in a pill that takes no click: `Badge`, `Tag pill`, `Row tags`, `Visibility`, `Role badge` |
| Layout and structure | The boxes and columns a page is built from: `Card`, `Panel`, `Section header`, `Disclosure`, `Page zones`, `Record view`, `Setting row`, `Action row` |
| Loading | What a surface shows while it waits: `Skeleton`, `Busy mark`, `Pending body` |
| Messaging | What a surface says when something needs saying: `Callout`, `Error line`, `Toast`, `Empty state`, `Surface state`, `Card boundary` |
| Navigation | What takes a reader to another place: `Breadcrumb`, `Record tabs`, `Contact link`, `Offsite link` |
| Overlays and layering | Surfaces drawn over the page: `Modal`, `Drawer bands`, `Record form dialog`, `Confirm modal`, `Popover`, `List popover`, `Tooltip`, `Overflow menu`, `File preview`, `Resolve sheet` |
| Status indicators | Where something stands: `Communication status`, `Pipeline ladder`, `Stage strip` |
| Text and data display | Records, figures and text to read: `List table`, `Data table`, `Table scroll`, `Pipeline board`, `Timeline list`, `Readings`, `Stat card`, `Heading`, `Kbd`, `Markdown`, the email family |
| Primitives | `Stack` and `Row` |
| AI and provenance | What an agent proposed, and where a value came from: `Decision card`, `Decision deck`, `AI pending`, `Evidence mark`, `Evidence receipt`, `Source evidence`, `Source email panel`, `Provenance tag`, `Trust`, `AI runtime chip`, `Margince core` |

Under every root, every part of a title is in Sentence case. A declared short form or proper name
(`AI`, `Margince`, `MCP Apps`) keeps its spelling. `sidebar.test.ts` holds one title per story file,
and no title that is both a leaf and a group.

## Driving a control in a test

`Select` is a button and a portalled listbox, so `userEvent.selectOptions` does not work on it. Use
the helper:

```ts
import { pickOption } from "../design-system/select-testing";

await pickOption(user, screen.getByRole("combobox", { name: "Stage" }), "Won");
```

## The gates that enforce this

The vitest suites run in `make fe-unit`, and the shell scripts in `make fe-ds-gates`; both are part
of `make frontend-check`. The axe sweep runs in `make frontend-e2e`. `native-controls.test.ts` is a
vitest gate that reads the TypeScript syntax tree. A parser can tell a real element from one inside
a comment or a string.

| Gate | What it refuses |
|---|---|
| `make native-controls` (`src/design-system/native-controls.test.ts`) | `<select>` / `<option>` / `<optgroup>` anywhere under `src/` and in every extension frontend layer, with no exception (`design-system/select.tsx` holds no native control) |
| `frontend/scripts/check-ds-purity.sh` | A hex literal or `rgb()`/`hsl()`/`oklch()` outside `tokens.css`. It also leaves out tests, the generated `schema.d.ts`, `provider-mark.tsx` (the marks of other companies) and `tokens-testing.ts` (which reads the sheet) |
| `frontend/scripts/check-font-lock.sh` | A fourth type family, and mono outside code: the `t-mono` class anywhere; a `font-family`/`font` that names a mono family on a rule whose selectors do not all have `code`, `pre`, `samp` or `.code-block` as their subject; a custom property in `tokens.css`, other than `--fontFamilyMono`, that carries one; a mono family in a TS string. `check-font-lock.test.sh` plants each shape, and each allowed one |
| `design-system/mono.test.ts` | The same mono rule, read with a real parser: it skips comments, and resolves selectors through nesting and `@media`. It reads `style` and `<style>` in HTML, and class names and inline `fontFamily` from the syntax tree of every TS/TSX file under `src/`, `e2e/`, `.storybook/` and every extension frontend layer |
| `design-system/weights.test.ts` | A weight no font file exists for. It reads the Google Fonts request in `index.html` against every `font-weight`, `font` shorthand and `--font*` token under `src/`, plus each inline `fontWeight`. It refuses a text family that does not load 400/500/700, a rule that asks for another weight (named by file and line), and a mono family that loads a weight the text families do not have. A missing request, a family with no weights, or an empty corpus fails |
| `frontend/scripts/check-icon-glyph.sh` | An emoji glyph in a source string: Lucide only |
| `frontend/scripts/check-ds-spacing.sh` | New raw px margin, padding or gap outside this tier (only in the changed lines) |
| `frontend/scripts/check-ds-spacing-roles.sh` | Screen CSS that spaces again a class this tier spaces and declares, sizes again one it sizes (`font-size`, `line-height`, `letter-spacing`), or spells a step where a role exists (`*-actions` gap, `*-cards` gap, `*-card`/`*-panel` padding). It reads the whole tree; the corpus comes from this tier on every run |
| `design-system/conformance.test.ts` | UI text typed into the code, and the same colour and font rules. A reduced-motion rule that a later plain rule of equal specificity beats. A motion that runs for ever with no reduced-motion answer. A second home for the pending read-out or the placeholder pulse. A focus ring that spells its own width and colour |
| `design-system/stylesheetnamespace.test.ts` | One stylesheet per class prefix: a screen's prefix (`auth-`, `book-`, `offers-` …) declared in any sheet but its home, across every `.css` under `src/` and each extension's frontend layer. Comments are removed first |
| `design-system/timelinerows.test.ts` | A rule that reaches the `li` of a `.timeline` list by anything but `>` right after `.timeline`, since that also reaches the `li` items deeper inside a row. A sibling (`+`, `~`) of such a row passes. It reads every `.css` under `src/` and each extension's frontend layer, and fails when its parsed selectors and a plain-text count disagree |
| `design-system/actionrow.test.ts` | A container of two or more sibling buttons without `gap: var(--gapActions)`, from a class it names or its own inline style. It reads markup and stylesheets together, so it finds a row that names a class nothing defines |
| `design-system/tokens.test.ts` | A token whose value drifted from the design canon |
| `design-system/type-tokens.test.ts` | The type half of the same sheet. The three weight tokens and the ten size shorthands must be declared with no condition (not inside a `@media`). Each weight and control token must have its expected value, with no second control height. No `--fontBody*`/`--fontHeading*` may spell a weight by value instead of reading `--fontWeight*`. It checks the file that `type-source.test.ts` lets through; both read `tokens.css` through `tokens-testing.ts` |
| `design-system/corners.test.ts` | A `border-radius` that names a length instead of a step (it can be waived in the line, with a reason); a `--r-full` or `50%` corner without `corner-shape: round`; a second `corner-shape: squircle` outside `tokens.css`; a step that is not a multiple of four, or has no doubled twin in the `@supports` block |
| `design-system/cardzones.test.ts` | A `Card` under `src/screens/` that carries a `title` or draws a `SectionHeader` as its own head band; that surface is a `Panel`. Read from the syntax tree. A card inside another surface is named in the test with its reason, and an entry that is out of date fails |
| `design-system/inlinelayout.test.ts` | Layout written as a literal inline `style`: a margin, padding, gap, display, flex, grid, alignment, inset, size, position, overflow, order or transform property with a literal value. It checks every module under `src/` and every unit's frontend layer (all forms of the language). A computed value (a popover position, a bar's percent) passes. It sees through ternaries, guards, defaults, `Object.assign`, casts, conditional spreads and hoisted `CSSProperties`. It checks the `style` attribute, a spread props object, and the `createElement` / `jsx` call. Every file is held at zero; the fix is a class in the screen's sheet, or `Stack` / `Row` in a unit. A fixture suite plants each shape |
| `design-system/panelhead.test.tsx` | A description back in a panel or card head. That is a `sub` / `description` / `intro` prop declared by `Panel`, `PanelGroupHead`, `Card` or `SectionHeader`, or a `.tsx` under `src/` that passes one. It is also a `className` that names `card-sub`, or a stylesheet that declares `*-head-sub` / `*-title-sub` / `card-sub` or a `.sub` inside `.panel-head` / `.section-header`. It also refuses a head title at any size but `medium` (except the page-naming `level={1}`), read off `data-size`. Both walks fail closed under a floor |
| `design-system/tick-spelling.test.ts` | A tick drawn outside `Checkbox`/`Radio`: `role="checkbox"`, `"menuitemcheckbox"`, `"radio"` or `"menuitemradio"` on a JSX element that is not an `<input>`; `aria-checked` on anything but an `<input>` and `switch.tsx`; `<input type="checkbox">` / `type="radio"` outside `atoms.tsx`. Read from the syntax tree, each finding named `file:line`. Stories and tests are left out (`interaction.stories.tsx` draws native controls to show `accent-color`). It fails closed under a floor of file counts, with a probe suite |
| `design-system/badge-spelling.test.ts` | A pill drawn outside `Badge`. That is a class named `*-badge`, `*-pill`, `*-lozenge` or `*-tag` whose rules lay a fill on a full corner (`--r-full` or `50%`). It is also a rule outside `atoms.css` that names `.badge` and sets anything but placement (margin, alignment, order, flex/grid placement, position, `max-width`, overflow). It is also markup that carries the `badge` class outside the atom, or a `Badge` given `style` or `className`. A new look is a variant proposed in this directory |
| `design-system/heading-spelling.test.ts` | A raw `<h1>`–`<h6>` element, or a `createElement("h2", …)` / `el("h2", …)` that builds one, anywhere under `src/`, tests and stories included. `heading.tsx` and the view builder in `mcp-apps/bridge.ts` may name the element, and the gate holds their two size → element tables equal |
| `design-system/type-source.test.ts` | Type declared by value outside `tokens.css`, in every `.css` under `src/` and every `style` object in a `.ts`/`.tsx` file that is not a test. `font-size`, `line-height`, `letter-spacing` and `text-transform` may only say `inherit`; `font` takes a `--font*` token, and `font-weight` a `--fontWeight*` one. Inside the two UA resets (`app.css`, `mcp-apps/view.css`) it allows `text-transform: none` and `line-height: 0` on a rule that selects only `sub`/`sup`. It also refuses capitals drawn through `font-variant-caps`, the `font-variant` shorthand, or `"smcp"` / `"c2sc"` / `"pcap"` in `font-feature-settings` (`font-variant-numeric: tabular-nums` is still allowed). It refuses a `.toUpperCase()` / `.toLocaleUpperCase()` drawn in TSX too. It fails closed: a missing `app.css`, under 100 stylesheets, or under 100 components |
| `design-system/controlheight.test.ts` | A thing you can press at a height other than `var(--controlHeight)`. That is a rule that sets `height`, `min-height`, `block-size` or `min-block-size` on a class marked `cursor: pointer`, on a class the markup puts on a `<button>`, or on `button` itself. Both readings have a floor. Every other height is in a register with its reason (a field is `--inputHeight`, a sign-in door owes 44px), and the register may only get shorter |
| `design-system/menu-anatomy.test.ts` | A menu that spells its own shape. Every option surface reads one inset (`--controlGap`), one floor (`--menuMinInlineSize`) and one ceiling (`--menuMaxBlockSize`), or carries its reason in the roster. The option surfaces are the ListTable menus, the popup of `Select`, the suggestion list, the overflow panel, the account menu and its flyout, and the settings search list. A second arm fails any element with a `menu`, `listbox`, `menuitem` or `option` role whose class the roster does not name |
| `design-system/catalog.test.ts` | A component in this directory that this table never names. A bare ✅ that no story backs: neither the module's own `<module>.stories.tsx`, nor a `<component>.stories.tsx` named for a component in the row's first cell that imports the row's module. A story or docs file whose title cannot be read. A story under `src/` filed under a root the sidebar table does not list. Catalog groups that are not the categories, in their order |
| `design-system/sidebar.test.ts` | A listed root with no story. A `Components/` story outside a declared category, a `Foundations/` story outside a declared topic, or a category or topic with no story. Under the shaped roots: a part of a title that is not in Sentence case, two files that share a title, or a title that is both a leaf and a group. A `storySort` in `.storybook/preview.tsx` whose roots, categories or topics are not these tables, in their order |
| `e2e/` (axe) | WCAG 2.2 AA on every core screen, plus the 390px sweep that checks no page scrolls to the side |
