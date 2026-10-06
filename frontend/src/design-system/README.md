# `src/design-system/`: the Margince design system

Read this before you hand-roll a control.

This file is the catalog: what exists and what each thing is for. How a screen
should look (grounds, chrome, type, depth, the anatomy of a record page) is
[`DESIGN.md`](../../../DESIGN.md) at the repository root; read that first when
the question is taste rather than parts.

## The rule

**Every interactive control comes from this directory.** A native `<select>`, a
hand-rolled dropdown, a bespoke menu, a second modal or one more "just this
once" chip is a defect. Two reasons:

1. A browser-drawn control is drawn by the browser. `<select>` takes our tokens
   on its closed face and none of them on the list behind it (`option` is not
   stylable in any engine we ship to), so it reads as a hole in a product built
   from these files.
2. A copy drifts without anyone noticing. Each copy grows its own gap, its own
   focus ring and its own idea of what a label is, and each one looks fine in
   review on its own.

If what you need is not here, add it here, with a story and a spec, and it
becomes the one spelling. Copy never lives in a primitive: words arrive through
props, translated by the caller with `t()`.

## Type comes from the tokens

One rule sets the type for the whole product. `body` in `app.css` reads
`--fontBody` and `html` beside it reads `--textPrimary`, and a reset hands both
to the elements a browser would otherwise scale itself. The root declares no
font, so `1rem` stays the browser's own 16px. A `font` on the root would resolve
its own `rem` against the size it was setting and shrink the whole document.
Everything inherits, so there is no size, leading, weight or neutral ink to pick
at a call site. `src/mcp-apps/view.css` carries the same shape for the
standalone views.

Every type token in `tokens.css` is one `font` shorthand in rem: weight, size,
leading and face in a single value, so a level cannot be half-worn. The weight
in it is itself a token (`var(--fontWeightBold)`), which a `font` shorthand
takes because `var()` is substituted before the shorthand's grammar is checked.

Size, leading, weight and tracking exist only in `tokens.css`. Every other
sheet and every style object in the tree wears a `--font*` token or inherits.
`design-system/type-source.test.ts` fails a second spelling of any of the four,
wherever it is written.

Body has three levels, each with its own paragraph spacing. The gap between
two blocks of prose is that spacing and nothing else: one sibling rule in
`app.css`, at zero specificity, puts `--paragraphSpacing` between them. A level
that is not the default therefore hands its own spacing down beside its `font`,
or the prose keeps the default's rhythm.

| Level | Token | Spacing | For |
|---|---|---|---|
| L | `--fontBodyLarge` | `--paragraphSpacingLarge` | Comfortable reading: marketing and blog prose. Rare in the product. |
| M | `--fontBody` | `--paragraphSpacing` | The platform default, and what `body` already wears. |
| S | `--fontBodySmall` | `--paragraphSpacingSmall` | Sparingly: secondary-level content and semantic messaging. |

**A heading's size comes from its context.** Seven tokens, all in the heading
face at `--fontWeightBold`:

| Token | For |
|---|---|
| `--fontHeadingXXLarge` | Brand and marketing content. |
| `--fontHeadingXLarge` | Brand and marketing, and the largest page title in the product. |
| `--fontHeadingLarge` | A page title in the product: a view's or a form's. |
| `--fontHeadingMedium` | A large component with room, balanced against Body M: a modal's title. |
| `--fontHeadingSmall` | A title in a small component where space is tight. |
| `--fontHeadingXSmall` | The same, tighter still: a flag's own title. |
| `--fontHeadingXXSmall` | Sparingly; pairs with Body S, as fine print does. |

**A heading's level comes from the structure.** `<h1>`–`<h6>` are how a
screen-reader user navigates a page and how everyone else sees it group. They
run in descending order, there is one `<h1>` (the page title), and a level is
never skipped (no `<h2>` followed by an `<h4>`). Size and level are independent:
an `<h3>` inside a roomy modal wears `--fontHeadingMedium` while an `<h2>` on a
dense card wears `--fontHeadingXSmall`.

A heading is written as `Heading`, which makes both decisions: `size` for the
token, `as` for the level. The raw `<h1>`–`<h6>` elements are gated out of the
tree so the pair stays together. There is no global CSS rule for `h1`–`h6`;
`Heading` maps each size to a default element, and `as` overrides it.

A new section gets a heading: the element and a heading token. Bold body text or
a size bumped by hand is not a heading. Assistive tech announces the element, so
a section introduced that way has no title for the reader who most needs one.

**Weight says what kind of text it is**, and it is named rather than spelled:
`--fontWeightRegular`, `--fontWeightMedium` and `--fontWeightBold` are the three
roles, and a rule reads the role.

- 400 regular for generic paragraphs, so they read as prose against a heading
  and against the text inside components.
- 500 medium for text beside a line icon, where the stroke of the glyph and the
  stroke of the letter should match. That covers most text inside a component.
- 700 bold for emphasis, for telling one thing from another, and for the heading
  tokens. Used anywhere else it stops meaning anything.

Both text families ship 400, 500 and 700, and `index.html` loads those three. A
fourth weight renders as a face the browser synthesized rather than one the
designer drew. `design-system/weights.test.ts` reads the font request and the
stylesheets against each other, resolving each weight token through
`tokens.css`, and fails on the difference.

The `.t-*` names in `base.css` are role hooks, not sizes. An element that plays
a role wears one; each role's rule is layered on top of the root with its own
gate.

- `t-caption`: the supporting line, Body S in `--textSecondary`. Size and ink
  come together, because either one alone reads as a mistake.
- `t-label`: a control's name, Body S at `--fontWeightMedium`, a rung under the
  value it introduces.
- `t-name`: the name of a group (a settings row, a fieldset's legend), medium
  weight at the size of what it heads. A field's label is quieter than its
  input; a row's name is not quieter than its row.
- `.t-danger` is declared after the roles, because it weighs what a role weighs
  and has to be the later rule to win when worn over one.

**Mono is for code only.** Geist Mono is worn by an element that is code
(`code`, `pre`, `samp`, `.code-block`) through one rule in `base.css`. An id, a
key, a URL and an amount are read, not run; a figure that must line up wears
`.t-num` (`font-variant-numeric: tabular-nums`). `--fontFamilyMono` is the one
token that spells it. `design-system/mono.test.ts` with `check-font-lock.sh`
fails the `t-mono` class, a mono family on any other selector, a second token
carrying one, or one in an inline style.

**Two neutral inks.** `--textPrimary` carries a heading, a record's name and
body prose. `--textSecondary` supports: a placeholder first, with each further
role decided at the element. Dark lifts both: `--textPrimary` goes to `#fff`
and `--textSecondary` to `lch(63.304% 1.425 272)` off the light
`lch(40% 1 282)`. Anything quieter than the second uses the same ink at a
smaller size or lighter weight; there is no third neutral.

`Foundations/Typography` in Storybook draws the body levels, the heading ladder
and the pairings.

## Corners come from the ladder, and they are smooth

Every radius in the tree reads one of six tokens: `--r-xs` 4, `--r-sm` 8,
`--r-control` 12, `--r-md` 16, `--r-lg` 20, `--r-full` the pill. A raw px corner
is a seventh value nobody chose.

`tokens.css` also declares `corner-shape: squircle` on `:where(*)` inside an
`@supports` query, and doubles the ladder inside it. A superellipse of radius R
reads about as round as a circular corner of R/2, so a corner keeps its apparent
size and gives up its two points. Nothing at a call site opts in.

**A round thing opts out**, beside its own radius:

```css
.badge {
  border-radius: var(--r-full);
  corner-shape: round;
}
```

`:where(*)` carries zero specificity, so that one line always wins. Write it
wherever you write `--r-full` or `50%`: a superellipse at full radius is a
lozenge, and on an avatar it is a squircle tile.

A generated box takes the shape too. `*` matches elements and not
pseudo-elements, so `::before`, `::after` and `::backdrop` are named beside it
in that rule. The doubled tokens reach a pseudo-element anyway, because a custom
property inherits and `corner-shape` does not, so an unlisted one would draw a
round corner at twice the radius its author asked for. `corners.test.ts` holds
the list against the pseudo-elements the tree gives a corner to.

## State colours

Five states, and a screen may not invent a sixth.

| State | What it says | Base (light / dark) |
|---|---|---|
| information | The neutral report, and work still in flight: an info mark, a spinner's ring, a row a job has not finished writing. | `#0485f7` / `#0485f7` |
| success | A favourable outcome: the thing finished the way the reader hoped. | `#17c964` / `#17c964` |
| warning | Caution before the fact: the sentence that stops a mistake while it can still be stopped. | `#f5a524` / `#f7b750` |
| danger | The serious or irreversible one: the write that failed, the record that cannot be brought back. | `#ff383c` / `#db3b3e` |
| discovery | What is new to this reader: onboarding, a capability they have not met. The one state that is not a verdict about the record. | `#964ac0` / `#b76be3` |

One base per state, and it is the only value anybody picks. Every other
member is derived from it, in `tokens.css` and nowhere else, so a retune moves
the whole family at once:

| Token | How it is derived | What it is for |
|---|---|---|
| `--<state>` | the base | A fill, a bar, a dot, a border: everything seen with nothing read on it. |
| `--<state>Text` | the base walked down in OKLCh lightness (light) or up (dark), chroma kept, until it clears 4.5:1 | Ink: badge lettering, an error line, a caption, a stat card's figure. Also the ground of a filled control. |
| `--<state>Surface` | `color-mix(in srgb, var(--<state>) N%, var(--bgElevated))` | The opaque tint, for a badge. A translucent one takes its contrast from whatever it lands on, and a badge lands on everything. |
| `--<state>Bg` | the same `N%` over `transparent` | A wash on a ground the token cannot know: a page-level tint, a row, a gradient stop. |
| `--<state>Border` | the base at `45%` over `transparent` | The hairline. |

A filled control's ground is the tone's `--<state>Text`, never its base. A
base is tuned to be seen at a bar's or a dot's size, and no single ink sits on
all five: white pays 2.04:1 on `--warning`, a near-black 3.19:1 on
`--discovery`, and the dark danger base takes neither. The Text token carries
ink, so it fills a destructive button, a completion disc and a `primary` badge.
They are lettered in `--textOnStatusControl`, the one token in the palette that
flips with the theme, because the ground under it does.

Discovery's light ink, surface and border are stated as hexes (`#48245d`,
`#eed7fc`, `#d8a0f7`). The design source gives that ramp directly, and no share
of `#964ac0` over `--bgElevated` reaches `#eed7fc`, which is bluer than the
ground it would be mixed into. The same hue and contrast gates measure them.

**Each tone has one spelling.** Use `warning`, not `warn`, in the prop, the CSS
class suffix, the `data-` attribute and the test: the state is a noun. Each
component that carries tones exports its vocabulary as one array that the type,
the story and the gate all read: `BADGE_TONES` (`atoms.tsx`), `CALLOUT_TONES`
(`callout.tsx`), `PANEL_TONES` (`panel.tsx`), `TOAST_TONES` (`toast.tsx`),
`SECTION_STATE_TONES` (`surfacestate.tsx`), `STAT_CARD_TONES` (`statcard.tsx`).

Two colours are not states. `--accent` is brand and primary action: a
checkmark, a "Connected", a completion disc or a "Strong" reading drawn in it
says *press me* about something that has already happened, so those read
`--success*`. `--ai` is provenance: a progress bar or a running step drawn in it
claims a model decided the figure, so work in flight reads `--info*` (a
spinner's ring, a queued delivery, a stage under way).

**The gate is `tokens.test.ts`.** It reads the five states off `tokens.css`. In
both themes it measures every `--<state>Text` on every ground it can land on,
on its own `Surface`, and on its own `Bg` over every ground. It holds every
stated member to its base's OKLCh hue, holds `Surface` and `Bg` to one share,
and fails when a state is missing from either dark arm. `state-ink.test.ts`
holds the consumer side in every sheet the app ships: no text is lettered in a
base, SVG `<text>` included. Only SVG geometry may wear one.

A colour that does not mean one of these five belongs to another family.
`--accent` is brand and primary action, and `--ai` is agent provenance.
`--orbAmber` / `--orbRed` are an agent run's outcome at a size where chrome
colours go muddy. The `--tag*` row means only "not that other tag".

## Indigo says a machine did it

`--ai` is the AI hue, and it carries meaning. An indigo tint on a surface
is a claim about provenance: the information, or the proposed action, came
from an agent rather than from a human. Paint it where that is true and
nowhere else. A card tinted indigo for looks tells every reader of that screen
something false about who decided.

It is a family of its own for the same reason the five states are: one meaning
per colour. `--accent` (deep emerald) is brand and primary action, the state
hues report how something went, and none of them means "authored by a model".
The split is stated at the top of `tokens.css` and pinned by `tokens.test.ts`.

This family splits base from ink the same way a state does; see
[State colours](#state-colours) for the rule and the gate.

| Token | Value | Role |
|---|---|---|
| `--ai` | `#5b61d6` | The line and the mark: a solid border on a staged card the reader has focused or accepted, a focus ring, a hairline. The strongest of the four, so the rarest. |
| `--aiLight` | `rgba(91, 97, 214, 0.08)` | The ground under agent-authored text: staged cards, the onboarding stage's wash, a `badge-ai` fill. |
| `--aiMed` | `rgba(91, 97, 214, 0.3)` | The border of something staged. Usually `1.5px dashed`, which is half the signal (see below). |
| `--aiText` | `#3f45b0` light, `#9ba0f0` dark | Text and glyphs on `--aiLight`. Never `--ai` for text over the tint: the two are close enough that it fails AA on the ground its own family paints. |

**Dashed means proposed; solid means real.** `1.5px dashed var(--aiMed)` marks
a thing an agent has staged and a human has not yet accepted: `.staging-card`,
`.deal-card.staged`, the deck's empty and staged slots. The dashes going solid
is what acceptance looks like. The tint says *who*, and the stroke says
*whether it counts yet*.

One family, one declaration site. `Panel tone="ai"` and `.staging-card` are
the same claim on two shapes, so both are declared in `panel.css`. `.panel-ai`
is for the panel a machine wrote; `.staging-card` (and `.deal-card.staged`) is
the staged variant, which differs only by the dash. Reach for the class rather than
spelling the tint and the edge again; a box that drew its own indigo would tell
a reader the two were different claims.

Tone says who wrote it; the badge says what is offered. `Panel tone="ai"`
goes on a panel whose body a model produced. A `Badge tone="ai"` in the header
band (the panel's `titleAction`, carrying `co.assistant.aiTag`) goes on a panel
that offers an AI verb: ask, draft, write again, assess again. They are two
facts, so they compose, and a panel that is both wears both.

**`ProvenanceTag` is the rule at its smallest**: one `Badge` per provenance,
and only its `agent` arm takes `tone="ai"`, with the Sparkles that tone draws.
Every other kind (a connector, a system job, a human, a buyer, an unrecorded
source) is the default soft badge, told apart by its words. A connector copies
what a mailbox already held and a system job runs a rule, so neither is a model
deciding. There is no provenance class to reach for; the tone is the claim.

### The orb is the same claim, made loudest

The agent Core and the lit window edge draw in the Core palette, and its working
tones are the AI indigo: `--orbBody` is declared as `var(--ai)` so the two cannot
drift, with `--orbGlow`, `--orbMid` and `--orbBright` around it. It is a family
because a lit glass body needs tones a flat UI accent cannot supply. It needs a light end to glow, a bright end for the one state with
energy, a dark to be seen against.

The tones are named by role (`--orbBody`, `--orbGlow`, `--orbMid`,
`--orbBright`), not by hue, so a repaint never makes a name wrong.

The three states that are not work keep their own hues. `--orbAmber` asks a
user for something, `--orbRed` failed, `--orbGrey` cannot reach a source, and
none of them goes indigo. Provenance and outcome are different questions: an orb
that turned indigo when a run failed would say who was working and stop saying
how it went. `--orbRed` is not `--danger`, because `--danger` is UI chrome and
goes muddy at 34px.

**Colour is never the only signal.** As with `Callout`, the words carry the
meaning. An agent-authored surface says so in text (an `ai` `ProvenanceTag`, a
"suggested" label, an "Approve" verb), and the tint makes it findable at a
glance for readers who can see it.

## What this directory already gives you

### Foundations

| Primitive | For | File | Story |
|---|---|---|---|
| `Logomark` | The product's own "M". One mark, every fill on `currentColor`, so the shell chip and the onboarding speaker draw the same mark | `logomark.tsx` | ✅ |
| `.arrive` / `.arrive-stack` | The arrival animation: content fades over `--dur-enter` and rises over `--dur-move`, staggered `--stagger-enter` apart, four deep and then together. A container whose children arrive says `.arrive-stack`, and its children need no class. `.wrap`, the shell's page gutter, is named in the rule, so every screen gets it. A stack container does not itself arrive (two nested fades make a dimmer fade), so mark each level where a screen nests its blocks deeper. It plays when an element is inserted, which gives a tab panel its transition while the tab strip stays still | `enter.css` | via every screen |
| `--focus-ring` / `--focus-glow` / `--focus-ring-forced` | The focus ring, in three shapes. An outline for a control on a surface, drawn outside the box so it never changes the control's size. A glow for a field, which already has a boundary. A transparent outline paired with the glow for a control that is neither, so forced-colors mode still has something to paint. The offset stays per component: a control clipped by its container draws the ring inside itself. A fitness function refuses another spelling | `tokens.css` | — |
| `--gapCards` / `--padCard` / `--padPanel` / `--gapActions` | The spacing roles: what an interval is for, where `--space-N` says only how big it is. `--gapCards` between sibling surfaces, `--padCard` inside a bare card, `--padPanel` inside a panel (a rung above the card, because the header band, rows and footer share that inset), `--gapActions` between two buttons side by side. Retune a role here and every screen moves with it. `check-ds-spacing-roles.sh` holds screen CSS to them, and holds that a screen re-spacing a primitive says so in this vocabulary | `tokens.css` | — |

Tokens and sheets, none of which a caller redeclares: `tokens.css` (the Ledger
Green canon, pinned by `tokens.test.ts`), `brand.css` (the derived layer:
`color-mix()` over a canonical token, never a new hex), `base.css`, and the
per-component sheets a component imports itself. Literal colours appear only in
`tokens.css` and in `provider-mark.tsx`, which draws other companies' sign-in
marks; `check-ds-purity.sh` enforces this. `interaction.stories.tsx` catalogues
the colours the browser owns (caret, checkbox tick, scrollbar thumb,
selection), which are set once at the document root and belong to no component.

### Forms and input

| Primitive | For | File | Story |
|---|---|---|---|
| `Button` | The one button. Variants: `primary`, `ghost`, `danger`, `federated`, `ai`, `aiQuiet`, `link`; `iconOnly` makes a square glyph button that still needs an accessible name. Height is always `--controlHeight` and there is no size prop; a labelled button has a width floor. `reason` / `reasonId` refuse the press and print why in `t-caption`, and a caller's `disabled` or `aria-describedby` cannot defeat them. `pending` marks a write in flight: `aria-disabled`, full ink and a turning mark, with `busyLabel` for a sentence about the wait. `federated` is the full-width sign-in door led by a `ProviderMark`; `unavailable` is its resting refusal. `ai` (filled, one per surface) and `aiQuiet` (tinted, for an AI verb among equals) mean a model does the work behind the click. `link` wears `.link-button` for a verb that needs Button's refusal and `pending` contracts but should read as a link; `button.test.tsx` holds its class pairing | `atoms.tsx` | ✅ |
| `.iconbtn` | A bare icon affordance that is not a `Button`: no fill, no border, no label. For the verb a row offers where a full button would out-shout the row. It carries the `--controlHeight` hit target, the hover and the focus ring | `base.css` | via `Button` |
| `.link-button` | A secondary text affordance (accent-coloured, no fill, no border) for an action that must not compete with the primary button beside it: a download, a "view existing", an in-row verb. A class a caller applies to its own `<a>` or `<button>`; use `Button variant="link"` when the verb also needs Button's refusal and `pending` contracts. Not `.co-rowlink`, which is for a row title that underlines only on hover. A lucide glyph may lead the label; the class sizes it | `atoms.css` | ✅ |
| `TextInput` | The one text field: `--inputHeight` on `--inputPaddingY`/`--inputPaddingX`, in root type and ink, with the placeholder in `--textSecondary`. `Textarea`, the `Field` shell, the `TokenInput` frame and the `Select` trigger take the same box; buttons stay on `--controlHeight` | `atoms.tsx` | ✅ |
| `SearchField` | A text field with the search affordance. `flush` is for a field whose container draws the chrome (the ⌘K palette's bar). It takes a `ref` for a caller that has to move focus to it | `atoms.tsx` | ✅ |
| `Textarea` | The one multi-line field | `atoms.tsx` | ✅ |
| `JsonField` | A JSON value an advanced reader edits by hand: a `Textarea`-shaped `.code-block` with a line-number gutter that marks the lines a problem points at. No highlighting or completion; the server validates, and `lineOfPath` / `parseProblem` turn its key path, or the parser's complaint, into the marked line. Tab indents | `jsonfield.tsx` | ✅ |
| `RichText` | The light formatting a business email needs: bold, italic, links, lists. The marks sit in a footer under the writing surface beside the caller's `hint`; `actions` adds the caller's own verbs there (the composer's paperclip). `disabled` refuses typing. Built on `contentEditable`, because the server's outbound allowlist (`activities.SanitizeOutboundHTML`) decides what a recipient receives. Every change reports both `html` and `text`, and both go on the wire. No image button: a remote image is a read receipt | `richtext.tsx` | ✅ |
| `Select` | The one dropdown: a button trigger plus a portalled listbox. Never a `<select>`. The trigger is field-sized (`--inputHeight`). An option whose label is not in the page's language carries `lang` (WCAG 2.2 AA 3.1.2). An `adornment` draws a mark before an option's label and on the closed face; it is `aria-hidden` and the `label` stays a plain string. `appearance="button"` draws the same trigger through `Button` (`primary`, `--controlHeight`, `pending` while a write is out) for the one value a reader sets beside a record's name, the company header's lifecycle stage; a value inside a line of text stays `InlineChoice` | `select.tsx` | ✅ |
| `MultiSelect` | The multi-value sibling of `Select`: the same trigger and listbox, where a pick toggles membership and the list stays open. For an enumerable vocabulary such as a custom field's options; `TokenInput` is the control for a set nobody can enumerate. `values`/`onChange` speak `string[]` | `select.tsx` | ✅ |
| `ComboBox` | A text box that takes any value and offers suggestions from a portalled listbox. Use it over `Select` when the known answers are a starting set and the real vocabulary belongs to somebody else (a model id). Typing is never overridden (`aria-autocomplete="list"`), Escape keeps the text, and with nothing to suggest it is an ordinary input. Shares `suggestlist.tsx` with `TokenInput` and `anchoredpopup.ts` with `Select` | `combobox.tsx` | ✅ |
| `useSuggestList` / `SuggestPopup` | The portalled listbox a text box offers, and its keyboard grammar: the half `ComboBox` and `TokenInput` share. It decides which rows match (a substring over label and value), which row is active, what Enter and Escape do, and where the popup sits. What a pick means stays with each host. `anchoredpopup.ts` is the layer below, shared with `Select` | `suggestlist.tsx` | ✅ |
| `TimezoneSelect` | City-first timezone dropdown built on `Select`, with current UTC offsets and keyboard typeahead. Preserves saved aliases and submits IANA identifiers | `timezoneselect.tsx` | ✅ |
| `ProjectPicker` / `ScopeLine` | Which project a surface is about, and the line saying which project its output was narrowed to. `ProjectPicker` is a `Select` over the shared `projects` section (`liveProjects` drops closed ones; `useSoleProjectDefault` picks the only live one). `ScopeLine` prints the server's report ("Scoped to KEY · N of M activities"). Every AI surface renders this pair | `projectpicker.tsx` | ✅ |
| `DateInput` | The one date field: a native `type="date"`, the exception to the `Select` rule because its closed face takes our tokens and the platform picker appears only on request. `value` is always `YYYY-MM-DD`, the contract's `format: date` shape | `dateinput.tsx` | ✅ (`Value inputs`) |
| `TimelineFilterBar` | The row of dials above a record timeline: a kind `Select`, a `SearchField`, and a from/to pair of `DateInput`s. Every dial is a server parameter, spelled once in `recordtimeline.ts` (`timelineQueryParams`). State is `TimelineFilters`, held by `useTimelineFilters` and handed to `useRecordTimeline`. Search commits on Enter or blur, and while it is active the bar says limited conversations are left out. `kinds` narrows which kinds the dial offers | `timelinefilterbar.tsx` | ✅ |
| `TokenInput` | A set of short values built one at a time, for the filter engine's `in` operator where the candidate set is not enumerable. Enter or comma commits, Backspace on an empty box removes the last, a duplicate is dropped, and blur commits. Optional `suggestions` help without constraining; a row is found by label or value. With no vocabulary it keeps the plain `textbox` role | `tokeninput.tsx` | ✅ (`Value inputs`) |
| `TokenList` | The `TokenInput` token without the text box, for a set assembled elsewhere. Takes `{id, label}`; `removeLabel` is a function so each remove button names its token; omitting `onRemove` renders a read-only set | `tokeninput.tsx` | ✅ (`Value inputs`) |
| `OptionCount` | The count beside an option's name in `SegmentedControl` and `RecordTabs`: tabular digits, the reader's number format, its own chip, and a visually hidden comma so a screen reader says "Contacts, 2". It renders inside the option's button and inherits the host's ink. Use it wherever a number says how much sits behind an option, tab, filter, menu entry or disclosure | `atoms.tsx` | via `Segmented control` and `Record tabs` |
| `Checkbox` / `Radio` | A tick with its label as the other half of the click target, and the product's only tick. Many-of-many is `Checkbox`; one-of-many is `Radio` sharing a `name` (`ChoiceList` where the whole question needs naming). `tick-spelling.test.ts` fails any other spelling | `atoms.tsx` | ✅ |
| `Field` | The label-above-control row every form is built from. It owns the id and hands the control `{ id, required, aria-describedby, aria-invalid }`. Slots: `hint` (the rule that always applies), `error` (a refusal), and `icon` / `trailing` (affordances inside the control's outline). `labelEnd` puts a "Forgot?" beside the label outside the accessible name; `labelHidden` keeps the label for assistive tech only | `atoms.tsx` | ✅ |
| `usePasswordReveal` | A password field's reveal control and the matching input `type`, returned together so they cannot get out of step. Pass it as `Field`'s `trailing` | `passwordreveal.tsx` | ✅ (via `Field → Affordances and refusal`) |
| `useClipboardCopy` | Copies one string to the clipboard and returns the button label, the copied state, the copy action and the failure notice. It checks for `navigator.clipboard`, which is undefined outside a secure context. It takes the text, so `copied` clears when the text changes. Labels and `remedy` are the caller's; `clipboard-spelling.test.ts` fails a hand-rolled copy | `clipboardcopy.tsx` | ✅ |
| `FieldGrid` / `FieldRow` | The label/value grid around a record's fields, with a fixed-width label column so values line up on every record and locale. A read-only row takes a plain node; an editable one wraps `InlineText`/`InlineChoice`. `valueRef` exposes the value cell; `stacked` seats a grouped editor full width. Pass `InlineChoice` `hideLabel` so the row's label is the only one. `FieldGrid icons` adds a decorative glyph column for every row | `fieldgrid.tsx` | ✅ |
| `MoneyInput` | An amount with its currency, formatted at the presentation edge | `moneyinput.tsx` | ✅ |
| `FileDropzone` / `FileDropzoneControl` | Choosing one file by drop or click. The `<input type="file">` is the control and the zone is chrome over it, so the keyboard path works. `emptyLabel` is the caller's. An empty selection never fires `onPick`, and the input clears after each pick. `accept` filters the picker only, not a drop. `FileDropzone` brings its own `Field`; use `FileDropzoneControl` inside a `Field` you already render | `filedropzone.tsx` | ✅ |
| `ServiceAccountKeyField` | A Google service-account key, pasted or chosen as its `.json` file, in one box. Only the latest read fills the box, and a hand edit counts as newer. `serviceAccountProblem` names what is wrong before anything is sent; the key is never echoed back | `serviceaccountkeyfield.tsx` | ✅ |
| `SegmentedControl` | A small closed set of options, all visible at once. `counts` puts a per-option count beside each name (omit a count you cannot state; a missing count is not zero). `marks` puts an `aria-hidden` dot on options with something waiting, and is never the only carrier of that fact | `atoms.tsx` | ✅ |
| `ChoiceList` | One question with every answer readable at rest: a native `fieldset` and `legend` of radios, a legend that can be hidden without going unnamed, and an optional `description` per answer that is part of its label. Use it over `Select` for a decision a reader must weigh, and over `SegmentedControl` when an option is a sentence. `disabled` on one choice refuses it; its `description` says why | `choicelist.tsx` | ✅ |
| `Switch` | A setting that writes when you flip it (`role="switch"`), where a `Checkbox` states an intent a later submit sends. `pending` follows `Button`'s semantics. `reason` refuses the flip by itself. `describedBy` adds an element the caller owns, such as a `SettingRow`'s description. A filter over a list is a pressed button, not a switch | `switch.tsx` | ✅ |
| `MeetingSlots` | Available meeting intervals with explicit selection, empty and disabled states; labels are supplied in the viewer's zone. Selection returns only start and end instants | `meetingslots.tsx` | ✅ |
| `MeetingWeek` | A week of free times as one column per day, for a host comparing days. Each slot has a full accessible `label` and a short `time` face; `selected` takes several starts. Without `onSelect` it is a read-only preview. An empty day says so in words | `meetingslots.tsx` | ✅ |
| `SaveBar` | The unsaved draft of a whole settings page, pinned to the foot of the view: one Save for several sections. Render it only while something is unsaved; its content is the caller's sentence and verbs | `savebar.tsx` | ✅ |
| `RecordPicker` | Search → candidates → pick, for choosing an existing record. `id`, `aria-describedby` and `aria-invalid` connect an enclosing `Field` to the search input | `recordpicker.tsx` | ✅ |
| `PassportSelect` / `ScopeChips` | Which agent passport, and the scopes it carries | `passportselect.tsx` | ✅ |
| `Calendar` | A month of days with one marked, for a date chosen inside a larger decision (scheduling a send). Everywhere else use `DateInput`. Presentational: the month, the chosen day and `today` come from the caller. Always six weeks. `refusal(day)` strikes through a day that cannot be offered and adds the reason to its accessible name | `calendar.tsx` | ✅ |
| `FilterPills` | Cuts through a list, one in view (All / Conversations / Changes), each its own outlined pill. Use `SegmentedControl` for a setting instead. `count` is optional per pill, and a missing count is not zero. `layout="list"` stacks the cuts as full-width rows | `filterpills.tsx` | ✅ |
| `SwipeRow` | A list row answered with the thumb where there is no width for its verbs. Stage, then confirm: a horizontal drag past 56px reveals the action and a press runs it. Horizontal only, so vertical scrolling is never read as an answer. It wraps the whole row. Repeated flicks walk several actions; `onStage` fires on the gesture. The gesture is an addition: callers keep their buttons where there is room and owe another path to these actions | `swiperow.tsx` | ✅ |
| `StageLadder` | The stages a record climbs, the one it stands on, and the move to any other, as one control. A `role="group"` holding an `<ol>` of chevrons; `current` is a marker, not a button; `done` is the trail behind it; `terminal` steps (won, lost) sit in their own `<ul>`. The run stays on one row and scrolls. `reason`/`reasonId` pass to `Button`'s refusal; `hint` is the line under it | `stageladder.tsx` | ✅ |
| `SortableList` | Rows a reader orders by hand: a grip per row, ↑ and ↓ from the keyboard, and the new place announced in a polite live region. `renderItem` draws the row; `onReorder` receives the whole new order once, on release. Uses pointer events, so it works on phones. No `onReorder` draws no grip; `busy` keeps grips focusable and refusing; `selectedKey` marks the open row. `orderAtPointer` / `moveKey` are exported | `sortablelist.tsx` | ✅ |
| `IconAction` | A verb whose glyph is its whole label: square, named through `aria-label` and on hover through `useTooltip`, both from one `label` prop. Use it only where the glyph is the verb (mail, phone, calendar, pencil, link, the overflow ellipsis); keep words for a verb whose consequence must be read first. `reason` / `reasonId`, `disabled` and `pending` pass to `Button`. `pressed` draws `aria-pressed` for a glyph that sets a state; `disclosure` draws `aria-expanded` and `aria-controls`; `inline` fits the square into a line of text | `iconaction.tsx` | ✅ |
| `ChipValueList` | The value step of one filter: a fixed list of radios, or a search box over a large set (a workspace's companies) with its own empty, loading, error and result lines. One value per filter. The chip's own `search` picks the shape | `listfiltervalues.tsx` | ✅ |
| `InlineChoice` | Hover-to-edit on a record page: a chooser that commits on pick. A reader who may not edit sees the plain value. A refused save keeps the answer and shows as the field's `error`; re-picking the stored value is no edit. `onEditingChange` pins the record version; `onDirtyChange` reports unsaved drafts. `FieldRow` puts it in the grid | `inlinechoice.tsx` | via `Field grid` |
| `InlineText` / `InlineEditVerb` | Hover-to-edit on a record page: a text field that commits on Enter or blur, with the same refusal, version and dirty behaviour as `InlineChoice`. `multiline` is for a paragraph (Cmd/Ctrl+Enter commits). `type` supports email, date and numeric validation; `step` sets numeric precision. `display` is the resting text when it differs from the edited value. `verb` makes the trigger `InlineEditVerb` (`Change {field}`); `InlineEditVerb` alone serves an edit that opens elsewhere | `inlinetext.tsx` | via `Field grid` |

### Images and icons

| Primitive | For | File | Story |
|---|---|---|---|
| `Avatar` | A record's round chip (contact, company, colleague's seat): monogram, optional logo, and a soft mesh ground under every monogram. The mesh comes from an FNV-1a hash of `identity` in `avatarmesh.ts`, skips the `--ai` and `--danger` hues, and `avatarmesh.test.ts` holds its contrast in both themes. `identity` is required and is the record's own id, so one record draws one mesh everywhere (`recordmark.test.tsx`). With a logo, no mesh is drawn; a failed logo falls back to the monogram. `size`: `sm` (default), `md`, `lg`, `xl`. The monogram is `--fontHeadingXXSmall` at every size | `atoms.tsx` | ✅ |
| `CompanyLogo` | A company's full logo in a caller-sized slot, keeping a wide wordmark's proportions. The caller's fallback stays visible until the image paints and returns on failure | `companylogo.tsx` | ✅ |
| `AvatarStack` | A group of contacts or users as overlapping monograms, folding past `max` into a "+N" chip. Expects a non-empty list. Its separating ring is a `box-shadow`, so a stacked face is the same size as a lone chip | `avatarstack.tsx` | ✅ |
| `ProviderMark` | A federated provider's own sign-in mark, drawn in that company's colours. It leads a `Button variant="federated"` and keeps its own size there | `provider-mark.tsx` | via `Button` |
| `AmbientWaves` | A WebGL2 canvas of slow indigo ribbons for the ground of a place a reader arrives in (the sign-in surface and `OnboardingStage`), never behind a working surface. It is `aria-hidden` atmosphere, not provenance colour. It sits at `z-index: -1`, so the host must open a stacking context (`position: relative; isolation: isolate;`), as `auth.css`'s `.auth-surface` and `onboarding-stage.css`'s `.ob-page` do. Without WebGL2 it renders an empty canvas over the caller's own ground. Its only prop is `className` | `ambient-waves.tsx` | ✅ |

### Labels

| Primitive | For | File | Story |
|---|---|---|---|
| `Badge` | A label a reader reads and never presses: a 20px pill in `--fontBodySmall`, sentence case, no dot by default. `variant`: `soft` (default, a tint with a same-tone hairline) or `primary` (solid fill, for a count or the one status a reader must not miss). `tone` comes from `BADGE_TONES`: `default`, `accent`, `info` (also work in flight), `success`, `warning`, `danger`, `ai` (an agent proposed it; always draws `Sparkles`, see [Indigo says a machine did it](#indigo-says-a-machine-did-it)), `discovery` (something new). `icon` leads the label; `live` puts a breathing dot there instead. Long labels truncate; `wrap` wraps instead. A kicker is `Eyebrow`; a clickable chip is a control, not a badge. `badge-spelling.test.ts` fails a hand-rolled pill or a restyled `.badge` | `atoms.tsx` | ✅ |
| `TagPill` | One tag as its word with a tone dot, built on the neutral soft `Badge`: a tag's colour carries no meaning, so it goes on the dot. An archived tag drops its dot and shows its archived wording | `tagpill.tsx` | ✅ |
| `VisibilityBadge` / `VisibilityLine` | Who may read a thing, as one mark: an icon for the shape of the audience and a word, in six states: `team`, `workspace`, `participants`, `selected`, `private`, `withheld`. A message narrowed by its record's reach says "Team"; a shared company or contact says "Shared". `private` reads "Private". Only `withheld` takes a tone. `opens` draws it as a `Popover` trigger (the record header's access chip in `screens/recordaccess.tsx`). `VisibilityLine` seats the verb that changes the state beside it | `visibility.tsx` | ✅ |
| `RowTags` | A record row's tags as a chip strip: two words then "+N", with the rest named in the title. Passive, single-line, and draws nothing for a row with no tags | `rowtags.tsx` | ✅ |
| `RoleBadge` / `FieldGuard` | A principal's role, and a withheld value that reads as withheld rather than absent | `rbac.tsx` | ✅ |

### Layout and structure

| Primitive | For | File | Story |
|---|---|---|---|
| `ActionRow` | A row of verbs that divides: secondary verbs on the leading edge and one call to action on the trailing edge. The two groups are real elements, so when the row wraps on a phone the primary stays on the trailing edge. No `role`, no label. Not for a form's submit row (`.form-actions`) or a modal footer (`.modal .actions`). `DecisionCard` is the model | `actionrow.tsx` | ✅ |
| `.card-actions` | A card's trailing action row: the verbs under a card's body, with the air above them and the gap between them. Use `.form-actions` for a form's submit row instead | `atoms.css` | via `Attention` |
| `.cell-actions` | A table row's verbs in its trailing cell: right-aligned, one line, no margin, no wrap. A narrow cell scrolls with the table inside `TableScroll`. Use a word for each verb | `atoms.css` | via `Senders` |
| `SettingList` / `SettingRow` | One settings decision per row: what it is and does on the left, its value on the right, at one x down the page. Use it in every settings card. `control` takes a node or a function receiving `{ id, aria-labelledby, aria-describedby }`; a `Switch` keeps its own label but takes the row's description through `describedBy`. `value` shows the current answer when the control does not. `layout="stack"` puts a control that is the subject (a table, a matrix, a log) below the naming; complexity picks it, not size; a control needing two inputs goes behind a verb in a `Modal`. The list draws the hairlines | `settingrow.tsx` | ✅ |
| `NumberSetting` / `NumberSettingRow` | A whole number an admin sets inside a range, in a `SettingRow`'s right column (`control` takes the row's function form). `NumberSettingRow` is the whole row when a label and a description are all it needs. It commits on Enter or blur, like a rename, and only a whole number in range reaches `onCommit`. Anything else stays in the box, and the caller's `refusal` is added to the row's description, so the reader hears the rule and how they broke it. `refusal` comes from the caller because only the caller knows the unit. Typing the stored value back is no edit. Pass `min`/`max` as `{ min: N, max: M }` keyed by the wire property: `backend/gates/settingbounds_test.go` holds that shape to the contract. | `numbersetting.tsx` | ✅ |
| `Disclosure` | A section the reader opens when they want it | `atoms.tsx` | ✅ |
| `Card` | The one card surface. `as` picks the element (`section` by default, also `div` / `article` / `form` / `li`), `inset` is the recessed variant, and `title` / `actions` render its `SectionHeader`. A hand-rolled `<div className="card">` is a second card the moment one of the five chrome values moves. A titled `Card` is for a card inside something else (a list row, an inset, an auth card); a titled surface in a page's own column is a `Panel`, and `cardzones.test.ts` fails a titled `Card` under `src/screens/` outside its allowlist | `atoms.tsx` | ✅ |
| `SectionHeader` | A block's heading: its title and, optionally, the verbs that act on it, never a description. Descriptive copy goes in the body. The title is `--fontHeadingMedium`; `level` `2` or `3` picks the element only, and `1` is the page's own name at `--fontHeadingLarge`. `panelhead.test.tsx` fails a description prop or class and a title at another size | `atoms.tsx` | ✅ |
| `Panel` / `PanelBody` / `PanelIntro` / `PanelRow` / `PanelPlate` / `PanelGroupHead` | The zone pane: one zone of a record (or any page column) on translucent ground with a hairline edge. The header band is fixed at `--panel-head-h` (56px) and holds a title and optional verbs, never a description; a long title truncates. `PanelBody` is padded content, `PanelRow` a full-bleed row (`interactive` makes the whole row one press target), and stacked `PanelBody`s get a seam. `PanelIntro` is the one descriptive line, first in a `PanelBody`. `tone` from `PANEL_TONES` marks the single lead card on a page: `accent`, the five states, or `ai` for a body a machine wrote. `actions` is a band for verbs that change the panel. `PanelPlate` is a recessed plate for context. `titleLevel` sets the element. The title labels the `<section>` as a landmark. `PanelGroupHead` names a group inside the pane. `panel.test.tsx` and `panelhead.test.tsx` hold the head's geometry and type | `panel.tsx` | ✅ |
| `RailPanel` | A `Panel` that tells an empty section from a withheld one. A section the reader's role cannot read is named in `sections_omitted`, so the card says it is hidden instead of drawing an empty list. Message states are `SurfaceState` inside a `PanelBody`; `ready` renders edge to edge. The `footer` shows only on `ready` and `empty` | `panel.tsx` | ✅ |
| `PageZones` | The page's columns as one grid: a work column plus up to two rails, in four shapes (`single` / `rail` / `aside` / `both`). The work column stays first and largest, folds to full width at 1200px, and the rails stack under it at 720px. It carries only the grid; `RecordView` composes it. Pass the shape that matches the slots you fill. `asideOpen` folds the details pane over `--dur-move`; the folded `<aside>` is `inert` and then unmounts | `pagezones.tsx` | ✅ |
| `OnboardingStage` | The framed card every onboarding question is asked in, with the Core beside a hero headline. The card is capped to the window and the board scrolls inside it, so the rail's Continue stays reachable. Mount it once per flow; a stage per step restarts the Core. `lit` says a model is bound (read from the server). `anchor="start"` is for a board that grows while read. `where` names the part of a multi-screen stop; `hint` is the foot's one line. The band shows the mark, the step's name, the progress marks and the Core's state in words. `coreScale` shrinks the orb by transform; `coreHidden` fades it when the board draws its own. `useStageTitleFocus` moves focus to the title after a board swap | `onboarding-stage.tsx` | ✅ |
| `StageActions` | A step's way onward, portalled from inside the board onto the `OnboardingStage` rail, so the step keeps its own state and the button stays in view. Without a rail the actions render in place. The primary always presses, and pressed early it names what is still needed | `onboarding-stage.tsx` | ✅ |
| `RecordView` | The record page shell: identity, readings, timeline, with `PageZones` underneath. `tabs` is its own slot directly under the identity. `band` holds what describes the whole record (readings, stepper, a refusal sentence) and scrolls as one row. It owns the column rhythm (`.record-rail`, `.record-aside`, `.record-timeline`). `timelineAnchorId` puts an id on the timeline section. `asideOpen` passes to `PageZones`. Its phone action bar uses the card inset so disabled-action explanations stay clear of its edges | `recordview.tsx` | ✅ |
| `.record-stack` | The work column's stack: one vertical column of full-width panels. Wrap a tab body that draws more than one panel in it; the work column itself has no interval, so bare siblings would touch | `composed.css` | via `Record view` |
| `.meta-row` | A list row read as a face and a body on a `PanelRow`: a meta line (who, source, time), the content under it, and a trailing action at a fixed x. Named cells (`.meta-row-line`, `.meta-row-who`, `.meta-row-time`, `.meta-row-action`, `.meta-row-entry`, or an `.emailentry`) so a caller can omit one; a row without an `.avatar` drops the face column | `composed.css` | via `ContactMemory`, `CompanyFactsPanel` |

### Loading

| Primitive | For | File | Story |
|---|---|---|---|
| `Skeleton` | One placeholder bar. Use `PendingBody` unless you are drawing a shape (a ring, a chart) that lines cannot stand in for | `atoms.tsx` | ✅ |
| `BusyMark` | The turning mark a control shows while its write is in flight. Decorative (`aria-busy` on the control is the fact). Under reduced motion it stops turning and stays visible | `atoms.tsx` | ✅ |
| `PendingBody` | The one pending state. `label` is required: it is what a screen reader hears, so name what is loading. `lines` reserves height so the page does not jump. `visible` shows the label too. `delayMs` holds everything back until the wait is worth reporting (for surfaces that re-read as the reader types). `QueryGate` / `QueryStates` / `SurfaceState` render it and forward `pendingLabel` / `pendingLines` (`loadingLabel` / `loadingLines` on `SurfaceState`). Fitness functions keep one home for the announcement and the pulse | `atoms.tsx` | ✅ |

### Messaging

| Primitive | For | File | Story |
|---|---|---|---|
| `ErrorLine` | The line that says a write or read failed, under the control or submit row it belongs to: a `<p role="alert">` in `--dangerText`. Pass `error` (the thrown value; draws nothing while null, words from `problemMessageOf`) or `children` (a translated sentence), never both. `id` is for `aria-describedby`; `actions` adds a verb on the same line; `inline` draws a `<span>`; `standing` drops the alert for a state already true on load. It owns no margin. Use `Field`'s `error` for a field, a `danger` `Callout` for a surface's statement, a `Toast` for a transient one. `errorline-spelling.test.ts` fails other spellings | `errorline.tsx` | ✅ |
| `Callout` | What a surface says about itself, in seven tones from `CALLOUT_TONES`: `info`, `accent` (info said emphatically), `warning`, `danger`, `success`, `discovery` (something new to this reader), `ai` (a machine produced what it is about). `title` is required and a title alone is the common shape; the tone's glyph (replaceable through `icon`), an optional body, the caller's verbs and a `dismiss` follow. Tone colours only the icon and the heading through `--callout-ink`. `kind` (`outcome`, `standing`, `event`) derives the announcement; `live` overrides it. No `className` | `callout.tsx` | ✅ |
| `EmptyState` | What a surface shows when it has nothing to show. Bare: the one-line "nothing here". With `title` and `plate`: an empty group inside a pane, as a dashed plate, its verb in the group's `PanelGroupHead`. With `title` alone: the first-run state with a paragraph and one `action` that creates the first record (the projects list is the model) | `atoms.tsx` | ✅ |
| `ToastProvider` / `ToastRegion` / `useToast` | The transient confirmation, fixed to the foot of the viewport. One provider and one region, mounted in `main.tsx`; a screen only calls `show`. It withdraws after 3.5s unless hovered or focused (WCAG 2.2.1). `action` adds a verb (such as Undo) and makes the toast stay until dismissed; a reporting toast queues behind it. `tone` from `TOAST_TONES`: `success` (default), `danger`, `warning`, `info`, `discovery`. The `<output>` is the live region, and focus is never taken. Which writes get a toast is set out in the story | `toast.tsx` | ✅ |
| `SurfaceState` | The nine states a surface can be in (`ready \| empty \| withheld \| unavailable \| loading \| unsupported \| failed \| stale \| partial`) as one component, with `sectionState()` to classify a composite read's section and `omitted()` to ask whether a grant withheld it. Only `empty` may say "there is none". `stale` puts its caveat above the rows and `partial` its count below. `emptyLabel` and `loadingLabel` are the caller's words; `label` and `labelLevel` name one part of a card. `SECTION_STATE_TONES` colours only the verdicts (`loading`, `failed`, `stale`, `partial`). `detail.withheldReason` replaces the generic withheld sentence | `surfacestate.tsx` | ✅ |
| `CardBoundary` | A render boundary around one card, so one card's throw leaves the shell and navigation in place. It says the card failed and retries with the query cache reset. It never shows the error's text | `cardboundary.tsx` | ✅ |

### Navigation

| Primitive | For | File | Story |
|---|---|---|---|
| `.entity-link` | One of our records named inline and reachable: a contact in a sentence, an account in a value, a sender in a header. Accent-coloured, underlined on hover, no chrome. A class, because building a route is a screen's job: a screen uses `screens/entityref` (`EntityRef` / `RecordRef`), and a design-system component that names a record wears the class on its own anchor (as `EmailDetail` does). Use `OffsiteLink` for an external address and `Button variant="link"` for an action. `newTab` on `EntityRef` sets `target` and `rel="noopener noreferrer"` together | `atoms.css` | via `Email entry` |
| `OffsiteLink` | A destination off our origin: a link when `webUrl` accepts the address, plain text when it does not. It sets `rel="noopener noreferrer"` and wears `.link-button` unless the caller passes a class. Named for the destination so it does not collide with lucide's `ExternalLink` icon | `offsitelink.tsx` | ✅ |
| `ContactLink` | An address or phone number the reader can act on, kept as plain text when the value is not valid (`format/contacturi` decides). Inside the product an address opens the composer on the record the caller names, hosted once in the shell by `screens/writeto`. Without a connected mailbox, or with no composer host mounted, it falls back to a `mailto:` link. A page with its own composer answers there. A number is a `tel:` link. `readOnly` keeps the address as text | `contactlink.tsx` | ✅ |
| `RecordTabs` | The strip that chooses which body of a record is open: quiet text tabs at `--fontWeightMedium` with the accent under the current one, spanning the work column. `counts` is partial, as in `FilterPills`; `trailing` holds the control that opens the details column. A strip with one body draws it as a label, not a button | `recordtabs.tsx` | ✅ |
| `Breadcrumb` | Where the reader is and how they got here: a named `nav`, an `<ol>`, one `<li>` per stop. The last stop is never a link. Separators are `aria-hidden` spans inside the item they follow. Only the last stop truncates, with `useTruncationTooltip`. No `className` or `style` | `breadcrumb.tsx` | ✅ |

### Overlays and layering

| Primitive | For | File | Story |
|---|---|---|---|
| `Modal` | The one dialog: portalled, Escape-closing, Tab kept inside. `placement="right"` is the drawer, floating with a gap and `--shadow-pop`; on a phone both are a full-screen sheet. `placement="full"` is the lightbox for content to read (`FilePreview`). Every dialog draws one way out besides Escape: an `IconAction` X in the top-trailing corner, last in tab order and never where focus lands. `closeReason` refuses that close with a sentence. It animates in and out (`usePresence`), `inert` while leaving. A title is a `Heading` with `modal-title`, which owns the space under it; omit the class inside a gap container | `modal.tsx` | ✅ (`Dialog` centred, `Drawer` right, `DrawerWide` banded) |
| `ResolveSheet` | Answering a finding from the nightly input check, as a right-placed `Modal`. Which fields are required depends on the outcome, mirroring the server's rules: `value_correct` and `not_relevant` ask why and offer an expiry, `remind_later` asks when. It submits only the chosen outcome's fields. `condition_cleared` is not offered. Holds no copy. `error` shows a refused save as an `ErrorLine`; `returnFocusTo` passes to `Modal` | `resolvesheet.tsx` | ✅ |
| `ConfirmModal` | A dialog that asks before something irreversible. It owns the heading, the error line and the Cancel/Confirm pair. `confirmDisabled` refuses as a state; `confirmReason` refuses with a sentence through `Button`'s `reason`. Pass one or the other. The footer wraps only between `actionsLead` and the Cancel/confirm pair | `confirmmodal.tsx` | ✅ |
| `NamePrompt` | A write whose only input is a name: trigger, dialog, name box, save. `icon` leads the trigger. It refuses an empty name and clears the box on success, using `ConfirmModal`'s chrome. `onSave` receives a `done` callback, so a refused write keeps the dialog open with the text in it | `nameprompt.tsx` | ✅ |
| `OverflowMenu` | The verbs a record offers but a reader rarely wants. Items are words with no glyph; `overflowmenu-icons.test.ts` fails a glyph inside one. On a record page the order is Edit, Merge, Share, Full history, the record's own verbs, then Archive. Icon-only verbs stay outside, as `IconAction` | `atoms.tsx` | ✅ |
| `Popover` | A short aside on click, portalled beside its trigger, for a paragraph or small list that may carry a link. Use it where a `Disclosure` would push the page around; use `useTruncationTooltip` for one hover line. Closes on Escape (returning focus) and on an outside click. `disabled` refuses to open but never closes an open panel or disables an open trigger. No `pending` | `popover.tsx` | ✅ |
| `FilePreview` / `FilePreviewProvider` / `useFilePreview` | One stored file opened over the darkened page, with save, print and close. `FileChip` opens it. The bytes are fetched once into a blob, and the media type comes from the filename against `previewMediaType`'s allowlist in `filechip.tsx`, so no script-capable type (HTML, SVG) runs under our origin. A PDF goes in a frame and prints itself; an image goes in an `<img>` and the page prints it through the `@media print` rules on `.modal-full` and `.file-preview-*`. A kind nothing can draw, or a modified click, keeps the download. One provider in `main.tsx`, held by `conformance.test.ts` | `filepreview.tsx` | ✅ |
| `useTooltip` | A control's own name on hover and on focus, for a control that cannot draw it. It lands in `aria-describedby`. Same machinery as `useTruncationTooltip` | `tooltip.tsx` | via `Icon action` |
| `Menu` | The popover panel `ListSurface` hangs its sort, filter and column sets in: a `fieldset` with a heading; `align` picks the opening edge | `listsurface.tsx` | via `List table` |
| The sort menu | Every attribute the server can order the list by, including hidden columns and the server's own default order. `ListTable` derives it from its columns, and the header and the menu both read `nextSortValue` | `listsurface.tsx` | ✅ (`List table → SortedByAMenu`) |
| `useTruncationTooltip` | The whole of a truncated string on hover or focus, and nothing when it fits. Use it wherever user data of unbounded length is drawn on one line | `tooltip.tsx` | ✅ |

### Status indicators

| Primitive | For | File | Story |
|---|---|---|---|
| `CommunicationStatus` | Whether a message can go, shown where the send is decided. Six states told apart by shape: `context_only`, `ready` (`MailCheck`), `attention` (`MailWarning`), `restricted` (`MailMinus`), `checking` (`BusyMark`, no envelope), `external`. `scope` sets the meaning: `contact_preferences` (never reaches `ready`) or `current_message`. `label` and `name` are required and translated. `exception` never recolours the mark. `CommunicationStatusLine` seats the mark beside the verbs, never inside one | `communicationstatus.tsx` | ✅ |
| `StageStrip` | A pipeline's shape at a glance: open stages left to right, shaded by win probability, with won and lost at the end. A reading, never a control (`StageLadder` moves records). `compact` is the decorative bar for a list row; `empty` is the label while there are no stages | `stagestrip.tsx` | ✅ |
| `StrengthMeter` | A relationship's strength band as three rising bars over its word: `strong`, `moderate`, `weak`, `none`. The word is the fact. `strengthmeter.css` is shared by the contact Routes panel | `strengthmeter.tsx` | ✅ |
| `PipelineLadder` | One message's path through the ingress pipeline, with what each step did and why. It renders whatever stages the server sends, using the server's `label` / `reason_text` for one it does not know. Its statuses keep `not_applicable` apart from `unknown`, and `withheld` (always rendered to a non-owner) apart from an omitted step | `pipelineladder.tsx` | ✅ |

### Text and data display

| Primitive | For | File | Story |
|---|---|---|---|
| `EmailEntry` | One retained email as a row, and the one reading of a message in the product. Takes the server's `EmailSummary` and a formatted timestamp; no density or variant props. `onOpen` opens the reader; `onSelect` and `selected` let a composer pick a reply target. It owns its padding, type, truncation and focus ring. A withheld row keeps its shape and loses its words | `emailentry.tsx` | ✅ |
| `EmailDetail` | One email read whole, in `Modal`'s drawer form over the record. `open` defaults to true; closed, it stays mounted to animate out, and reopening refetches. It fetches on open only. Quoted history and the sign-off fold behind a control. A withheld message draws `SurfaceState`'s `withheld`. Resolved participants and filed records are links that open in a new tab. Record names and the reply composer come in as render props | `emaildetail.tsx` | via `Email entry` |
| `OpenEmailDrawer` | The record page's one email drawer: `EmailDetail` plus the state that keeps the last message in place while the drawer closes. Mount it on the record, not inside a tab. It binds the access editor, filed links and reply verb, so every surface that opens a message offers the same three | `openemaildrawer.tsx` | via `Email entry` |
| `EmailText` | Full normalized email text with paragraphs, a visible sign-off and expandable quoted history, shared by the email reader and composer preview | `emailtext.tsx` | ✅ |
| `EmailWords` | A message's words alone (the server's preview line), for a thread card that already draws the sender and time. It applies `EmailEntry`'s withheld rule | `emailentry.tsx` | via `Email entry` |
| `EmailReference` | A citation of an email: subject, date and opener, with no preview and no access badge. For naming a message inside another layout (a chronology, a brief's evidence row, a graph receipt). `stacked` puts the date under the subject for a narrow column | `emailreference.tsx` | ✅ |
| `ActivityReferenceList` | The activities a derived number was computed from, as rows a reader can check. Email rows render `EmailReference` and open through `onOpenEmail`; other kinds are text. A row the reader may not read says so and offers nothing to press. The count beside it stays the caller's | `activityreferencelist.tsx` | ✅ |
| `CellStrip` | Several pills in one table cell, on one line: text beside a badge truncates first, then each badge. It never wraps. Use `RowTags` for a tag column | `listtable.tsx` | ✅ (`List table → Cell strips`) |
| `Markdown` | A knowledge-corpus document, read-only, with the cited passage marked. `markdown-parse.ts` returns data with a closed set of shapes and this file builds every element, so raw HTML shows as text and never runs. A link is an `<a>` only for `http`, `https` or `mailto`; any other link shows its label. Tables go in `TableScroll`, named by their header row. It supports the syntax the shipped handbook uses; anything else renders as source text. `highlight` reports through `onHighlight`: the quote found and marked (whitespace collapsed, as `claims.CollapseSpace` does), the `line` banded, or `"none"` | `markdown.tsx` | ✅ |
| `InlineMarkdown` | One line of markdown, inline: bold, italic, code and links, using `Markdown`'s parser and element list. For a model's written answer. `links={false}` keeps link labels and drops hrefs, for prose nothing quote-checks. Use `Markdown` for headings or tables | `markdown.tsx` | ✅ |
| `FactList` | Label→value pairs a reader scans. Rows arrive as an array, so a caller drops absent facts instead of showing blanks | `factlist.tsx` | ✅ |
| `ProjectLinks` | The one section where a record shows its projects and where a reader attaches or detaches one. The record page supplies an adapter (what is linked, how to link and unlink, whether several are allowed); the component owns the surface, verbs, empty, confirm and refusal states. `allowsMany: false` turns "Attach" into "Move". The project page uses it for its companies | `projectlinks.tsx` | ✅ |
| `Heading` | The one spelling of a heading. `size` (required) is one of the seven `--fontHeading*` steps; `as` overrides the default element (`xxlarge` / `xlarge` → `h1`, `large` → `h2`, `medium` → `h3`, `small` → `h4`, `xsmall` → `h5`, `xxsmall` → `h6`, plus `div` and `span`). No margin and no colour. Other attributes pass through. `heading-spelling.test.ts` fails a raw `<h1>`–`<h6>` under `src/` and holds `heading()` in `mcp-apps/bridge.ts` to the same table | `heading.tsx` | ✅ |
| `Kbd` | A key cap, in the body face: a key someone presses is not code | `atoms.tsx` | ✅ |
| `StatCard` | One reading with its basis. `label` and `value` are required strings; an empty reading says "0" or "None yet". The figure is `--fontHeadingLarge` with tabular digits on one line; the label one line in `--textSecondary`; `detail` two lines in `t-caption`. `tone` from `STAT_CARD_TONES` (`info`, `success`, `warning`, `danger`) colours the figure in `--<state>Text` and the `meter` in the base. `alert` tints the tile. `basis` opens a `Popover` of source rows from a trigger on the label line. `onOpen` adds the fixed "Open →" door; the whole card is its press target. `narrow="row"` folds the tile into a line in a narrow `StatStrip`. `source` may hold an `IconAction inline`. `statcard.test.tsx` holds the ink and the door | `statcard.tsx` | ✅ |
| `StatStrip` | A record's readings as one plate of ruled slots, read across as a single comparison. It takes `StatCard`s and owns only the plate: slot count from the children drawn, the rules between slots, and the fold (the last slot fills its row). A slot may be a `button` wrapping the card, for a strip of filters; a reading that opens something uses `StatCard onOpen`. `floor` carries a caveat for the whole row. Compact readings fold into full-width rows in narrow containers, with the explanation and action each on their own line | `statstrip.tsx` | ✅ |
| `ReadingsFloor` | The sentence a row of readings shows when its source was read to a limit, so every figure is a floor. It belongs to the row; `StatStrip` and `ReadingsGrid` both draw it through `floor` | `readingsfloor.tsx` | via `Readings grid` (`TheFloorAlone`) |
| `ReadingsGrid` | A record's readings as free-standing cards, each used on its own with its `basis`, `meter` and `onOpen`. It owns the row: `auto-fit` columns on a 9.6rem floor, every card the height of the tallest | `readingsgrid.tsx` | ✅ |
| `Meter` / `Sparkline` / `Chip` | A proportion as a bar (`value` and `max`, never a percentage), a short series as a bare polyline, and one attribute of a record as an icon pill (a fact that may be a link). `Meter`: `tone` colours the fill; `restTone` gives the track its own value; `dense` is the thin label bar; `part` draws a contained stricter measure as the solid head of the fill. `Chip dense` matches a badge's 20px geometry for table rows. Size from the props, not from a screen sheet | `readings.tsx` | ✅ |
| `CumulativeChart` / `BulletChart` / `RangeChart` / `GroupedBars` | Reporting readings on shared, zero-based axes: actual/previous event curves with target reference, owner actual against target, median-to-P75 distributions, and independent paired event counts. Null observations leave gaps. Formatted readings and evidence callbacks come from the caller; each chart includes an expandable numeric alternative. Without a comparison label, the comparison legend and controls are omitted. A bullet chart draws a target marker only for an explicit allocation; an absent target is not zero | `report-charts.tsx` | ✅ |
| `BarList` | Several one-dimensional readings as a ranking on one scale: label, bar, formatted amount per row. `max` names a whole the rows do not reach; a `max` below the largest row is ignored. Amounts arrive formatted. Bars are `aria-hidden` and a `sr-only` table carries the values. Long stage and team member labels wrap within their column. Optional `onSelect` makes each full bar an accessible evidence action | `readings.tsx` | ✅ |
| `SegmentBar` | Disjoint parts of one total on one track, with an optional `marker` for a figure the parts are read against (won, evidence and best case with the call marked). One to three parts, most to least certain, in steps of one hue from `--accentText`. The legend names every part and the mark; the track is `aria-hidden`. Amounts arrive formatted. Optional `onSelect` activates individual segments through canonical buttons | `readings.tsx` | ✅ |
| `Waterfall` | A reconciled running-total bridge: opening/closing anchors start at zero, signed movements start at the preceding total, and connectors show continuity. One scale includes every intermediate total and negative values. Visible amounts and an accessible table accompany the geometry. An optional selection callback provides keyboard-accessible evidence actions. Reconciliation is checked in production; reductions use amber because leaving open pipeline is not necessarily a bad outcome | `waterfall.tsx` | ✅ |
| `FileChip` | One stored file as a small card: kind glyph, filename, and a click that opens `FilePreview` for a kind the browser can draw or downloads one it cannot. PDF and images have their own glyphs. The email attachment shelf and the account and contact file lists use it. It takes either `href` or `withheld`, never both. `withheld` is the sentence for a file recorded by name only, such as a private message's attachment whose bytes were never kept. It draws the same card, dashed and quieter, as text instead of a link | `filechip.tsx` | ✅ |
| `DataTable` | A simple column/row table with optional row navigation, inside a `TableScroll`. `label` is required and matches the heading above the table. `align: "end"` right-aligns a column of figures in tabular digits; `grow` names the column that takes the spare width | `datatable.tsx` | ✅ |
| `CellStack` | Two facts in one table cell, one under the other: a value and the fact that qualifies it (a failure sentinel under its time) | `cellstack.tsx` | ✅ |
| `TableScroll` | The box a too-wide table scrolls sideways inside, so the page does not. Use it for any hand-drawn `<table>`; `DataTable` already does. It takes a tab stop and becomes a named `region` only while it overflows. `label` is required | `atoms.tsx` | ✅ |
| `RelationshipMap` | The account's routes as a fixed three-column picture: our colleagues, the account and its deal, their contacts in role lanes. Data-only; strings arrive translated. Selecting a contact lights the strongest route and the panel names it and the alternatives. Fixed layout, no physics. It scales down a little, then scrolls; the side panel moves under the drawing by container query. Strength is stroke width (weakest dashed), kind is shape, engagement is a word. One tab stop, arrow keys walk lane by lane | `relationshipmap.tsx` (geometry and route arithmetic in `relationshipmap.layout.ts`) | ✅ |
| `PipelineBoard` | The pipeline surface: one column per stage, each with its money head and the `DealCard`s in it. A stage is 300px (`--board-col-w`) above the 700px phone breakpoint, so a 1440px screen shows a cut fourth stage; on a phone a stage takes four fifths of the screen and the board swipes. `mailAside` and `cardActions` (`DealCardHooks`) are what a caller hangs on every card | `composed.tsx` | ✅ |
| `DealCard` | One deal on the board. The company slot: `company` names it, `companyWithheld` draws `FieldGuard`'s mask, `companyUnreadable` says the lookup failed, and none of them draws nothing. The lines keep one order: company and owner as a caption, the name clamped to two lines, the compact figure with the close date, then the foot. The foot is how the deal is moving: the stall, the mail line (`lastEmail`) and single-threaded. `mailAside` makes the mail line a hover `Popover` trigger; the line is `DealMailChip`, exported for the deals table. `actions` (`DealCardActions`) adds up to three named `IconAction`s (summary, email, add task) at the foot's trailing edge as bare glyphs, and one left out is absent | `dealcard.tsx` | ✅ |
| `IdentityLine` | The line of facts under a record's name. `separator`: `dot` for clauses of one sentence (account, deal), `space` for separate handles (a contact's address, number, profile). It drops absent children, so no dot is left stranded. `IdentityFact` is one fact with an optional glyph (`quiet` for a qualifying fact); `IdentityMeta` stacks two lines | `identityline.tsx` | ✅ |
| `RecordFacts` / `Fact` | The head's facts strip: caption over value, cells wrapping as the head narrows, as one `dl`. `Fact` sets its `label` in `Eyebrow` over a `dd`. Which facts, in what order, stay the caller's | `recordfacts.tsx` | ✅ |
| `RecordCard` | A record listed elsewhere (an account's contacts, search results) as its mark, name, `position`, and handles a reader can act on. `kind` (company or contact) and `identity` key the mark; `logo` passes to `Avatar`. The mark and name share one link and each handle is its own control; the card is not one link. `aside` is what the surface knows (which colleagues reach this contact). `.record-card-list` is the stack. A name inline in a sentence is `EntityRef` | `recordcard.tsx` | ✅ |
| `GroupedTimelineList` / `TimelineList` / `TimelineRow` | The activity timeline, grouped. Every row carries its day and time in the gutter. A thread is one open card: kind, count, participants and whose move it is over the subject, then the messages (the same `TimelineText` fold), with those past the first three behind a count. A bulk send folds behind its newest copy, drawn through `EmailEntry` | `composed.tsx` | ✅ |
| `CountUp` | A number that climbs while a count is still being earned, from where it was rather than from zero. Tabular figures. Under reduced motion it shows the number immediately. Not for a settled figure | `countup.tsx` | ✅ |
| `CrawlCanvas` | The site drawn as it is read, on the screen where a reader waits for a crawl. Pages hang off the pages that linked to them; indigo marks current work and a read page cools to grey. It draws only what the server sent. Effectively `aria-hidden`; the ticker and counters carry the words. Geometry lives in `crawl-graph.ts` | `crawl-canvas.tsx` | ✅ |
| `Eyebrow` | The kicker over a section: the element carries `.t-eyebrow` from `base.css`. `as` picks `h2` / `h3` / `h4` / `span` / `dt`. A selector-only site (`.firmo dt`) can reach the same declarations | `eyebrow.tsx` | ✅ |
| `ListTable` | A record list as a table: columns, rows, query dials above, footer below. Header cells are sentence case at `--fontWeightMedium` (from `app.css`). Controlled: the screen owns sort, filter and search. `selection` adds row checkboxes and a bulk bar. A column declares its kind: `fixed` (identity), `numeric`, `verbs`. `initiallyHidden` starts a column unticked. Columns stop at their minimum and the body scrolls sideways. `page` with `onPage` lets the address own the page; narrowing resets to page one. `bodyRef` exposes the scroll element (`app/scrollmemory.ts`). See [Building a list screen](#building-a-list-screen) | `listtable.tsx` | ✅ |
| `ListSurface` | The chrome `ListTable` renders into, usable alone: saved-view tabs, the count line, search, filter chips, sort, archived toggle, footer. Left half narrows the list (search, filters, Filter, archived); right half changes how it is drawn (Sort, `displayMenu`, `tools`, Save view). Every verb is a `Button` | `listsurface.tsx` | ✅ |
| `CountLine` | The sentence under a list saying what is on screen out of what exists, and the sort. It spells "23 of 1,204"; `more` keeps an unknown total from reading as known | `listsurface.tsx` | via `List table` |

### Primitives

| Primitive | For | File | Story |
|---|---|---|---|
| `Stack` / `Row` | Space between things, from the scale, and nothing else (no ground, border, padding or type). For extension units, which cannot import CSS and so cannot write a class. `gap` names a spacing step. `Row` adds `align` and `justify` (`between` puts a label left and a verb right) and wraps by default. A visible box is a `Card` or a `Panel` | `stack.tsx` | ✅ |

### AI and provenance

| Primitive | For | File | Story |
|---|---|---|---|
| `SourceEvidence` | The thing a task was read out of. It resolves the source activity's kind through the plain activity read, then draws an email in place through `SourceEmailPanel` or offers a button for a meeting transcript, which opens through the host's `onOpenTranscript`. Nothing is drawn until the kind is known | `sourceevidence.tsx` | ✅ |
| `SourceEmailPanel` | The message a task was read out of, drawn inside the task panel instead of a second drawer. It makes its own read under `emailDetailKey`, so the server checks access again. Its envelope (From, To, Cc, the date and the Bcc-withheld note) is `EmailEnvelope`, exported from `emaildetail.tsx` and drawn there too, so both readings name a message's parties one way. Without it, a reader took the quoted history's header lines for the reply's. No audience editor, filed links or reply verb. A withheld message shows nothing below the state, subject and date included | `sourceemailpanel.tsx` | ✅ |
| `EvidenceReceipt` | What a number was drawn from: how many records were eligible, how many carried the needed values, and what the figure does not cover. `state` is a one-word `Badge`, absent until the caller has one. `calculation` and `calculationSummary` come together or not at all. Presentational; used by forecast, pipeline and agent reports | `evidencereceipt.tsx` | ✅ |
| `AiPending` | The wait for a model's answer: a breathing indigo tile, ragged answer lines and a passing sheen, with the sentence shown and spoken. It composes `PendingBody`; under reduced motion it rests. Only for an agent filling a card; an ordinary fetch is `PendingBody` | `aipending.tsx` | ✅ |
| `EvidenceMark` | The provenance affordance: a dotted underline on a value a human did not type, opening to where it came from. On a value that is already a control it sits beside it with a word ("read", "bought"); `subject` names the value, and the accessible name starts with the visible word (WCAG 2.5.3) | `evidencemark.tsx` | ✅ |
| `AutonomyDot` | The 🟢/🟡 autonomy semantics as a token component, never an emoji glyph | `trust.tsx` | ✅ |
| `EvidenceChip` / `ConfidenceMeter` | How sure we are, and on what evidence. Inside `EvidenceMark` and on the staging surfaces, never stacked under a field. `evidence.lines` adds 1-based line numbers: consecutive numbers merge into a range (`lines 12–14`) and gaps stay listed | `trust.tsx` | ✅ |
| `ProvenanceTag` / `provenanceLabel` | Where a value came from and who captured it: a `Badge` with one label per kind of actor (agent, connector, system job, human, buyer, unrecorded). Only `agent` takes `tone="ai"`. `agent`, `system` and `buyer` may arrive unnamed and then show the kind. `buyer` is a Deal Room participant from outside the company | `provenance.tsx` | ✅ |
| `confidenceLevel` | The confidence bands: a wire 0..1 mapped to the level `ConfidenceMeter` draws, high at 0.8 and med at 0.5, inclusive. Null in, null out: an unrecorded confidence is not low | `trust.tsx` | ✅ |
| `StagingCard` | Staged-not-real state, on the staged variant of the panel-ai family in `panel.css`: dashed until a human accepts it. `DecisionCard` and the deal board wear this class | `trust.tsx` | ✅ |
| `FieldDiff` | The inline old→new value diff; a null side reads as a marker, never a blank | `trust.tsx` | ✅ |
| `PassportChip` | An agent's passport id on a soft `ai` `Badge` | `trust.tsx` | ✅ |
| `DecisionCard` | The card that asks a human to decide something an automation staged, in two layouts: `deck` (tall, whole payload) and `row` (compact list form). It shows the proposed content: a draft's `subject`/`body`, or `current_X`/`proposed_X` through `FieldDiff`. `expires_at` tints the edge in three bands from `decisionUrgency`, and past the deadline Accept is not drawn. Verbs divide on `ActionRow`: Accept on the trailing edge, the rest as `IconAction`s. Verbs are callbacks and words are `labels`; `meta`, `aside`, `editor`, `detail` and `display` (resolved per kind in `screens/approvalkind.ts`) supply the rest. `compact` with `DecisionCompactWords` gives one line per decision with the proposal in a `Popover`. Internals: `decisioncard.payload.ts` (`draftOf`, `diffsOf`, `restFields`) and `decisioncard.content.tsx` (`DecisionContent`, `DraftBody`, `DecisionEvidence`) | `decisioncard.tsx` | ✅ |
| `DecisionStatusChip` | The decision's header chip: the verdict once answered, else the live countdown. It reads `decisionExpiryMs` / `decisionLapsed` / `decisionUrgency`, so it escalates on the same bands as the card's edge. A lapsed pending proposal gets no chip. `labels.expiresIn` builds the sentence; an unknown status reads as lapsed. Takes a `DecisionDeadline` (`status`, `expires_at`) | `decisioncard.tsx` | ✅ |
| `DecisionToolChip` | Names the tool that staged a proposal (`send_email`), distinct from its kind (`advance_deal`). The verb and words arrive from the caller. No verb, no chip | `decisioncard.tsx` | ✅ |
| `decisionUrgency` / `decisionUrgencyTone` / `decisionExpiryMs` / `decisionLapsed` | The deadline vocabulary the card, its chip and every countdown badge read. `decisionUrgency` owns the 1h/6h bands; `decisionUrgencyTone` maps them to a `Badge` tone | `decisioncard.tsx` | ✅ |
| `DecisionDeck` | The queue of staged decisions, answered one at a time. Stage, then commit: a swipe or key stages locally, the tray shows what is waiting, and nothing is sent until an explicit commit, which makes the tray the undo. Drag and arrow keys go through `dragVerdict`/`keyVerdict`; `U` un-stages, `Enter` commits. A `bundle_id` group is one decision. `SegmentedControl` switches to the list form, the default under `prefers-reduced-motion`. `title` puts the heading on the toggle's row; `frame` lets a `Panel` place the toggle and content. `listCap` / `listRest` cap the list form. Parts: `DeckItemCard` (`decisiondeck.item.tsx`); `decisiondeck.verdicts.ts` (`dragVerdict`, `keyVerdict`, `sharedFacts`, `deckKeyHandler`, `verdictSends`); `decisiondeck.stack.tsx` (`DeckStack`, `useDeckDrag`); `decisiondeck.frame.tsx` (`DeckSurface`, `DeckQueue`, `StagingTray`, `useCommitTakesFocus`). In a panel, a queue whose every card is staged draws nothing | `decisiondeck.tsx` | ✅ |
| `MarginceCoreScene` | The product's AI identity: a glass ball with four ribbons, drawn on the GPU, in five lifecycle states (idle · ingest · working · warning · error). State is motion first (`margince-core-motion.ts`), colour second. `aria-hidden`, no click. Callers pass `state`; `size` picks hero or chrome, `surface` dark or paper; sizing via `--coreSize` / `--coreGlass` / `--coreHalo`. Without WebGL2 it draws a static dress. `margince-core-shader.ts`, `margince-core-gl.ts` and `margince-core-engine.ts` are internals | `margince-core.tsx` | ✅ |
| `AiRuntimeChip` | Which model answered, how many calls, and what it cost. The spend is always shown; hover reveals the per-model breakdown and a press pins it. The breakdown scrolls within its available space (`useScrollRegion` in `scrollregion.ts`), named by its heading, with Escape returning focus | `airuntimechip.tsx` | ✅ |

### Libraries

| Primitive | For | File | Story |
|---|---|---|---|
| `useRecordTimeline` / `useTimelineFilters` | The activity timeline's query and filter state. `timelineQueryParams` builds the request, `hasTimelineFilters` says whether anything is narrowed, and `dayStartIso` resolves a day boundary in the workspace zone. `enabled: false` holds the read until the record's own read allows it (the contact's Meetings tab reads `kind: "meeting"` this way) | `recordtimeline.ts` | — |
| `usePrefersReducedMotion` / `useTypeStream` / `useDocumentIntro` | Motion, with one rule: reduced motion jumps to the end state, never to nothing | `motion.ts` | — |
| `useDialogFocus` | What a dialog owes the keyboard: Escape closes from anywhere inside, Tab stays in, focus goes in on open and back to the opener on close. `Modal` and the ⌘K palette use it. `container` is the dialog's box, not its overlay. Only the topmost dialog owns Escape and Tab. `initialFocusTo` picks a field; `returnFocusTo` names a replacement opener. A popover, menu or sheet asks `coveredByDialog(itsTrigger)` before answering Escape | `dialogfocus.ts` | via `Dialog` and `Shell → Command palette` |
| `usePresence` | Keeps a closing surface mounted until its exit animation ends, by waiting on `getAnimations({ subtree: true })`, so it needs no duration. Returns `{ mounted, state }`; put `state` on the element as `data-state`. With nothing animating (reduced motion, jsdom) it unmounts at once | `presence.ts` | via `Dialog` and `Drawer` |
| `useUnsavedGuard` / `UnsavedGuard` | Unsaved edits, and leaving without saving them. Whatever holds a draft calls `useUnsavedGuard(dirty)`; `UnsavedGuard` wraps the surface and keeps it on screen while asking. A reload uses `beforeunload` (registered only while dirty); in-app navigation gets a `ConfirmModal`. Escape and the backdrop keep the edit. Scoped to the surface, so Back and Forward are not intercepted | `app/unsaved.tsx` | — |
| `useUrlParams` | The query half of the address, and the one way to read or change it. The path names what is on screen (`app/router.tsx`); this names which view, so `routeIdentity` ignores it. Writes replace the history entry, so Back from a record restores the list. Returns a `ReadonlyMap`; `currentParams()` reads it outside a render. The list mapping is `listQueryFromParams` / `paramsFromListQuery` in `screens/listquery.tsx`, bound in `useListQuery`. A screen's own drawing dial goes in `screenDials`; the deals board passes the same names to `withoutScreenDials` / `mergeScreenDials` | `app/urlstate.ts` | — |
| `subscribeToWindowFocus` and friends | Whether this window has focus: one signal for the draw loop and the stylesheet | `window-focus.ts` | — |
| `readStored` / `writeStored` / `forgetSeat` | Web Storage, reached one way. Every key is declared in `STORAGE_KEYS` with its lifetime: `device` (kept by this browser), `seat` (forgotten on sign-out), `account` (named by its account). A browser refusing storage reads as absent. `storage-spelling.test.ts` fails any other file naming `localStorage` or `sessionStorage` | `app/storage.ts` | — |

## Building a list screen

A record list is `useListQuery` bound to `ListTable`, and a screen's job is to
declare what its records are. `screens/listquery.tsx` holds the binding;
`screens/products.tsx` is the shortest complete example, and
`ListTable → Default` / `Paged` / `SortedByAMenu` in `listtable.stories.tsx`
show the surface on its own.

Each of the four dials asks the server rather than slicing what is already on
screen. The list is a keyset cursor over a set larger than what is loaded, so a
table that reordered its own page would misrepresent the other pages.

| Dial | Where it lives | The rule |
|---|---|---|
| Search | `q`, debounced by `SEARCH_DEBOUNCE_MS` | `searchable={false}` for a list whose GET has no `q`, and then no box is drawn |
| Sort | `sort`, one field plus the house tie-breaker | A column declares the server field it orders by; that makes its header live and puts it in the sort menu. A screen may not name an ordering its endpoint cannot give (`screens/list-sort-claim-coverage.test.ts`) |
| Filters | one wire param per chip | A chip's key is the parameter name, so the address and the endpoint use one vocabulary. `dataChips` for options the server names at runtime |
| Page | `page` + `per` in the address, `cursor` on the wire | The reader's place survives opening a record and pressing Back. A read fetches a whole multiple of the page size through `listFetchLimit`, so the pager can offer numbers without a round trip (`screens/list-page-size-coverage.test.ts`) |

What the binding gives you:

- Every dial is in the address, so a narrowed list is a link somebody can paste.
- Back from a record returns to the page and scroll position it was opened from.
- One `CountLine` phrasing for what is on screen out of what exists, which says
  "loaded so far" instead of inventing a total a keyset cursor cannot know.
- The reader goes back to page one when a narrowing changes what page one
  means, and not when they merely arrive.

At the call site:

- Do not draw a dial the endpoint does not answer. Omit it; do not disable
  it. `/partners` advertises a `sort` its handler never reads, and a screen
  that trusted it showed a "Newest" tab over rows in uuid order.
- **Two lists on one route need `paramScope`**, or they share one parameter
  space and send each other's fields to the wrong endpoint. A list that owns its
  address alone keeps bare names, so `#/companies?q=acme` stays a clean link.
- **Name a screen's own drawing dial in `screenDials`** (board or table, which
  pipeline's board). It belongs in the address and not on the wire; otherwise
  it is sent as a filter, counted as a narrowing, and wiped by "clear filters".

`emptyNote` is the sentence a screen adds when it knows why its list is empty.
It is drawn under the narrowed line as well as the bare one, because the usual
case is narrowed: a "Mine" tab for a reader who owns nothing. The caller decides
which emptiness a note explains. `mineEmptyNote` in `screens/recordlist.tsx` is
that sentence for the owner-scoped lists. A caller whose note would blame the
data source for what the reader's own dial did passes none.

## Absent, disabled, or withheld: the cause decides

A surface a reader cannot use is in one of three states, and the cause picks
which. An absent card and an empty card look the same on screen and mean
opposite things.

| Cause | State | What the reader gets |
|---|---|---|
| It does not apply here: a posture, a rollout flag, a capability this installation does not have | absent | Nothing. There is no fact to report. |
| A precondition the reader could fix is unmet: nothing selected yet, a write in flight, delivery not configured | disabled | The control, inert, and what would make it live. |
| A permission denies it | withheld | The surface keeps its place and says that it is withheld. |

The third row is the one that gets broken, because returning `null` on a denial
is the shortest code. It makes a false statement. A retention card that
vanishes for an ops seat reads as "this installation keeps nothing", and an
absent audit trail reads as "nothing has happened here". Both are claims about
the data, made in place of a claim about authority.

Four consequences:

- **A withheld card asks the server for nothing.** The answer is already known,
  so keep `enabled: canRead` on the query.
- **Gate on the probe's answer.** `/me` in flight is not a denial; branching
  before it answers flashes the notice at every reader.
- A write affordance inside a readable surface may be absent, provided the
  surface states its read-only posture once (`auto.readOnly` and
  `cf.noPermission` are the pattern). Withholding twelve buttons one by one is
  noise; leaving out the page's one explanation is the defect.
- A surface that is only an action may be absent on a denial. A card that
  holds no fact cannot be misread as "zero" or "nothing happened". An absent
  Reset-data card says nothing about the installation, while "you may not reset
  this installation" is noise on every page. A surface that reports anything at
  all does not qualify.

**`SurfaceState` is the primitive for the third row**, and for the other states
a surface can be in that are not content. Reach for it before hand-rolling a
message line: it already words withheld, unavailable and unsupported
differently, and lets only `empty` say there is none.

Other primitives that carry this:

- `Switch`'s `reason` renders the explanation, points the control at it with
  `aria-describedby`, and refuses the flip by itself. The caller cannot cancel
  it with `disabled={false}`.
- `Button`'s `reason` does the same for an action: it disables the button,
  renders the sentence beside it and wires `aria-describedby`. A `title` on a
  disabled button is announced by no screen reader, and a disabled button
  cannot be focused. `reasonId` points several refused controls at one sentence
  already on the page.
- `FieldGuard` covers a withheld value rather than a withheld surface.
- Otherwise hand the sentence to `EmptyState` (`<EmptyState>{t(…)}</EmptyState>`)
  where `SurfaceState` does not fit.

The sentence carries no wrapper and no type class of its own. The plate declares
both, and a `t-caption` or `t-sub` inside it overrides the primitive from the
call site. A sentence shown instead of content is card body text, and
`SurfaceState`, `EmptyState` and the `.empty` plate all draw it that way.

`Switch` versus `Checkbox` follows from the same rule: a `Checkbox` states an
intent that something later submits, and a `Switch` is the action. A control
that writes when flipped but announces itself as a checkbox misleads the reader
about what their next click does.

That pairing also answers a stateful control a permission denies, where the
control is the only place a reader can see the setting's value. Absent would
hide a granted read, and withholding the surface would hide the fact. A
`Switch` carrying `reason` shows the state, refuses the change, and says why,
with the explanation attached to the control.

## Seeing them

```sh
cd frontend && pnpm storybook      # the catalog on :6006, light and dark
```

The Theme control in the toolbar flips `data-theme` the same way the shell
does, so every token re-resolves. Check both themes before you call a surface
done. Stories live beside their component as `<name>.stories.tsx`; the
change-scoped capture gate (`frontend/scripts/fe-uat.mjs`) keys on that
co-location.

The sidebar's roots stand in the order `.storybook/preview.tsx` sorts them, and
a story's `title` is the only thing that files it under one. The capture gate
keys on `importPath`, never on the title, so a title may describe the surface
rather than the file:

| Root | What is under it |
|---|---|
| `Get started/` | The introduction: what the catalog is and how it is shelved. |
| `Foundations/` | The rules under every component, one node per topic: `Color`, `Typography`, `Radius`, `Brand`. |
| `Components/` | One node per component in this directory, under the category below that says what it is for. |
| `Patterns/` | Screen-tier building blocks that are not a page: the query gate, the create/edit/merge/share actions, the composer. |
| `Shell/` | The application frame and the Home page. |
| `Records/` | The pages a rep works in, and the cards on them. Each page a record opens is one node: `Contact 360/`, `Company 360/`, `Deal 360/`, `Project 360/`, `Leads/`, `Offers/` and `Deal room/`. `Record 360/` holds the pieces two or more of those pages mount. A list page, the table of every record of one kind (`Companies`, `Contacts`, `Deals/`, `Projects`, `Partners`), is a leaf or node of its own, except the leads list, which sits in `Leads/` beside the lead page. `Worklist/` and `Reports/` are nodes of their own. |
| `Settings/` | `<Group>/<Page>/<Card>`, mirroring the settings catalog: the groups of `SETTINGS_GROUPS` and the pages of `SETTINGS_PAGES`, under their own sidebar labels. `screens/settingsstories.test.ts` fails a story filed under a group or page the catalog does not declare. |
| `Onboarding/`, `Signed out/` | The first run, and the pages reachable without a session. |
| `MCP Apps/` | The governed tool surfaces and their document forms. |

`Components/` has one level of category, named as Atlassian's design system
names them, with AI and provenance as the one category of our own:

| Category | What is under it |
|---|---|
| Forms and input | Controls a reader fills, picks or presses: Button, Text input, Textarea, Search field, Field, Checkbox and radio, Segmented control, Select, Combobox, Value inputs, Switch, Choice list, Field grid, the inline edits, the pickers, the filter controls, Icon action |
| Images and icons | Marks that stand for a contact or a company, and the ground a place is drawn on: Avatar, Avatar stack, Company logo, Ambient waves |
| Labels | A word in a pill that takes no click: Badge, Tag pill, Row tags, Visibility, Role badge |
| Layout and structure | The boxes and columns a page is built from: Card, Panel, Section header, Disclosure, Page zones, Record view, Setting row, Action row |
| Loading | What a surface shows while it waits: Skeleton, Busy mark, Pending body |
| Messaging | What a surface says when something needs saying: Callout, Error line, Toast, Empty state, Surface state, Card boundary |
| Navigation | What takes a reader somewhere: Breadcrumb, Record tabs, Contact link, Offsite link |
| Overlays and layering | Surfaces drawn over the page: Modal, Confirm modal, Popover, Tooltip, Overflow menu, File preview, Resolve sheet |
| Status indicators | Where something stands: Communication status, Pipeline ladder, Stage strip |
| Text and data display | Records, figures and text to read: List table, Data table, Table scroll, Pipeline board, Timeline list, Readings, Stat card, Heading, Kbd, Markdown, the email family |
| Primitives | Stack and Row |
| AI and provenance | What an agent proposed and where a value came from: Decision card, Decision deck, AI pending, Evidence mark, Evidence receipt, Source evidence, Source email panel, Provenance tag, Trust, AI runtime chip, Margince core |

Under every root, every segment is Sentence case, and a declared acronym or
proper noun (`AI`, `Margince`, `MCP Apps`) keeps its spelling.
`sidebar.test.ts` holds one title per story file and no title that is both a
leaf and a group.

## Driving a control in a test

`Select` is a button and a portalled listbox, so `userEvent.selectOptions` does
not apply to it. Use the helper:

```ts
import { pickOption } from "../design-system/select-testing";

await pickOption(user, screen.getByRole("combobox", { name: "Stage" }), "Won");
```

## The gates that enforce this

The vitest suites run in `make fe-unit` and the shell scripts in
`make fe-ds-gates`; both are part of `make frontend-check`. The axe sweep runs
in `make frontend-e2e`. `native-controls.test.ts` is a TypeScript-AST vitest
gate, because a parser can tell a real element from one inside a comment or a
string.

| Gate | What it refuses |
|---|---|
| `make native-controls` (`src/design-system/native-controls.test.ts`) | `<select>` / `<option>` / `<optgroup>` anywhere under `src/` and in every extension frontend layer, with no exemption (`design-system/select.tsx` contains no native control) |
| `frontend/scripts/check-ds-purity.sh` | A hex literal or `rgb()`/`hsl()`/`oklch()` outside `tokens.css`. It also excludes tests, generated `schema.d.ts`, `provider-mark.tsx` (other companies' marks) and `tokens-testing.ts` (which parses the sheet) |
| `frontend/scripts/check-font-lock.sh` | A fourth type family, and mono outside code: the `t-mono` class anywhere, a `font-family`/`font` naming a mono family on a rule whose selectors do not all have `code`, `pre`, `samp` or `.code-block` as their subject, a custom property carrying one other than `--fontFamilyMono` in `tokens.css`, a mono family in a TS string. `check-font-lock.test.sh` plants each shape and each allowed one |
| `design-system/mono.test.ts` | The same mono rule read properly: comments skipped, selectors resolved through nesting and `@media`, `style` and `<style>` in HTML, class names and inline `fontFamily` read from the syntax tree of every TS/TSX file under `src/`, `e2e/`, `.storybook/` and every extension frontend layer |
| `design-system/weights.test.ts` | A weight no font file exists for. It reads the Google Fonts request in `index.html` against every `font-weight`, `font` shorthand and `--font*` token under `src/` plus each inline `fontWeight`. It refuses a text family that does not load 400/500/700, a declaration asking for another weight (named by file and line), and a mono family loading a weight the text families lack. A missing request, a family with no weights, or an empty corpus fails |
| `frontend/scripts/check-icon-glyph.sh` | An emoji glyph in a source string: Lucide only |
| `frontend/scripts/check-ds-spacing.sh` | New raw-px margin/padding/gap outside this tier (diff-scoped) |
| `frontend/scripts/check-ds-spacing-roles.sh` | Screen CSS that re-spaces a class this tier spaces and declares, re-sizes one it sizes (`font-size`, `line-height`, `letter-spacing`), or spells a rung where a role exists (`*-actions` gap, `*-cards` gap, `*-card`/`*-panel` padding). Whole-tree; the corpus is derived from this tier on every run |
| `design-system/conformance.test.ts` | Hard-coded user-facing copy and the same colour and font rules; a reduced-motion rule a later plain rule of equal specificity beats; an infinite animation with no reduced-motion answer; a second home for the pending announcement or the placeholder pulse; a focus ring that spells its own width and colour |
| `design-system/stylesheetnamespace.test.ts` | One stylesheet per class namespace: a screen's prefix (`auth-`, `book-`, `offers-` …) declared in any sheet but its home, across every `.css` under `src/` and each extension's frontend layer. Comments are stripped |
| `design-system/actionrow.test.ts` | A container of two or more sibling buttons without `gap: var(--gapActions)`, from a class it names or its own inline style. It reads markup and stylesheets together, so it catches a row naming a class nothing defines |
| `design-system/tokens.test.ts` | A token whose value drifted from the design canon |
| `design-system/type-tokens.test.ts` | The type half of the same sheet: the three weight tokens and the ten size shorthands declared unconditionally (not inside a `@media`), each weight and control token at its expected value, no second control height, and no `--fontBody*`/`--fontHeading*` spelling a weight by value instead of reading `--fontWeight*`. It checks the file `type-source.test.ts` excuses; both read `tokens.css` through `tokens-testing.ts` |
| `design-system/corners.test.ts` | A `border-radius` that names a length instead of a rung (waivable in line, with a reason); a `--r-full` or `50%` corner without `corner-shape: round`; a second `corner-shape: squircle` outside `tokens.css`; a rung that is not a multiple of four or has no doubled twin in the `@supports` block |
| `design-system/cardzones.test.ts` | A `Card` under `src/screens/` that carries a `title` or draws a `SectionHeader` as its own head band; that surface is a `Panel`. Read from the syntax tree. A card inside another surface is named in the test with its reason, and a stale entry fails |
| `design-system/inlinelayout.test.ts` | Layout written as a literal inline `style`: a margin, padding, gap, display, flex, grid, alignment, inset, size, position, overflow, order or transform property with a literal value, in every module under `src/` and every unit's frontend layer (all dialects). A computed value (a popover position, a bar's percentage) passes. It sees through ternaries, guards, defaults, `Object.assign`, casts, conditional spreads and hoisted `CSSProperties`, at the `style` attribute, a spread props object and the `createElement` / `jsx` call. Every file is held at zero; the fix is a class in the screen's sheet, or `Stack` / `Row` in a unit. A fixture suite plants each shape |
| `design-system/panelhead.test.tsx` | A description back in a panel or card head: a `sub` / `description` / `intro` prop declared by `Panel`, `PanelGroupHead`, `Card` or `SectionHeader`; a `.tsx` under `src/` passing one; a `className` naming `card-sub`; a stylesheet declaring `*-head-sub` / `*-title-sub` / `card-sub` or a `.sub` inside `.panel-head` / `.section-header`; and a head title at any size but `medium` (except the page-naming `level={1}`), read off `data-size`. Both walks fail closed under a floor |
| `design-system/tick-spelling.test.ts` | A tick drawn outside `Checkbox`/`Radio`: `role="checkbox"`, `"menuitemcheckbox"`, `"radio"` or `"menuitemradio"` on a JSX element that is not an `<input>`; `aria-checked` on anything but an `<input>` and `switch.tsx`; `<input type="checkbox">` / `type="radio"` outside `atoms.tsx`. Read from the syntax tree, each finding named `file:line`. Stories and tests are excluded (`interaction.stories.tsx` draws native controls to show `accent-color`). Fails closed under a file-count floor, with a probe suite |
| `design-system/badge-spelling.test.ts` | A pill drawn outside `Badge`: a class named `*-badge`, `*-pill`, `*-lozenge` or `*-tag` whose rules lay a fill on a full corner (`--r-full` or `50%`); a rule outside `atoms.css` naming `.badge` that sets anything but placement (margin, alignment, order, flex/grid placement, position, max-width, overflow); markup carrying the `badge` class outside the atom, or a `Badge` given `style` or `className`. A new look is a variant proposed in this directory |
| `design-system/heading-spelling.test.ts` | A raw `<h1>`–`<h6>` element, or a `createElement("h2", …)` / `el("h2", …)` that builds one, anywhere under `src/`, tests and stories included. `heading.tsx` and the view builder in `mcp-apps/bridge.ts` may name the element, and the gate holds their two size→element tables equal |
| `design-system/type-source.test.ts` | Type declared by value outside `tokens.css`, in every `.css` under `src/` and every non-test `.ts`/`.tsx` style object: `font-size`, `line-height`, `letter-spacing` and `text-transform` may only say `inherit`; `font` takes a `--font*` token and `font-weight` a `--fontWeight*` one. Inside the two UA resets (`app.css`, `mcp-apps/view.css`) it allows `text-transform: none` and `line-height: 0` on a rule selecting only `sub`/`sup`. It also refuses capitals drawn through `font-variant-caps`, the `font-variant` shorthand, or `"smcp"` / `"c2sc"` / `"pcap"` in `font-feature-settings` (`font-variant-numeric: tabular-nums` stays allowed), and a rendered `.toUpperCase()` / `.toLocaleUpperCase()` in TSX. Fails closed: missing `app.css`, under 100 stylesheets, or under 100 components |
| `design-system/controlheight.test.ts` | A pressable thing at a height other than `var(--controlHeight)`: a rule setting `height`, `min-height`, `block-size` or `min-block-size` on a class marked `cursor: pointer`, a class the markup puts on a `<button>`, or `button` itself. Both readings are floored. Every other height is in a register with its reason (a field is `--inputHeight`, a sign-in door owes 44px), and the register may only shrink |
| `design-system/menu-anatomy.test.ts` | A menu that spells its own anatomy. Every option surface (the ListTable menus, `Select`'s popup, the suggestion list, the overflow panel, the account menu and its flyout, the settings search list) reads one inset (`--controlGap`), one floor (`--menuMinInlineSize`) and one ceiling (`--menuMaxBlockSize`), or carries its reason in the roster. A second arm fails any element with a `menu`, `listbox`, `menuitem` or `option` role whose class the roster does not name |
| `design-system/catalog.test.ts` | A component in this directory that this table never names; a bare ✅ that no story backs (neither the module's own `<module>.stories.tsx` nor a `<component>.stories.tsx` named for a component in the row's first cell that imports the row's module); a story or docs file whose title cannot be read; a story under `src/` filed under a root the sidebar table does not document; catalog groups that are not the categories in their order |
| `design-system/sidebar.test.ts` | A documented root with no story; a `Components/` story outside a declared category, a `Foundations/` story outside a declared topic, or a category or topic with no story; under the shaped roots, a segment that is not Sentence case, two files sharing a title, or a title that is both a leaf and a group; a `storySort` in `.storybook/preview.tsx` whose roots, categories or topics are not these tables in their order |
| `e2e/` (axe) | WCAG 2.2 AA on every core screen, plus the 390px no-horizontal-scroll sweep |
