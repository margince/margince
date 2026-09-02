# DESIGN.md — the Margince visual language

This is the base every new surface is designed against. It states the look the
product is moving to, the tokens that carry it, the anatomy of each page type
and the rules of restraint that keep it from drifting back. It is a design
document; the mechanics of building a screen (which primitive, which file, which
gate) stay in [`frontend/src/design-system/README.md`](frontend/src/design-system/README.md),
and the two do not overlap: this file decides *how it should look*, the catalog
decides *what you build it from*.

Read this once before designing anything a person can see, and again when a
screen "looks fine but not like the others" — that sentence is the symptom this
file exists to cure.

## 1. Why the product looks stitched together today

Diagnosis first, because a new language that does not name the old failure
repeats it.

- **Everything is a box.** Panels inside panels, each with its own 1px border,
  its own radius and its own shadow, on a page that is nearly the same white as
  the panels. Nothing is allowed to be a *section*; every block has to be a
  *card*, so every block has the same visual rank and the eye has nowhere to land.
- **The chrome is the same colour as the content.** A white sidebar beside a
  white page beside white cards is three surfaces asking to be the foreground.
  The frame competes with what it frames.
- **One typeface at one size does every job.** Titles, labels, values and body
  differ by a few pixels and a weight step, so a record name and a field label
  read as the same kind of thing.
- **Numbers are set like words.** Money and counts sit in a proportional face
  with no alignment, so a column of amounts is a ragged block rather than a
  ledger.
- **Colour is spent on nothing.** The brand emerald is a button and a link; the
  rest of the page is grey. A product with a strong opinion about *who did this*
  (a person or an agent) shows that opinion at chip size only.

The result reads as a competent generic admin template. The fix is not a new
colour; it is a hierarchy.

## What the refined products actually do

The products people name when they say "that looks professional" — Linear,
Attio, Vercel, Stripe, Raycast, Notion — disagree on colour and agree on
almost everything else. The techniques below recur in every one of them, and
each is stated here as the rule this language adopts, so a screen can be
checked against it. The sources are listed at the end of the section.

1. **Depth comes from a surface ladder, not from shadows.** Linear and Raycast
   draw no drop shadow at all: a card is one step lighter than the page, a
   hovered row one step lighter again, and that ordering is the whole
   elevation system. Vercel is "border-first": a 1px hairline defines every
   static element and a real shadow is reserved for something floating above
   the plane. **Rule:** paper → surface → raised is the ladder; a hairline OR a
   shadow, never both on one element; only a popover, a menu or a drawer casts
   a shadow that a reader would notice.
2. **When a shadow is cast, it is tinted and layered.** Stripe's shadows are
   blue-grey (`rgba(50,50,93,.25)`) because its brand is navy, with a second
   tighter layer close to the element; a pure black shadow is what makes a
   card look pasted on. **Rule:** `--shadow-raised` and `--shadow-pop` are
   ink-green (`rgba(16,26,21,…)`) in two layers, the near one tight and the far
   one soft, from a single light source above.
3. **A rim light on the top edge is what makes a filled thing look made.**
   Raycast's buttons and keycaps carry `inset 0 1px 0 rgba(255,255,255,.1)`;
   the same one-pixel highlight is on every "premium" control the craft guides
   describe. **Rule:** the primary button, a raised card and a keycap take a
   1px inset white highlight on their top edge; nothing else does.
4. **Nested radii obey one law.** Mismatched corners on a control inside a
   card is the most common single reason an interface reads as "off":
   inner radius = outer radius − padding. **Rule:** a 14px card with 12px
   padding holds 8px controls; `--r-card` 14, `--r-control` 8, and a keycap 4.
5. **Text is near-black slate, never pure black; grays carry the brand hue.**
   Stripe sets text in deep slate on near-white; Refactoring UI's rule is that
   a gray far from mid-lightness needs saturation or it looks washed out.
   **Rule:** `--ink #101a15` and the whole ink and ground ladder sit on the
   emerald hue at low chroma, which is what stops the page from reading as a
   grey template with a green sticker.
6. **One chromatic accent, spent almost nowhere.** Linear's lavender is brand,
   the primary button and the focus ring, and nothing else; Raycast's primary
   action is plain white; Vercel's blue is a status colour and a focus ring.
   **Rule:** emerald is the chrome, the one primary verb in view, a link and
   the focus ring. The board's cards, the readings, the rows are colourless.
7. **Type: one family from display to body, weight capped at 600, tight
   negative tracking at size, tabular figures everywhere.** Linear's whole
   scale runs 600 → 400 with `-0.6px` on a 28px headline; Vercel sets large
   heads at `-0.04em`; every guide marks `font-variant-numeric: tabular-nums`
   mandatory for money and tables. Raycast turns on a stylistic set so its
   Inter stops looking like everyone's Inter. **Rule:** display at 600 with
   `-0.025em`, body at 400 and 500, 700 nowhere; every figure mono and
   tabular; the display face is the one place the type has a voice.
8. **Density is a feature.** Attio's whole product is a dense grid with
   high-contrast labels, subtle material and no loud brand elements; the 2026
   guidance is that every visible element earns its place and density and
   clarity coexist through strong opinions about the workflow. **Rule:** rows
   are 44–48px, a table shows eight columns before it scrolls, and the record
   page answers the rep's first five questions above the fold.
9. **The keyboard is visible.** Linear, Raycast and Attio surface a command
   field with its shortcut, and show the key beside every verb in a menu; a
   keycap is drawn as a small physical key (a one-step gradient, a 4px
   radius). **Rule:** `⌘K` on the command field, `/` on the ask field, and the
   shortcut on every menu row.
10. **Motion: transform and opacity only, three durations, one easing
    family; success states and figures move.** Stripe animates a number into
    its new value and marks success quietly; every guide holds hover at
    150–200ms and larger moves at ~300ms and forbids animating layout.
    **Rule:** the existing 90 / 140 / 200 / 360ms set, `--ease-out`
    everywhere, `--ease-spring` on release only; a reading that changes counts
    up over `--dur-move`; a saved row flashes `--accentWash` once.
11. **Spacing obeys proximity.** Space inside a group is at most half the
    space around it; the label sits 4–8px from its field, fields 12–16px
    apart, sections 32–48px apart. **Rule:** the `--space-*` ladder with the
    2:1 test applied to any group that looks "loose".
12. **Optical alignment beats geometric.** An icon beside a label, a play
    triangle, a chip's dot: centred by eye, nudged by a pixel. **Rule:** when
    a row looks off by a pixel, it is; nudge it, and say so in a comment.

Sources for this section: the Linear UI redesign write-up and the LogRocket
survey of "Linear design", the Raycast and Linear system extractions in the
awesome-design-md collection, the Vercel Geist breakdowns, the Stripe
dashboard breakdown, Refactoring UI as summarised by its readers, the
styleseed visual-craft rules and Emil Kowalski's design-engineering notes.

## 2. The language in five sentences

1. **A lit ground, glass on it.** The page is a pale green paper lit from two
   corners: an emerald glow behind the rail, an indigo glow behind wherever the
   agent is speaking. Every surface on it is a pane of frosted glass with a
   one-pixel rim on its top edge. Light is the default; dark is the same room
   with the lights down, not a second design.
2. **Panes, not boxes.** A pane is translucent, so it never fights the ground;
   a section is one pane, its rows divided by hairlines inside it. There is no
   card inside a pane.
3. **One display face carries identity.** The name of a company, a person or a
   deal is set large in the display face, and the verdict words the agent
   speaks ("You owe them", "Promise overdue") are set in it too. Nothing else
   is.
4. **Figures are set in the mono face, tabular.** Every amount, count, date and
   identifier aligns like a ledger and can be scanned down a column.
5. **Colour means something.** Emerald is the primary action, the brand and the
   light behind the rail. Indigo is a claim that an agent authored or proposed
   this, and the light behind it. Success, warning and danger are outcome.
   Nothing is coloured to look nice.

Rules 1 and 5 are already held by gates; this file adds 2, 3 and 4.

## 3. Colour

The semantic split is unchanged and remains pinned by `tokens.test.ts`:
`--accent` (emerald) is brand and primary action, `--ai*` (indigo) is agent
provenance, `--success` / `--warn` / `--danger` are status. What changes is the
GROUND, which is now lit, and the SURFACE, which is now glass.

### Light (the default)

| Token | Value | Role |
|---|---|---|
| `--ground` | `#f1f5f2` | The paper the page is read on. |
| `--glowA` | `rgba(24,190,120,.15)` | The emerald light, a 900×620 radial at the top-left corner, behind the rail. |
| `--glowB` | `rgba(91,97,214,.12)` | The indigo light, an 820×620 radial at the bottom-right corner, behind the context column where the agent speaks. |
| `--pane` | `rgba(255,255,255,.66)` + `backdrop-filter: blur(16px)` | Every section, reading and card. Translucent, so the light shows through. |
| `--paneEdge` / `--paneEdgeStrong` | `rgba(16,26,21,.08)` / `.14` | The one-pixel edge of a pane; the strong one for a table header or a form edge. |
| `--rim` | `rgba(255,255,255,.95)` | The one-pixel inset highlight on a pane's top edge. It is what makes glass read as glass. |
| `--railPane` | `rgba(255,255,255,.42)` + `blur(24px)` | The rail: glass over the emerald glow. |
| `--field` | `rgba(255,255,255,.7)` | An input, the command field, the ask field. |
| `--well` | `rgba(16,26,21,.045)` | A chip ground, the tab strip, a keycap. |
| `--hair` | `rgba(16,26,21,.07)` | The rule between rows inside a pane. |
| `--ink` / `--ink2` / `--ink3` | `#101a15` / `rgba(16,26,21,.64)` / `.44` | Titles and values / running text / labels and meta. |
| `--accent` / `--accentText` / `--accentWash` / `--accentGlow` | `#0b7a53` / `#0a6f4b` / `rgba(11,122,83,.10)` / `.28` | Fill, text on a tint, a selected row, the halo under the primary button. |
| `--primary` | `#0b7a53` | The one filled button. Flat, white ink, no rim, no halo. |
| `--ai` / `--aiText` / `--aiWash` / `--aiEdge` | `#5b61d6` / `#3f45b0` / `rgba(91,97,214,.09)` / `.35` | Agent provenance: the row's flat tint, its edge, the tile and the one filled agent verb. |
| `--success` / `--warn` / `--danger` | `#0a6f4b` / `#8a5a0a` / `#b42318` as text, each with a `.1`–`.16` wash and a `.25`–`.35` edge | Outcome, as a chip: text, wash and edge from one family. |

### Dark

The same room with the lights down. The glows brighten (`.28` / `.30`) because
they are the only light; the panes go to `rgba(255,255,255,.05)` with a
`.09` edge and a `.10` rim; the ink inverts to `#eef3ef`; the accent lifts to
`#2ee59d` and the primary button becomes a mint gradient with dark ink; the
agent text lifts to `#c3c6ff`; a `.05` grain overlays the ground so the glows
do not band. Every token above has a dark value, and the three-state theme
pattern (`:root`, `prefers-color-scheme` guarded by `:not([data-theme="light"])`,
`[data-theme="dark"]`) is how they switch.

### How colour is spent

- **The two glows are the chrome.** There is no coloured sidebar; the rail is
  glass, and the emerald behind it is what says which product this is. The
  indigo behind the context column says where the agent lives on the page.
- **The page carries at most ONE accent-filled control in view.** The primary
  verb of the page. Every other button is a glass pane.
- **Emerald never fills a surface.** A selected row and a "hot" reading take
  `--accentWash` with an emerald edge. A whole pane in emerald is a billboard.
- **Indigo is a fact, not a mood.** An agent row is a flat `--aiWash` with an
  `--aiEdge`; `1.5px dashed` while proposed, solid once it is real. No gradient
  and no halo: the tint must never make the words harder to read. Text on it
  is `--aiText` for labels and `--ink` for the sentence itself.
- **Status is a dot before it is a chip.** The dot-and-label form is the
  default; the outlined chip is for the one status that must not be missed.
- **Monogram tones identify, never rank.** The six `--monoN` pairs stay.

## 4. Type

Three families, which is the ceiling `check-font-lock.sh` holds.

| Role | Family | Why |
|---|---|---|
| Display | **Bricolage Grotesque** | The one face with a personality. Its optical sizing tightens at large sizes, so a record name at 34px has the character a generic grotesque cannot give and the same family stays calm at 20px for a section title. |
| UI and body | **Geist** | Neutral, narrow enough for dense rows, with a real medium weight and good hinting at 13px. It disappears, which is its job. |
| Figures and identifiers | **Geist Mono** | Amounts, counts, dates, VAT ids, passport ids. Drawn from the same family as the body so a figure sits on the same line as its label without a visible font change. |

### The scale

Nine steps, and the page uses at most five of them.

| Token | Size | Face | Where |
|---|---|---|---|
| `--fs-identity` | `clamp(28px, 2.4vw, 36px)` | Display, 600, tracking −0.025em | The name at the top of a record. Once per page. |
| `--fs-h1` | 24px | Display, 600, tracking −0.02em | The title of a list page or a settings page. |
| `--fs-h2` | 18px | Display, 600 | A section title. |
| `--fs-h3` | 15px | Body, 600 | A card title, a row's leading text. |
| `--fs-body` | 14px | Body, 400, line-height 1.5 | Reading text. |
| `--fs-ui` | 13px | Body, 500 | Controls, table cells, tabs, chips. |
| `--fs-meta` | 12px | Body, 400 | Timestamps, provenance, helper text. |
| `--fs-eyebrow` | 11px | Body, 600, uppercase, tracking 0.08em | Section labels above a group. |
| `--fs-figure` | inherits | Mono, `tabular-nums` | Any number a reader compares to another. |

- Headings take `text-wrap: balance`. Running text sits at 60–70 characters.
- **A figure is always mono and tabular.** `€184,500`, `3 open`, `Thu 4 Sep`,
  `DE 812 345 678`. A proportional number in a column is a defect.
- **Uppercase is for eyebrows only.** Not buttons, not tabs, not table headers.
- Weight 700 is reserved for the one word that must outrank a heading. It
  appears on almost no screen.

## 5. Space, shape, depth

- **4px base**, the existing `--space-*` ladder. Panes sit 14px apart; rows
  inside a pane are 40–48px, divided by `--hair`; the page gutter is 32px.
- **Radii by role**: panes 16px, the agent card 18px, buttons and fields 10px,
  chips full, keycaps 5px. Inner radius = outer radius − padding.
- **Depth is light, not shadow.** A pane is translucent over a lit ground with
  a rim on its top edge; that is its whole elevation. Nothing at rest casts a
  shadow and nothing glows: the two corner glows on the ground are the only
  light on the page. Buttons are flat fills or flat outlines. A popover, menu
  or drawer takes `--shadowPop` because it is genuinely above the page; a
  board card takes it on hover, because it is being picked up.
- **No pane inside a pane.** A section is one pane; the things in it are rows.

## 6. The shell

```
┌──────────┬──────────────────────────────────────────────────────────┐
│ rail     │ crumb · search / command  ⌘K · me                         │
│ (glass   │                                                          │
│  over    │   identity: mark · NAME (display) · badges               │
│  emerald │             facts line · quiet line · three standing facts│
│  glow)   │             verbs, right                                 │
│          │   tabs (a glass strip)                                   │
│ Home     │   ┌ work column ──────────────┐ ┌ context column ───────┐ │
│ RECORDS  │   │ readings  ·  ·  ·  ·       │ │ CONTEXT        Hide › │ │
│ Contacts │   │ moment (indigo) · the call │ │ Details               │ │
│ Companies│   │ record spine               │ │ Active deals          │ │
│ …        │   │ what needs a person today  │ │ Their key people      │ │
│ ● agent  │   │ …                          │ │ Tags        (indigo   │ │
│  (indigo)│   └────────────────────────────┘ └──────────────  glow) ─┘ │
└──────────┴──────────────────────────────────────────────────────────┘
```

- **Rail**: 232px, `--railPane` over the emerald glow, `--ink2` labels, the
  active row a pane with a rim. The agent's status is a small indigo pane at
  the foot with the orb breathing (`--dur` 4s, reduced-motion still).
- **Top row**: not a bar. The breadcrumb, the command field (`⌘K`) and the
  reader's monogram sit on the ground itself; the content scrolls under
  nothing.
- **Context column**: 300px, headed `Context` with `Hide`, its panes over the
  indigo glow. It is the window's aside, not the record's, so it does not move
  when a tab changes. Below 1100px it is one region at a time.
- **Directions set aside.** Emerald chrome (a saturated sidebar), deep ink
  chrome, an editorial paper, a monochrome command surface, a bento of tinted
  tiles and a tactile soft-shadow set were built and compared on the same
  page. Aurora won on the one thing the product needs to say at a glance: an
  agent works this account with you, and you can see where it is.

## 7. How the tool feels, and how a page is structured

**The feel.** Calm, lit, and legible at arm's length. A rep opens an account
and in one screen knows where it stands, what they owe, and what happened; the
agent speaks in one place on the page and says what it rests on; every figure
is in the same face and lines up; nothing casts a shadow or glows. It should
feel like a well-lit desk with one folder open on it, not a dashboard.

**Five rules of structure**, learned by inventorying what the two record pages
render today and finding deals in four places, people in three, and two verdict
cards stacked above a third list of things to do:

1. **Every fact has one home.** If a deal appears in the readings, it does not
   also appear in the rail. If people are in the context column, they are not
   also a section in the work column.
2. **Zones, numbered, in reading order.** A record page is six zones with a
   heading each. The number is not decoration: it is the order a rep reads an
   account in, and it lets a colleague say "look at 3".
3. **One list of what needs you.** The agent's read of the record is the lead
   row of that list, not a separate card above it; the agent's finds, the manual
   moves, the overdue tasks and the next meeting are rows below it, sorted by
   urgency. A person has one place to look and one place to clear.
4. **One timeline.** The spine (the axis with the gap drawn at width) heads the
   list of what happened; they are one component, not two panes.
5. **The context column answers "who and what is this", not "what is
   happening".** Ask at the top, then the people, the fields, the tags. State
   and work stay in the work column, where the reading order is.

### The company record, in six zones

1. **Who.** Mark, name, lifecycle and relationship badges, one line of facts
   (site · industry · size · owner · way in), one quiet line (last contact ·
   created · captured by), and the verbs: Write email (filled), Log activity,
   Add task, more.
2. **Where this account stands.** Four readings. Health carries the call: the
   standing word, whose move it is, and the three rated dimensions as dots.
   Open pipeline, Invoiced, Conversation, each a door into its tab. "Not shown"
   and "Not assessed" stay distinct.
3. **What needs you.** The moment as the lead row (indigo ground, the verdict
   word in the display face, what it rests on, the agent's action in the agent's
   fill). Then the agent's finds, the overdue and due tasks, the next meeting.
   The foot names what was hidden from the reader.
4. **What happened.** The spine, then the recent rows. "N new since your last
   visit" in the zone head.
5. **Commercial.** Contract state, won and lost on one line; then each open
   deal with its one status clause; then the project it belongs to.
6. **About.** The dossier lead and paragraph, its provenance and age, the
   signals, the fit verdict with a link to how it was judged. Last, because it
   changes slowest.

Context column: Ask (with three prepared questions), People (three, with who is
in touch from our side), Details (nine fields including mail capture), Tags.

**What merged, moved or went.** The call card merged into Health. Next steps,
the agent's suggestions, the manual moves and the tasks merged into one list.
The record spine merged into the timeline. Work in flight, the commercial
figures and projects merged into Commercial. The dossier, signals and growth
fit merged into About. Active deals left the rail; key people became the rail's
People; Ask moved from the foot of the page to the top of the column. The
Details grid lost its empty duplicate rows and gained mail capture. Nothing the
page could say is gone; each thing is said once.

### The contact record, in six zones

1. **Who.** Round monogram, name, the buying-role badge, title and employer,
   the glyph line (email, phone, city, LinkedIn, owner), and the verbs: Write
   email (the transport, filled), Call, Add task, more.
2. **Where you stand with her.** One strip of seven: Overall (the pulse
   verdict, in its colour only when not withheld), Last inbound, Last outbound,
   Reciprocity as counts, Coverage, Next meeting, Consent.
3. **What needs you.** The moment as the lead row with its actions and their
   readiness; then the commitments and open loops as rows with a read-only
   tick.
4. **What was said.** Conversation memory with its All / Email / Meetings /
   Calls / Notes cut, replied or unanswered, a reply verb per row.
5. **The deal she decides.** Title, amount, stage, close, owner, the buying
   committee.
6. **Understanding her.** The relationship brief, then Priorities, Objections
   and Success as three rows, then provenance and "Correct something".

Context column: Ask, the waiting email, Details (title, company, before,
reports to, languages, city, the withheld mobile), Who knows her, Consent &
channels.

**What merged, moved or went.** The rail's relationship pulse merged into the
strip. Commitments merged into the needs list. The rail's recent activity merged
into what was said. The brief and what matters merged into Understanding. What
Margince read moved to Data & tools, its home. Consent became a compact rail
pane with the purposes; the full grant/withdraw table lives behind Manage.

### The list page, home and the board

The list page: `--fs-h1` title with the count in mono, saved-view tabs as a
glass strip, one search field and the query dials on one line, the table in
one pane with eyebrow headers, hairline rows, mono figures right-aligned and
the selected row in `--accentWash` with a 2px emerald mark. Home: a display
greeting with the date in mono; the week's readings as panes with a sparkline
each; Decide (the agent's cards), Today (meetings), Follow up (who wrote last).
The board: columns on the ground with the stage as an eyebrow and the sum in
mono; glass cards that lift on hover; a card the agent moved dashed indigo
until a person confirms it.

## 8. Components, restated in this language

The primitives in the catalog keep their names and props; this is how each one
looks now.

| Primitive | Treatment |
|---|---|
| `Button` primary | `--primary` flat fill, white text, 9px radius, 36px. One per view. |
| `Button` secondary | `--paneSolid` fill, `--paneEdgeStrong` outline, `--ink` text, flat. Icon-only at 36×36 for the overflow. |
| `Button` ghost | No fill, no edge, `--ink2` text; hover `--well`. |
| `Button` danger | Outlined in `--danger`; fills only inside a `ConfirmModal`. |
| `TextInput` / `Select` | `--bgSurface`, `--hairlineStrong` 1px edge, 36px, 8px radius; focus is a 2px emerald ring with a 3px wash. Label above in `--fs-ui` weight 500; helper below in `--fs-meta`. |
| `Badge` | `quiet` by default: a 6px dot and a label. Filled pill only for the one status that must not be missed. |
| `Chip` | `--bgInset` ground, `--fs-ui`, no border. |
| `Panel` | Becomes a **pane**: `--pane` glass, a `--paneEdge`, a `--rim`, 16px radius; a title row, then rows divided by `--hair`. `PanelPlate` (the inset well) takes `--well`. |
| `StatCard` | A **reading** pane: eyebrow, mono figure (or a display-face word), basis line, a meter when it rates, the dotted basis link. `hot` takes `--accentWash` with an emerald edge; `warn` takes the warn edge. |
| `ListTable` / `DataTable` | Headers as eyebrows on paper; rows on surface; hairlines; mono figures right-aligned. |
| `RecordTabs` | A glass strip (`--well`) with the current tab as a raised pane; counts in mono. |
| `Modal` | `--paneSolid` (glass does not work over a scrim), 16px radius, `--shadowPop`, a scrim of `rgba(15,29,24,.5)`. The drawer form slides from the right with the same surface. |
| `EmptyState` | Centred, `--inkMuted`, one sentence and one verb. The only centred thing on a page. |
| `Callout` | A hairline-left rule in the tone's colour on a `--bgInset` ground. Never a filled coloured box. |
| `StagingCard` / `DecisionCard` | The agent row: flat `--aiWash`, `--aiEdge` (dashed while proposed), the indigo tile and the eyebrow "Margince read this record", the verdict word in the display face. It is the lead row of the needs list, never a card floating above it. |
| `Kbd` | A small physical key: `--bgInset` with a one-step gradient to `--bgSurface`, a 1px inset white rim, mono 11px, 4px radius. Appears on the command field, the ask field and beside every verb in a menu. |

## 9. Motion

The existing durations and curves stay (`--dur-tap` 90ms, `--dur-state` 140ms,
`--dur-move` 200ms, `--dur-enter` 360ms with a 40ms stagger, `--ease-out`,
`--ease-spring`). What this language adds is where motion is *spent*:

- A page's sections enter with the stagger, from a visible resting state (an
  8px rise and a fade), never from `opacity: 0`.
- A row highlights on hover in `--dur-state`; nothing else on a row moves.
- The agent's card, when it arrives, draws its dashed edge in over
  `--dur-enter`. That is the one flourish, and it is a fact (a proposal just
  landed) rather than decoration.
- A reading whose value changed counts to the new figure over `--dur-move`;
  a row just saved flashes `--accentWash` once and fades over `--dur-enter`.
- Only `transform` and `opacity` animate. Nothing animates its own layout.
- `prefers-reduced-motion` jumps every one of these to its end state.

## 10. Restraint — what a screen in this language does not do

- Does not put a pane inside a pane. A section is one pane; the things in it
  are rows.
- Does not cast a shadow, a glow or a gradient at rest. Depth is the light
  behind the glass, and a button is a flat shape.
- Does not say a fact twice. One home per fact; the rest are links to it.
- Does not fill a button, a row or a surface with emerald except the one
  primary verb and a selected row's wash.
- Does not tint anything indigo that a person wrote.
- Does not set a number in the proportional face.
- Does not centre a title, a section or a table.
- Does not use uppercase outside an eyebrow, or weight 700 outside the rare
  word that must outrank a heading.
- Does not add a fourth typeface, an emoji glyph, a gradient hero, or a shadow
  under a shadow.
- Does not invent a component that the catalog already names. Grep first.

## 11. Adopting it

The tokens above land in `frontend/src/design-system/tokens.css`, and
`tokens.test.ts` pins values, so a repaint is a change to both in one commit —
the test is what proves the canon moved rather than one screen. The order of
work that gets the most visible change for the least churn:

1. **The lit ground and the glass rail.** The two glows on the shell's
   ground, the rail as `--railPane` with blur, the top strip dissolved into the
   ground. One file each (`shell.css`, `topbar.css`), and the whole product
   stops looking like a template.
2. **Type.** Swap the three families and the scale. `check-font-lock.sh` holds
   the count at three.
3. **Panes.** Restyle `Panel` from a bordered white card to a glass pane with
   a rim. Every record page and settings page changes with it.
4. **Figures.** `font-variant-numeric: tabular-nums` and the mono face on every
   figure through `StatCard`, `ListTable` cells and `FieldGrid` values.
5. **Grounds.** Move the paper and surface values; re-check every `color-mix()`
   derivation in both themes with Storybook's theme control.

Each step is one PR and a Storybook pass in both themes. Verify contrast on
the new grounds with the existing axe suite before the values are pinned.

## 12. Checklist for a new screen

- [ ] One identity element in the display face; nothing else uses it.
- [ ] Every figure is mono and tabular, right-aligned in a column.
- [ ] Every section is one pane; rows inside it are divided by hairlines; nothing casts a shadow at rest.
- [ ] One emerald-filled control in view.
- [ ] Anything an agent wrote sits on `--aiWash` with the indigo tile and the
      "Margince read this record" eyebrow; dashed while proposed.
- [ ] A withheld section says "Hidden from you"; an empty one says what to do.
- [ ] Checked in both themes; contrast holds on the glass over both glows.
- [ ] The page carries every section the current screen renders (§7), or says
      which it dropped and why.
- [ ] Every control came from `frontend/src/design-system/`; the catalog table
      names anything new.
