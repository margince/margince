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

1. **Emerald chrome, paper page.** The rail and the top strip are the brand
   emerald in BOTH themes; the page is a quiet, light paper ground. The frame
   is the one saturated thing on screen, so the content never has to compete
   with it, and it says which product this is from across the room. Surfaces
   stay light: dark mode is a reader's choice, never the default look.
2. **Sections, not cards.** A page is a column of sections divided by type and
   hairlines. A card is spent only on the thing that is genuinely a separate
   object: a proposal, a deal on a board, a person in a list.
3. **One display face carries identity.** The name of a company, a person or a
   deal is set large in the display face, and nothing else on the page is. It is
   the one place the type has a personality.
4. **Figures are set in the mono face, tabular.** Every amount, count, date and
   identifier aligns like a ledger and can be scanned down a column.
5. **Colour means something.** Emerald is the primary action and brand. Indigo
   is a claim that an agent authored or proposed this. Success, warning and
   danger are outcome. Nothing is coloured to look nice.

Rules 1 and 5 are already held by gates; this file adds 2, 3 and 4.

## 3. Colour

The semantic split is unchanged and remains pinned by `tokens.test.ts`:
`--accent` (emerald) is brand and primary action, `--ai*` (indigo) is agent
provenance, `--success` / `--warn` / `--danger` are status. What changes is the
GROUND and the CHROME, which is where the "bootstrap" feel came from.

### Light

| Token | Value | Role |
|---|---|---|
| `--bgChrome` | `#0d6f4c` | The rail. Brand emerald, one step deeper than `--accent` so a primary button on the page still outranks it. |
| `--chromeTopbar` | `#0b6343` | The top strip: one step deeper again, so the two chrome pieces read as one material with a fold. |
| `--bgChromeRaised` | `rgba(255,255,255,.14)` | A hovered or active rail row: a white lift on the emerald. |
| `--chromeInk` / `--chromeInkStrong` | `rgba(255,255,255,.74)` / `#ffffff` | Text on the chrome. Labels at the alpha, the active row and the workspace name solid. |
| `--chromeBar` | `#a8f0cf` | The 2px mark on the active rail row: mint, because emerald on emerald is invisible. |
| `--bgPage` | `#f2f5f3` | The paper the page is read on. Warm-green paper, not white. |
| `--bgSurface` | `#ffffff` | A section body, a list row, a form. The only pure white. |
| `--bgRaised` | `#ffffff` + `--shadow-raised` | A card that is a separate object (a proposal, a board card, a menu). |
| `--bgInset` | `#e9eeeb` | A well: a chip ground, a code block, an inset row. |
| `--hairline` | `#e1e7e3` | The rule between sections and rows. The default divider; never a box. |
| `--hairlineStrong` | `#cbd4ce` | A rule that must survive a busy background: a table header, a form edge. |
| `--ink` | `#101a15` | Titles and values. |
| `--inkBody` | `#33403a` | Running text. |
| `--inkMuted` | `#66736c` | Labels, meta, timestamps. AA on every ground above. |
| `--inkFaint` | `#9aa59f` | Placeholders and disabled. |
| `--accent` | `#0b7a53` | Primary action, brand, link. |
| `--accentInk` | `#0a6f4b` | The accent as TEXT on a tinted ground. |
| `--accentWash` | `rgba(11,122,83,.08)` | The ground under an accent chip or a selected row. |
| `--ai` / `--aiText` / `--aiWash` | `#5b61d6` / `#3f45b0` / `rgba(91,97,214,.08)` | Agent provenance. Unchanged. |
| `--success` / `--warn` / `--danger` | `#15803d` / `#92400e` / `#b91c1c` | Outcome. Unchanged. |

### Dark

Dark is not light inverted. Surfaces lift toward the light, the chrome stays
where it is, and the accent brightens one step so it survives on ink.

| Token | Value |
|---|---|
| `--bgChrome` / `--chromeTopbar` | `#0a5a3d` / `#094f36` (the emerald darkened one step, still unmistakably green) |
| `--bgChromeRaised` | `rgba(255,255,255,.14)` |
| `--bgPage` | `#0e1814` |
| `--bgSurface` | `#131f1a` |
| `--bgRaised` | `#192822` |
| `--bgInset` | `#0b1511` |
| `--hairline` | `rgba(255,255,255,.07)` |
| `--hairlineStrong` | `rgba(255,255,255,.14)` |
| `--ink` / `--inkBody` / `--inkMuted` / `--inkFaint` | `#eaf0ec` / `#c2cbc6` / `#8b978f` / `#5c6862` |
| `--accent` | `#2bb673` |
| `--accentInk` | `#5fd39b` |
| `--aiText` | `#9ba0f0` |

### How colour is spent

- **The chrome is the emerald's one large use.** Because the rail and strip
  carry it at full strength, the page itself spends it sparingly: that is what
  keeps the primary button readable as the verb rather than as more brand.
- **The page carries at most ONE accent-filled control in view.** The primary
  verb of the page. Every other button is outlined or plain.
- **Emerald never fills a surface.** A selected row takes `--accentWash`, a
  count chip takes `--accentWash` with `--accentInk` text. A whole card in
  emerald is a billboard.
- **Indigo is a fact, not a mood.** `--aiWash` under a card means an agent
  wrote what is in it. `1.5px dashed` on its edge means it is proposed and not
  yet accepted; the dashes going solid is acceptance. Text on `--aiWash` is
  `--aiText`, never `--ai`.
- **Status is a dot before it is a fill.** `Badge quiet` (a dot and a label) is
  the default; the filled pill is for the one status the reader must not miss.
- **Monogram tones identify, never rank.** The six `--monoN` pairs stay as they
  are. A record is the same colour everywhere and the colour says nothing
  about it.

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

- **4px base**, the existing `--space-*` ladder: 4 / 8 / 12 / 16 / 20 / 24 / 32 /
  40 / 48 / 64. Sections breathe at 32; rows sit at 12; inline gaps at 8.
- **Page gutter 40px** on the reading ground, 24px inside a section body.
- **Radii by role**: `--r-control` 8px for buttons, inputs and chips;
  `--r-card` 14px for the rare separate object; `--r-pill` full for badges.
  A section has NO radius because it has no box.
- **Depth by role**, and mostly none. A section is a hairline above and below.
  A raised card is one step up the surface ladder with a 1px `--hairline` and a
  1px inset white rim on its top edge; the tinted `--shadow-raised: 0 1px 2px
  rgba(16,26,21,.06), 0 8px 24px rgba(16,26,21,.06)` is there for the hover
  lift, not for rest. A popover, a menu and a drawer take `--shadow-pop`. The
  page never stacks shadows, and no element carries a hairline and a shadow at
  once: a card inside a card is the thing this document bans.
- **Nested radii**: inner = outer − padding. A 14px card with 12px padding
  holds 8px controls; a keycap inside a control is 4px.
- **The chrome has a fold, not a border.** The rail and the strip are two
  steps of one emerald with no line between them; the active row is a white
  lift, not a box. That is what makes the chrome read as a material rather
  than a coloured template.

## 6. The shell

```
┌──────────┬──────────────────────────────────────────────────────────┐
│ rail     │ top strip: crumb · search / command  ⌘K · me                │
│ (emerald)├──────────────────────────────────────────────────────────┤
│          │                                                          │
│ Home     │   page (paper)                                           │
│ RECORDS  │                                                          │
│ Contacts │   ┌ identity ───────────────────────────────────────┐    │
│ Companies│   │ monogram · NAME (display) · facts · verbs        │    │
│ Leads    │   └──────────────────────────────────────────────────┘    │
│ WORK     │   readings ribbon: fig │ fig │ fig │ fig                  │
│ Pipeline │   tabs ─────────────────────────────────────────────     │
│ Tasks    │   ┌ work column (7) ─────────┐ ┌ context column (5) ─┐   │
│ Approvals│   │ section                  │ │ agent card (indigo) │   │
│ INTELL.  │   │ ───────────────────      │ │ connections         │   │
│ Reports  │   │ section                  │ │ ask                 │   │
│          │   └──────────────────────────┘ └─────────────────────┘   │
│ ● agent  │                                                          │
└──────────┴──────────────────────────────────────────────────────────┘
```

- **Rail**: 232px, `--bgChrome`, white-alpha icons and labels, groups under
  eyebrows (Records / Work / Intelligence). The active row is
  `--bgChromeRaised` with a 2px `--chromeBar` mark on its left edge. The
  workspace mark is white with the emerald M cut out of it. The agent's status
  lives at the rail's foot: the orb, its state, today's spend.
- **Top strip**: 52px, `--chromeTopbar`, the breadcrumb on the left, the
  command field in the middle (`⌘K` in a `Kbd`), the reader's monogram on the
  right. It is chrome, so it is emerald; the content scrolls under it.
- **Two alternatives were built and set aside.** A deep ink-green chrome
  (`#0f1d18`) reads as black and turns the frame into a dark slab beside a light
  page; a light paper chrome (`#e4ede8`) is calm but is the template look this
  language exists to leave. Both stay in the demo as switchable variants so the
  comparison can be made on screen rather than argued.
- **Page**: paper ground, 40px gutter, max reading width 1240px, left-aligned.
  Nothing on a page is centred except an empty state.

## 7. Page anatomy

### The record page (company, contact, deal)

1. **Identity block.** Monogram or logo at 56px, the name in `--fs-identity`,
   under it one line of facts in `--fs-ui` separated by middots (city · sector ·
   size · site), then the verbs on the right: one filled primary, the rest
   outlined, overflow in a menu. No box.
2. **Readings ribbon.** Four to six figures in a single row, each a label in
   `--fs-eyebrow` over a value in mono at 20px, separated by vertical hairlines,
   NOT by tiles. Each figure carries its basis under it in `--fs-meta` ("3 open ·
   1 stalled"). A withheld figure says "Hidden from you" in `--inkMuted`, never a
   blank. This is the account's state strip; it is where the page tells the truth.
3. **Tabs.** A rule with the current tab underlined in emerald. Counts beside
   the labels in mono.
4. **Body: 7 / 5 columns.** The work column is a stack of sections; the context
   column holds the agent's cards and the relationship map. On a narrow
   viewport the context column drops below.
5. **A section** is: eyebrow-or-h2 title on the left, its one verb on the right,
   a hairline, then rows. Rows are 44px, divided by hairlines, with the leading
   text in `--fs-h3` weight 500 and the figure right-aligned in mono.
6. **The agent's card** is the one card in the column: `--aiWash` ground,
   `--ai` 1px edge (dashed while proposed), a `.provenance-agent` chip in the
   header saying "Suggested", the reasoning in body text, and the Accept / Edit /
   Dismiss triad. A person can tell at arm's length which block on the page a
   machine wrote.

### The list page (companies, contacts, deals)

`--fs-h1` title with the count in mono beside it, the saved-view tabs, one
search field and the query dials on one line, then the table. Table headers in
`--fs-eyebrow` on the paper ground; rows on `--bgSurface` divided by hairlines;
figures right-aligned mono; the row's monogram leads. The count line under the
table says "23 of 1,204 · by last touch".

### Home

A greeting in the display face with the date in mono ("Thursday, 4 Sep"). Then
three regions: **Decide** (the approval cards the reader owes, each an agent
card), **Today** (meetings and follow-ups as rows), **Readings** (the week's
figures as a ribbon with a sparkline each). No dashboard tiles, no gauges.

### The board (pipeline)

Columns on the paper ground with the stage name as an eyebrow and the column's
sum in mono. Cards are the ONE place `--bgRaised` is used at volume: company
name in `--fs-h3`, amount in mono, the owner's monogram, days in stage as meta.
A staged card an agent moved is dashed indigo until a person confirms it.

## 8. Components, restated in this language

The primitives in the catalog keep their names and props; this is how each one
looks now.

| Primitive | Treatment |
|---|---|
| `Button` primary | `--accent` fill, white text, 8px radius, 36px, weight 500. One per view. |
| `Button` secondary | `--bgSurface` fill, `--hairlineStrong` edge, `--ink` text. |
| `Button` ghost | No fill, no edge, `--inkBody` text; hover `--bgInset`. |
| `Button` danger | Outlined in `--danger`; fills only inside a `ConfirmModal`. |
| `TextInput` / `Select` | `--bgSurface`, `--hairlineStrong` 1px edge, 36px, 8px radius; focus is a 2px emerald ring with a 3px wash. Label above in `--fs-ui` weight 500; helper below in `--fs-meta`. |
| `Badge` | `quiet` by default: a 6px dot and a label. Filled pill only for the one status that must not be missed. |
| `Chip` | `--bgInset` ground, `--fs-ui`, no border. |
| `Panel` | Becomes a **section**: no border, no radius, no shadow; a title row, a hairline, rows. `PanelPlate` (the inset well) keeps `--bgInset`. |
| `StatCard` | Becomes a **reading** in a ribbon: eyebrow, mono figure, basis line, hairline between neighbours. |
| `ListTable` / `DataTable` | Headers as eyebrows on paper; rows on surface; hairlines; mono figures right-aligned. |
| `RecordTabs` | Underline tabs on a rule; counts in mono. |
| `Modal` | `--bgRaised`, `--r-card`, `--shadow-pop`, a scrim of `rgba(15,29,24,.5)`. The drawer form slides from the right with the same surface. |
| `EmptyState` | Centred, `--inkMuted`, one sentence and one verb. The only centred thing on a page. |
| `Callout` | A hairline-left rule in the tone's colour on a `--bgInset` ground. Never a filled coloured box. |
| `StagingCard` / `DecisionCard` | The agent card of §7. |
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

- Does not put a card inside a card. A section is a hairline; a card is a
  separate object; there is no third container.
- Does not draw a border where a hairline and space would do.
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

1. **Chrome.** `--bgChrome` and `--chromeTopbar` for the rail and top strip in
   both themes, the mint active mark, the agent foot. One file each
   (`shell.css`, `topbar.css`), and the whole product stops looking like a
   template.
2. **Type.** Swap the three families and the scale. `check-font-lock.sh` holds
   the count at three.
3. **Sections.** Restyle `Panel` from a bordered card to a hairlined section.
   Every record page and settings page changes with it.
4. **Figures.** `font-variant-numeric: tabular-nums` and the mono face on every
   figure through `StatCard`, `ListTable` cells and `FieldGrid` values.
5. **Grounds.** Move the paper and surface values; re-check every `color-mix()`
   derivation in both themes with Storybook's theme control.

Each step is one PR and a Storybook pass in both themes. Verify contrast on
the new grounds with the existing axe suite before the values are pinned.

## 12. Checklist for a new screen

- [ ] One identity element in the display face; nothing else uses it.
- [ ] Every figure is mono and tabular, right-aligned in a column.
- [ ] Sections are divided by hairlines; the only cards are separate objects.
- [ ] One emerald-filled control in view.
- [ ] Anything an agent wrote sits on `--aiWash` with a provenance chip; dashed
      while proposed.
- [ ] A withheld section says "Hidden from you"; an empty one says what to do.
- [ ] Checked in both themes; contrast holds on the new grounds.
- [ ] Every control came from `frontend/src/design-system/`; the catalog table
      names anything new.
