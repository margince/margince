# DESIGN.md — the Margince visual language

This is the base every new surface is designed against. It states the look the
product is moving to, the tokens that carry it, the anatomy of each page type
and the rules of restraint that keep it from drifting back. It is a design
document; the mechanics of building a screen (which primitive, which file, which
gate) stay in [`frontend/src/design-system/README.md`](frontend/src/design-system/README.md),
and the two do not overlap: this file decides *how it should look*, the catalog
decides *what you build it from*.

Read this once before designing anything a reader can see, and again when a
screen "looks fine but not like the others" — that sentence is the symptom this
file exists to cure.

## 1. Five things the language refuses

Stated first, because the rules below are easier to hold when the failure
each one prevents is named.

- **Everything as a box.** Panels inside panels, each with its own border,
  radius and shadow, on a ground nearly the same colour as the panels. When
  every block is a card, every block has the same visual rank and the eye has
  nowhere to land. A page has *sections*, and few of them are cards.
- **Chrome the colour of the content.** A sidebar, a page and a card in the
  same white are three surfaces asking to be the foreground. The frame must
  recede from what it frames.
- **One typeface at one size for every job.** When titles, labels, values and
  body differ only by a few pixels and a weight step, a record's name and a
  field label read as the same kind of thing.
- **Numbers set like words.** Money and counts in a proportional face with no
  alignment make a column of amounts a ragged block rather than a ledger.
- **Colour spent on nothing.** When the accent is a button and a link and the
  rest is grey, a product with a strong opinion about *who did this* (a contact
  or an agent) shows that opinion at chip size only.

The answer to all five is not a new colour; it is a hierarchy.

## What the refined products actually do

The products contacts name when they say "that looks professional" — Linear,
Attio, Vercel, Stripe, Raycast, Notion — disagree on colour and agree on
almost everything else. The techniques below recur in every one of them, and
each is stated here as the rule this language adopts, so a screen can be
checked against it. The sources are listed at the end of the section.

1. **Depth comes from a surface ladder, not from shadows.** Linear and Raycast
   draw no drop shadow at all: a card is one step lighter than the page, a
   hovered row one step lighter again, and that ordering is the whole
   elevation system. Vercel is "border-first": a 1px hairline defines every
   static element and a real shadow is reserved for something floating above
   the plane. **Rule:** paper → surface → raised is the ladder, and it carries
   the separation. A resting surface takes ONE tight layer on top of that —
   `--shadow-rest`, 1px down and 2px of blur at 5% — which gives it a top side
   without making it float; only a popover, a menu or a drawer casts a shadow a
   reader would call one. Never two shadows on one element.
2. **When a shadow is cast, it is tinted and layered.** Stripe's shadows are
   blue-grey (`rgba(50,50,93,.25)`) because its brand is navy, with a second
   tighter layer close to the element; a pure black shadow is what makes a
   card look pasted on. **Rule:** there are exactly three depth tokens, all
   from a single light source above — `--shadow-rest`, one tight layer for a
   thing with a top side; `--shadow-well`, that same layer turned `inset` for a
   field, which has a floor instead; and `--shadow-pop`, soft and far, for what
   is genuinely above the plane. All three are themed: the ink is the theme's
   to decide, the geometry is not.
3. **A rim light on the top edge is what makes a filled thing look made.**
   Raycast's buttons and keycaps carry `inset 0 1px 0 rgba(255,255,255,.1)`;
   the same one-pixel highlight is on every "premium" control the craft guides
   describe. **Rule:** the primary button, a raised card and a keycap take a
   1px inset white highlight on their top edge; nothing else does.
4. **Nested radii obey one law.** Mismatched corners on a control inside a
   card is the most common single reason an interface reads as "off":
   inner radius = outer radius − padding. **Rule:** a 20px pane with an 8px
   inset holds 12px controls; `--r-lg` 20, `--r-control` 12, and a keycap 4.
   The ladder runs in fours — 4 / 8 / 12 / 16 / 20 and the pill — and doubles
   under `corner-shape: squircle`, where a superellipse of radius R reads about
   as round as a circular corner of R/2.
5. **Text is near-black slate, never pure black; grays carry the brand hue.**
   Stripe sets text in deep slate on near-white; Refactoring UI's rule is that
   a gray far from mid-lightness needs saturation or it looks washed out.
   **Rule:** `--ink #101a15` and the whole ink and ground ladder sit on the
   emerald hue at low chroma, which is what keeps the page from reading as a
   grey page with a green accent.
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
   `-0.025em`, body at 400 and 500, 700 nowhere; every figure tabular in the
   body face; the display face is the one place the type has a voice.
8. **Density is a feature.** Attio's whole product is a dense grid with
   high-contrast labels, subtle material and no loud brand elements; the 2026
   guidance is that every visible element earns its place and density and
   clarity coexist through strong opinions about the workflow. **Rule:** rows
   are 44–48px, a table shows eight columns before it scrolls, and the record
   page answers the rep's first five questions above the fold. Dense is not
   the same as cramped: the density comes from saying each thing once, not
   from shaving the space between rows.
9. **The keyboard is visible.** Linear, Raycast and Attio surface a command
   field with its shortcut, and show the key beside every verb in a menu; a
   keycap is drawn as a small physical key (a one-step gradient, a 4px
   radius). **Rule:** `⌘K` on the command field, `/` on the ask field, and the
   shortcut on every menu row.
10. **Motion: transform and opacity only, four durations, one easing
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

1. **A lit ground, and one pane per zone.** The page is a pale green paper lit
   from two corners, an emerald glow behind the sidebar and an indigo one
   at the top of the far edge. Each zone of a record is one white pane on it,
   with a hairline edge and a 20px corner, and inside a pane there is only ever
   a title, a rule and rows. Dark is the same room with the lights down.
2. **Everything is a list.** A record's attributes are a list of label and
   value in a panel on the left that folds. What happened is a list. What
   needs you is a list. Money is a list. Because every list is the same list,
   a rep never learns a second layout.
3. **One display face carries identity.** The name of a record, a zone's
   title and the verdict the agent speaks are set in the display face;
   everything else, figures included, is one quiet sans with tabular figures.
4. **Buttons are flat and there is one filled one.** No gradient, no rim, no
   glow: a filled emerald verb for the move the page names, white outlines for
   the rest.
5. **Colour means something.** Emerald is the one filled verb, a link and the
   light behind the sidebar. Indigo is a tinted row that says an agent wrote
   it, and the light at the top of the far edge. Green, amber and red are a
   soft tint behind a word. Nothing is coloured to look nice.

Rules 1 and 5 are already held by gates; this file adds 2, 3 and 4.

## 3. Colour

The semantic split is unchanged: `--accent` (emerald) is brand and primary
action, `--ai*` (indigo) is agent provenance, `--success` / `--warn` /
`--danger` are status. Those names are the ones `tokens.test.ts` pins, together
with `--ai`, `--aiLight`, `--aiMed` and `--aiText`. What changes is the ground,
which is lit, and the surface, which is one translucent pane per zone.

The tables below are the design TARGET, and they name some rungs the tree does
not carry yet — `--ink4`, `--aiBg`, `--aiLine`, `--ok`, `--bad`. Read a name that
does not appear in `frontend/src` as a value still to be introduced, not as one
to reach for today: the shipped spelling of the agent tint is `--aiLight`, its
edge is `--aiMed`, and status is `--success` / `--warn` / `--danger`.

### Light (the default)

| Token | Value | Role |
|---|---|---|
| `--bg` | `#f1f5f2` | The paper the page is read on. |
| `--glowA` / `--glowB` | `rgba(24,190,120,.06)` / `rgba(91,97,214,.10)` | The emerald light at the top-left corner behind the sidebar; the indigo light at the top-right. Radials of 900×620, the only decoration on the page. |
| `--pane` / `--paneEdge` | `rgba(255,255,255,.72)` + `blur(12px)` / `rgba(16,26,21,.08)` | A zone, the details panel, a board card, a control at rest. |
| `--bg2` / `--bg3` | `rgba(255,255,255,.55)` / `rgba(16,26,21,.05)` | The sidebar (glass over the glow, `blur(20px)`); a pill, a keycap, the active sidebar row. |
| `--bgChip` | `rgba(16,26,21,.07)` | The shipped spelling of `--bg3`'s pill and keycap: the fill under a badge, a key-cap, a segmented strip, the trough of a meter. TRANSLUCENT, so a chip reads one step deeper than whatever ground it lands on — an opaque value can only be a step off one, and on a plate drawn in the same grey it is a chip nobody can see. `.07` and no deeper: `--accentText` on this fill reads 4.57:1 over `--bgCard`, the worst of the four grounds a chip lands on, and `.08` would take that to 4.49:1 — under the floor. |
| `--line` / `--line2` | `rgba(16,26,21,.08)` / `.16` | The hairline between rows; a control's outline, the spine's axis. |
| `--ink` / `--ink2` / `--ink3` / `--ink4` | `#101a15` / `#33403a` / `#66736c` / `#9aa59f` | Names and values / body / labels and meta / placeholders and dates. |
| `--accent` / `--accentText` / `--accentBg` | `#0b7a53` / `#0a6f4b` / `#e8f3ee` | The one filled verb; a link; a selected row or a done stage. |
| `--ai` / `--aiText` / `--aiBg` / `--aiLine` | `#5b61d6` / `#3f45b0` / `rgba(91,97,214,.09)` / `.35` | The agent's filled verb; its label; the tinted row; the dashed edge of a staged row. |
| `--ok` / `--warn` / `--bad` | `#15803d` / `#a16207` / `#b91c1c` | Status, as a soft badge — the tint behind a word, lettered in the tone's ink and edged in its hairline — or as a solid one for a count and the one status that must not be missed. |

### Dark

The same room with the lights down: `--bg #0c1311` (a hair above the mock's
`#0a100e`, so the sidebar's hover step still fits under the page), the glows brighter
(`.10` / `.20`) because they are the only light, panes at
`rgba(255,255,255,.045)` with a `.09` edge, ink from `#eef3ef` down to
`#5c6862`, the accent lifted to `#2bb673` with dark ink on it, the indigo text
lifted to `#b3b7f5`. The chip inverts rather than mirrors: `--bgChip` becomes
`rgba(255,255,255,.09)`, because a chip on a dark ground has only one direction
to step. The three-state theme pattern (`:root`,
`prefers-color-scheme` guarded by `:not([data-theme="light"])`,
`[data-theme="dark"]`) is how they switch.

### How colour is spent

- **The two glows are the chrome.** The sidebar is glass over the emerald
  light; there is no coloured bar anywhere.
- **The page carries at most ONE filled control in view**, in emerald: the
  move the page names. Every other button is a pane with an outline.
- **Indigo is a fact, not a mood.** The agent's read is a row on `--aiBg`; a
  staged change is a row with a dashed `--aiLine` edge until a contact accepts
  it. Nothing else is indigo.
- **Status is a soft badge before it is a solid one.** A column of statuses
  is a column of soft badges, one weight down the page; the solid fill is for
  a count and the one status a reader must not miss.
- **Avatars are neutral.** `--bg3` with `--ink2` initials; a record is told
  apart by its name.

## 4. Type

Three families, which is the ceiling `check-font-lock.sh` holds. The third is
for code alone: a mono face on an amount or a date dressed a business fact as
machine output and shouted in a dense row, and `tabular-nums` gives a column the
alignment mono was bought for.

**Size answers the context; level answers the structure.** How big a heading is
says what it introduces and where — brand and marketing at the top of the
ladder, a product page's title below that, a component's own title at the
bottom. Which of `<h1>`–`<h6>` it is written as says where it sits in the page,
which is how a screen-reader user moves through it: one `<h1>`, descending, no
level skipped. Neither decision is allowed to settle the other. Body is three
levels on the same foundation, each carrying the paragraph spacing that
separates two blocks of its own prose, and weight is meaning rather than
decoration — and there are exactly three, shipped in both text families and
loaded as such, so a fourth is a face the browser synthesized: 400 is prose, 500
is text that sits beside a line icon and most text inside a component, 700 is a
heading or an emphasis that has earned it.

Everything is rem on the browser's own 1rem = 16px, so a reader who enlarges
that moves the product with them.

**Almost none of it is applied yet.** `body` in `app.css` reads `--fontBody`,
and that is the whole of it: no `h1`–`h6` mapping, no class hooks, and the
`.t-*` names in `base.css` are role hooks with no rules on them. The tokens and
the usage rules that go with them live in
[`frontend/src/design-system/README.md`](frontend/src/design-system/README.md),
which is what an implementer reads. So every size, weight, tracking and neutral
ink named anywhere else in this document is a TARGET for that rebuild rather
than a description of the sheet that ships.

| Role | Family | Where |
|---|---|---|
| Heading | **Outfit** | Every heading, and the one line a record is identified by: its name, the Brief greeting, a zone's title, the agent's verdict word, a reading's word and its figure. |
| Body and UI | **Geist** | Everything else, prose included. |
| Figures | **Geist**, tabular (`.t-num`) | Every amount, count, percent, duration and date in a row or a cell. An identifier is plain body type. |
| Code | **Geist Mono** | `<pre>`, `<code>` and `<samp>` through one rule in `base.css`, and the `.code-block` surface; nothing else. A `<kbd>` is body type. |

## 5. Space, shape, depth

- **4px base**, the existing `--space-*` ladder, spent generously. The page
  gutter is 32px; panes sit 24px apart; a pane has 20px above and below its
  content and 26px at its sides; a zone title has 12px under it.
- **Rows breathe.** A list row is 13px above and below its content (about
  48px tall with two lines); a table row 44px; a sidebar row 38px; a control
  32px (28px in a row); the attribute rows in the details panel 32px. The
  test for "gedrungen": if two rows could be mistaken for one, the row is too
  short.
- **Type at rest is 13.5px on 1.55**, so a row's second line does not touch
  its first; prose is 14px on 1.65 at 72 characters.
- **A role that has a class is spelled as the class** — `.t-caption`,
  `.t-sub`, `.t-label`, `.t-eyebrow`, `.t-h3` and the rest, all in `base.css`.
  What each one draws is §4's rebuild, taken one role at a time.
- **Radii by role**: 20px for a pane, the details panel and a reading card;
  16px for a board card and the agent's row; 12px for a control; 8px for a chip
  and 4px for a keycap; full for a pill and a monogram.
- **Depth is light first, shadow second.** A pane is translucent over the lit
  ground with a hairline edge; that is its elevation. On top of it the question
  is what the thing IS. A surface — a pane, a card, a reading, a board card —
  and a control with a FILL have a top side, and take `--shadow-rest`, one
  tight 1px/2px 5% layer. The fill is what decides a control: a ghost button
  casts, because the pane is its fill; a bare icon button has none and casts
  nothing, and neither does a text affordance. A FIELD is a place to put
  something, so it has a floor rather than a top side and takes
  `--shadow-well`, the same layer turned inside — a text box, a textarea, a
  field shell, and anything DRAWN as a field, the topbar's search included,
  because a reader reads the shape and not the element. The one exception runs
  the other way: the `Select` trigger holds a closed face rather than a place
  to type, so it is verb-shaped and takes the resting layer. Either way a control closes the gap on hover, active and focus — a
  verb presses into the page, a field's floor comes up to meet the pointer. A
  popover, menu or drawer takes `--shadow-pop` instead. Nothing at rest glows
  or has a gradient, and nothing stacks a shadow under a shadow.
- **No pane inside a pane.** A zone is one pane; the things in it are a
  title, a rule and rows. The only enclosed shapes inside a pane are the
  agent's tinted row and a staged row's dashed edge.

### More, and less

One layout holds a record with nothing on file and a record with forty
deals. A pane exists only when it has something to say: an empty zone is one
line in its place with its verb ("No deals yet · New deal"), a withheld zone
is its sentence, and the order of zones never changes with content. A thin
company leads with the deep-read card where the 360 would be; a thin contact
shows its readings as "Not shown" without a tone and says nothing has been
captured. A rich record is capped, not stretched: three rows and "N more"
into the tab, five readings and no sixth, three claims in the 360 sentence,
six stops on the spine, five in the folded thread, the lead row plus three in
What needs you, and "at least N" on a truncated page.

### Responsiveness

One ladder for every screen, on the product's existing breakpoints: at
1400px the figures drop a size and five readings stay across; under 1200px
the readings wrap, the two columns stack, the details panel becomes a drawer
over the page and the tab strip scrolls in its own box; under 720px the head
stacks (mark and name, facts, then primary + more), readings go two per row,
and the spine and the map scroll sideways in their own boxes; at 390px the
phone bar and the no-sideways-scroll rule hold. Fixed widths exist only for
the details panel and the map; everything else is `fr`, `minmax(0, …)` and
`min-width: 0`. A page never scrolls horizontally; the thing that is too wide
scrolls inside itself.

## 6. The shell

```
┌────────┬───────────────────────────────────────────────────────────────┐
│ sidebar│ crumb        [ Search or run a command  ⌘K ]                me │
│ (glass ├───────────────────────────────────────────────────────────────┤
│  over  │ mark  NAME (display)  badges                  verbs, right     │
│  the   │       facts line                                               │
│  glow) │ tabs ──────────────────────────────────────────── [Details] │
│ Brief  │ ┌ THE 360 ─────────────────────────────┐ ┌ details (288) ────┐│
│ RECORDS│ │ ● {name} · 360                        ││ │ Ask               ││
│ …      │ │ VERDICT  because · sources (hover)    ││ │ Details           ││
│ WORK   │ │ readings ┊ readings ┊ readings        ││ │ Contacts / Seats    ││
│ …      │ │ spine · · · gap · today · ahead       ││ │ Tags / Room / Docs││
│ ● agent│ │ the thread, newest first              ││ └───────────────────┘│
│        │ └───────────────────────────────────────┘│                      │
│        │ ┌ What needs you ┐ ┌ Commercial ┐ ┌ About ┐                     │
└────────┴───────────────────────────────────────────────────────────────┘
```

- **Sidebar**: 224px expanded, 64px collapsed, and collapsed is the default.
  Glass over the emerald glow, a hairline on its right. Workspace name and the
  fold at the top, then the product's own ten rows in its own order: Brief;
  Records — Contacts, Companies, Leads, Filters & views; Work — Worklist,
  Pipeline, Projects; Intelligence — Reports, Ask Margince. No badges: the
  queues that used to badge (approvals, tasks) are lanes of the Worklist, which
  reports its counts on the page. Settings is not a row; it opens from the
  account menu and publishes its own second level (Overview, then the subject
  groups) on the same glass, which the sidebar becomes while a settings route is
  open — at the sidebar's own width and under the sidebar's own head, because it
  is the same column showing a different list rather than a second panel. The
  level names itself with the heading over its first group, and the way out of it
  reads "Back to app". The agent's orb stays at the foot.
- **Top bar**: 48px, glass, a hairline under it. The breadcrumb on the left,
  the command field in the middle (`⌘K`), the reader's monogram on the right.
  It is the application's one bar and every screen has it. Nothing that
  belongs to a record sits in it: the Details toggle is in the page.
- **Record head**: a 56px mark, the name in the display face with its badges
  on the same line, one line of facts under it, the verbs on the right. **The
  verbs are the record type's base actions and never the task of the day**:
  a company shows Write email · Log activity · Add task · more on every
  company; a contact Write email · Call · Add task · more; a deal Write email
  · Log activity · Edit deal · more; a lead Qualify · Write email · Edit ·
  Disqualify · more; a project Log activity · Edit project · more. **None
  of them is filled.** The head's verbs are all outlined, so the page has one
  filled verb and it is the move the call names ("Confirm the rate", "Reply
  on her thread") inside What needs you, where its reason sits beside it.
  The one exception is a lead, whose Qualify is filled because qualifying is
  the record's whole purpose. The **more** button
  (an outlined 36px square with the ellipsis) holds Archive, Share, Merge and
  the rest. The
  **Details** control sits at the right end of the tab row, in the page.
- **Details panel**: 300px on the right of the reading, one pane, closed
  until the Details control at the end of the tab row opens it. It holds Ask, the attributes as label and
  value rows (with the evidence underline on a machine-read value and a
  lighter "Add …" on an empty one), and the short lists that describe the
  record (contacts, seats, tags, the Deal Room, documents). It is where the
  current product keeps its context column, so a rep's hand does not move.
- **The glows**: the emerald light at the top-left is faint (`.06` in light,
  `.10` in dark); the indigo one at the top-right a step stronger. They are
  atmosphere, not a feature, and the eye should not find them.

## 7. How the tool feels, and how a page is structured

**The feel.** Calm, lit, and legible at arm's length. A rep opens an account
and in one screen knows where it stands, what they owe, and what happened; the
agent speaks in one place on the page and says what it rests on; every figure
is in the same face and lines up; nothing glows, and the one shadow anything
rests on is too slight to notice as a shadow. It should feel like a well-lit
desk with one folder open on it, not a dashboard.

**Five rules of structure**, each preventing a shape a record page falls into
as it grows (a fact with several homes, verdicts stacked above the list they
should lead):

1. **Every fact has one home.** If a deal appears in the readings, it does not
   also appear in the rail. If contacts are in the context column, they are not
   also a section in the work column.
2. **Zones, numbered, in reading order.** A record page is five zones with a
   heading each. The number is not decoration: it is the order a rep reads an
   account in, and it lets a colleague say "look at 3".
3. **One list of what needs you.** The agent's read of the record is the lead
   row of that list, not a separate card above it; the agent's finds, the manual
   moves, the overdue tasks and the next meeting are rows below it, sorted by
   urgency. A contact has one place to look and one place to clear.
4. **One timeline.** The spine (the axis with the gap drawn at width) heads the
   list of what happened; they are one component, not two panes.
5. **The context column answers "who and what is this", not "what is
   happening".** Ask at the top, then the contacts, the fields, the tags. State
   and work stay in the work column, where the reading order is.

### Glance, then depth

The overview that breathes is the one that shows less. A record's overview
has two depths, and the shallow one is the default.

**The glance** is the whole first screen. It keeps the air of the sparse
version and brings the product's own features back in reduced form — each
one present, none at full depth:

- The name at 32px in the display face beside a 56px mark, both centred on
  one axis so a facts line that wraps never breaks the head; the facts with the live dot
  ("In conversation · Hamburg · Freight forwarding · 240 employees · Owner"),
  and three verbs. No badges beyond the standing, no second line.
- **A quiet tab row** under the head — no pane, no rule, a 2px accent under
  the current tab — so every sub page (History, Contacts, Deals, Tasks,
  Finance, Documents, Profile, Partner) is one click away. The links inside
  the panes ("All deals", "Full history") open the same tabs.
- **Five readings as roomy cards**, 138px tall, an uppercase label, one
  figure at 26px in the display face with tabular figures (22px under 1400px), one line of basis at the foot, and empty
  space between them. **Every reading is a claim, so every reading carries
  its evidence**: an "evidence" chip at the right of the label that opens, on
  hover or focus, the popover with what the figure rests on (the rows it sums,
  the date it was read, the connection it came from). **Every reading is a
  door**: the card opens the sub page that holds its rows (Open pipeline →
  Deals, Invoiced → Finance, Conversation and Last touch → History, Next →
  Tasks), and the evidence chip does not. When the details panel is open they shrink to 21px
  figures and never wrap.
- **The 360 as the first pane**, full width, on the same hairline as every
  other pane: the record's name and "· 360" as the pane title, the agent's word
  at 34px, one sentence beside it whose claims are hoverable sources (the
  chip sits inline on the agent's tint; hover opens what it rests on), the
  three rated dimensions, then the spine. The thread is folded — "Read the
  thread · 5" opens the five most recent exchanges inside the pane; "Full
  history" is the History tab.
- **Left column**: What needs you, one list. Your move leads it as the
  agent's row (a headline in the display face, one sentence, its sources,
  the filled verb that does it); the agent's suggestion is the second row
  of the same list, not a card beside it; then the tasks, the call as the
  task of preparing it. Below it Deals, three rows.
- **Right column**: Ask as a pane of **prepared questions** (four the record
  can answer today — "What changed since I last looked?", "Why is Rostock
  stalled?" — each a full-width row on `--bg3` that tints indigo on hover;
  no free-text field until the answers earn one), About (the lead sentence,
  one paragraph, its sources, "Profile" for the rest), Contacts as chips with
  a "+N".
- **The details panel is hidden until asked**, and the Details control at
  the end of the tab row opens it on the right at 300px with the fields, the contacts and
  the tags. It is the only thing on the glance that starts closed.
- No zone numbers. Commercial and the rest of About are the deep overview,
  one click away, with the same panes.

**One home per fact, one word per screen.** The glance says each thing once:
the readings carry the figures, the 360 sentence carries the because, the
spine carries the time, the needs list carries the verbs. "Your move" is a
reading, a spine stop and the lead row's eyebrow, never a second verdict word:
the 360 owns the only display-face word on the screen, and it is about the
record ("Good", "Live", "Promise overdue"), while the needs list's lead is a
sentence about you. The way in sits on the facts line, because that is the
first thing a rep wants before a call.

**The same depth on every record.** Contact, deal and lead follow the company
glance line for line; what changes is the content of each slot.

| Slot | Company | Contact | Deal | Lead |
|---|---|---|---|---|
| Live dot | In conversation | Your move (warn) | Your move (warn) | In motion |
| Facts | city · industry · size · owner · way in | title · employer · email · phone · way in | account · value · stage · close · owner · partner | title · company · email · source · owner |
| Verbs (base, never the task) | Write email · Log activity · Add task · more | Write email · Call · Add task · more | Write email · Log activity · Edit deal · more | Qualify · Write email · Edit · Disqualify · more |
| Readings | Open pipeline · Invoiced · Conversation · Last touch · Next | Whose move · Open promises · Deals she decides · Next meeting · She answers in | The money · The close · Stage · The contacts · Momentum | Company · Score · First response · Next · Your move |
| 360 word | Good | Promise overdue | Live | In motion |
| In the 360 | the spine | the spine | the stage stepper, then the spine | the ladder, then the spine |
| Needs-list lead | Your move | The move the call names | Margince suggests, then the staged change | Ready to qualify (no agent tint: a lead carries no suggestions) |
| Second left pane | Deals | The deal she decides, with the room | The buying committee, with the cover gap | The score as factors |
| Right column | Ask (prepared questions) · About · Contacts | Ask · Understanding her · Around her | Ask · What this deal is · Offers · Deal Room | Ask · If she is qualified · What she asked for |
| Tabs | Overview · History · Contacts · Deals · Tasks · Finance · Documents · Profile · Partner | Overview · History · Network · Deals · Meetings · Data & tools · Documents | Overview · History · Documents | Overview · History |

The test: a rep back from a week away reads the glance in ten seconds and
knows where the account stands, what happened last, what they owe, and what
the agent proposes. Whatever those ten seconds do not need goes to the depth.


### The order of the zones

1. **Who.** Identity, the standing badges, one line of facts, the verbs.
2. **The 360.** One element, and the one place the page is allowed to look
   like a feature: where the record stands and what happened, together. It
   opens with the indigo tile and "{name} · 360" — the tile IS the claim of
   authorship, and a sentence beside it repeating it read as a caption typed
   onto the record's name —
   then the verdict word in the display face with its because-sentence and
   its sources, then the readings as cells, then the stage stepper or the
   ladder where the record has one, then the spine and the thread, newest
   first, and a foot that says who wrote it. Its edge is the same
   hairline as every other pane — no thicker border, no coloured rule; the
   indigo tile and a faint indigo wash in one corner are its whole ornament. Every claim in it is hoverable: a source chip opens the quote it
   rests on and where the quote came from.
3. **What needs you.** One list. The move the call names is its lead row; the
   agent's finds, staged approvals, the reply that is owed, overdue and due
   tasks and the next meeting are the rows under it.
4. **The money or the score.** Commercial on a company and a deal; the score
   on a lead.
5. **The slow-changing rest.** About on a company, understanding on a
   contact, what the deal is and the buying committee on a deal.

The details panel is the same on every record: Ask at the top, then the
panes that answer "who and what is this".

**Sub pages.** Every tab in the strip opens a real page, in the same panes:
History (the filter strip, then the **rail timeline** below), Contacts (the
coverage band, **the committee map**, the roster as a table with list, board
and map cuts),
Deals (the commercial band, the deals table with won and lost), Tasks (tick,
snooze, open), Finance (five readings with their evidence; **invoiced by month** as bars,
the **overdue share** as a meter with its legend, **payment behaviour** as a
line of days late per settled invoice, oldest first; the recent invoices
table; the provider and sync line; and the reasons finance cannot be read
drawn as rows, never as a zero), Documents
(contracts, then files with category and origin), Profile (the details form,
what they do, the facts Margince read with confirm and correct, linked records,
data and tools), Partner (the programme and its deals); on a contact History,
Network (the best route, the ways in, **the relationship map**, what changed),
Deals (the seats she
holds), Meetings (next, with brief and room; past), Data & tools (the provider
snapshot and what Margince read), Documents; on a deal Documents and History;
on a lead History.

### The deep overview, record by record

The numbered anatomies below are the deep overview: everything a record can
say, in order. The glance draws its panes from them and the sub pages carry
the rest; nothing below is lost, it is one click further in.

### The company record

1. **Who.** Mark, name, lifecycle and relationship badges, one line of facts
   (site · industry · size · owner · way in), one quiet line (last contact ·
   created · captured by), and the verbs: Write email, Log activity,
   Add task, more.
2. **Where this account stands.** Four readings. Health carries the call: the
   standing word, whose move it is, and the three rated dimensions as dots.
   Open pipeline, Invoiced, Conversation, each a door into its tab. "Not shown"
   and "Not assessed" stay distinct.
3. **What happened.** The spine, then the recent rows. "N new since your last
   visit" in the zone head.
4. **What needs you.** The moment as the lead row (indigo ground, the verdict
   word in the display face, what it rests on, the agent's action in the agent's
   fill). Then the agent's finds, the overdue and due tasks, the next meeting.
   The foot names what was hidden from the reader.
5. **Commercial.** Contract state, won and lost on one line; then each open
   deal with its one status clause; then the project it belongs to.
6. **About.** The dossier lead and paragraph, its provenance and age, the
   signals, the fit verdict with a link to how it was judged.

Context column: Ask (with three prepared questions), Contacts (three, with who is
in touch from our side), Details (nine fields including mail capture), Tags.

**What merged, moved or went.** The call card merged into Health. Next steps,
the agent's suggestions, the manual moves and the tasks merged into one list.
The record spine merged into the timeline, and the timeline moved up to third.
Work in flight, the commercial figures and projects merged into Commercial. The
dossier, signals and growth fit merged into About. Active deals left the rail;
key contacts became the rail's Contacts; Ask moved from the foot of the page to
the top of the column. Nothing the page could say is gone; each thing is said
once.

### The contact record

1. **Who.** Round monogram, name, the buying-role badge, title and employer,
   the glyph line (email, phone, city, LinkedIn, owner), and the verbs: Write
   email (named for the transport when there is exactly one), Call, Add task,
   more — all outlined.
2. **Where you stand with her.** One strip of seven: Overall (the pulse
   verdict, in its colour only when not withheld), Last inbound, Last
   outbound, In · out as counts, Colleagues, Next meeting, Consent.
3. **What was said.** Conversation memory with its All / Email / Meetings /
   Calls / Notes cut, replied or unanswered, a reply verb per row.
4. **What needs you.** The moment as the lead row with its actions and their
   readiness; then the commitments and open loops as rows with a read-only
   tick.
5. **The deal she decides.** Title, amount, stage, close, owner, the buying
   committee.
6. **Understanding her.** The relationship brief, then Priorities, Objections
   and Success as three rows, then provenance and "Correct something".

Context column: Ask, the waiting email, Details, Who knows her, Consent &
channels.

**What merged, moved or went.** The rail's relationship pulse merged into the
strip. Commitments merged into the needs list. The rail's recent activity
merged into what was said, which moved up to third. The brief and what matters
merged into Understanding. What Margince read moved to Data & tools. Consent
became a compact rail pane; the grant and withdraw table lives behind Manage.

### The deal record

1. **Who.** Mark, name, the status badge and the project chip, the company
   and partner line, the three facts (Value, Stage, Owner) and the pulse
   sentence ("It's your move. They wrote last on 1 Sep — 3 days ago."). Verbs:
   the reply that is owed (filled, because the pulse says it is your move),
   Log activity, Edit deal, more (Archive, Share, Reopen).
2. **Where this deal stands.** The money (with the newest offer and its
   status), The close (days, forecast category, provisional or waiting), The
   contacts (engaged of total, champion named, single-threaded), The momentum
   (days since the last contact, stalled). Under them the stage stepper: done
   stages tinted, the current one filled, terminal stages last, the rule that
   a terminal stage asks first stated beside it.
3. **What happened.** The spine (meetings, calls, offers, the gap, today, the
   expected close), then the rows, including the agent's own stage change.
4. **What needs you.** The Deal360 briefing as the lead row: the verdict word
   (Live / Drifting / Blocked / Cold), the because-sentence with its citations
   inline, the coverage signals as chips, "What to do next" with its one verb,
   "Read the full briefing", written-by and write-it-again. Then any staged
   approval (dashed, Accept / Dismiss), then the reply that is owed.
5. **Commercial.** The offers table (number, revision, status, value, sent),
   the FX basis in the zone head, then its offer actions. Forecast and wait-until are edited in Details;
   custom fields have their own Details section.
6. **The buying committee.** The map (our circle, their seats, threads only to
   the engaged, a dashed ghost per coverage gap), then the stakeholder table
   with role, contact, talking, dates and edit. Add stakeholder in the zone
   head.

Context column: Ask, Deal Room (state, invited, signed in, last seen, open),
Who is on this deal (seats with Engaged / No two-way contact and who of ours
carries it), Related evidence, Documents.

**What merged, moved or went.** The stage stepper moved beside the readings.
Deal360, the approvals queue and the email box merged into the needs list.
Offers and the FX line sit in Commercial. Editable fields and custom fields
live in Details. The committee map,
the seats and the stakeholders table merged into one zone, with the read-only
seats list kept in the rail as the short form. Log activity became a verb, not
a form on the page. The Documents tab keeps its full list; the rail shows the
latest two.

### The lead record

A lead is kept apart from the contact graph on purpose, and the page says so:
the ground carries the accent tint as its marker, the badge reads "Lead", and
the segregation sentence sits under the identity. Nothing on a lead is indigo;
the tint says "not a contact yet", which is a different claim from "an agent
did this".

1. **Who.** Monogram, name, the Lead badge, title and company as text (a lead
   has no company record), email, LinkedIn, source and date, owner. Verbs:
   Qualify (filled), Edit, Disqualify, more (Share).
2. **Where this lead stands.** Score (with its top two factors), Status (with
   how it was set: by hand, or automatically from a reply or a meeting), First
   response (the SLA verdict and the target), Source. Under them the ladder:
   New › Contacted › Engaged › Qualified › Disqualified, the terminal two
   opening a dialog.
3. **What happened.** The spine from the web form to the booked demo, then the
   rows, including the automatic status change.
4. **What needs you.** "Ready to qualify" as the lead row when the evidence
   the product asks for exists (a reply or a meeting), with the derived reason
   and "Qualify and open deal"; then the reply that is owed, then the tasks.
5. **The score.** Explain this score (each factor with its points, decay and
   activity count, the reconciliation line), then what you know about this
   lead (the three questions, how you know, add to the score), with Override
   score in the zone head.
6. **Details** live in the context column with Owner and the qualify preview
   ("Merges into nobody", the suggested deal), because a lead has few fields
   and the page's work is the ladder and the score.

**What merged, moved or went.** The ladder moved beside the readings. The
promoted-lead panel, the email box and the tasks merged into the needs list
(the promoted panel becomes the lead row once the lead is qualified). Owner
and score split: owner to the rail, the score breakdown and the manual signals
to one zone. The write-refusal callout and the terminal sentence keep their
place under the identity.

### The shell around every record

The sidebar collapses to a 64px icon rail and that is the default. Labels
return on hover as tooltips, the agent's orb stays at the foot. The details
panel on the right of the reading opens from the Details control at the end
of the tab row, and starts closed.

### The timeline, as the tool draws it

History is a **rail**, not a list of rows. Each entry is a grid of four: the
date in tabular figures, right-aligned in a 76px column with the time under it
in `--ink4`; a 20px rail column carrying a 1px `--line2` line that runs the
full height so consecutive entries join, with a mark on it; the body; the
verb slot. The mark says what kind of thing happened: a solid 8px dot for an
exchange, a hollow one for a field change, an indigo one for a change the
agent made, a dashed indigo ring for a staged change, and a 24px circled
glyph for a thread. The body opens with the kind in 10.5px uppercase, the
direction words ("they wrote", "we sent", "both sides") and the contacts; then
the title at 13.5px 600; then **the text of the message itself**, clamped to
three lines, because a timeline of subject lines is a list of things you
cannot read; then a meta line (the summary's author, the attachment, Restore).
A thread is one entry: its mark on the rail, a card on the body side with the
count and the contacts, and the messages inside it newest first, each with its
avatar, direction and two lines of text. A hairline under each entry is the
only divider; there are no day headings, the date column is the axis. The
same rail, without the message text, is the 360's folded thread.

### The spine, as the tool draws it

The spine keeps the product's own geometry. Each stop is a column of four
rows on one baseline: the date at 11.5px in `--ink3`, then the **rail** — a
2px `--accent` rule with a 9px filled dot at its start, so the rule to the
right of a dot is the span that stop covers — then the title at 13px, then
the detail. The gap stop takes 1.5× the width, because on this axis the
width is the waiting: its date row is the day count at 26px 600 in amber in
the display face — the one count on the spine not in the body face, because it is read
as a word ("12 days") and compared with nothing — its rule is dashed amber, and it has no dot, because
nothing happened. **Today is a marker, not a stop**: a 2px × 15px black bar
on the rail with TODAY in 10.5px uppercase and the date under it, in a column
that is only as wide as its label. Right of it the rule turns dotted grey and
the dots hollow with an accent ring: those stops have not happened, and a
solid rule to a date nobody has reached draws the future as firmly as the
past. An event dated today (the 14:00 call) is an ahead stop after the
marker, not the marker itself.

### The relationship map

One drawing primitive, the product's `RelationshipMap`, on three pages, with
its own geometry: three columns of 184 / 200 / 184px with 72px gutters and
16px padding (744px, scrolling sideways rather than shrinking), node heights
by kind (a colleague 40, a contact 60, a company 48, a deal 44, a gap
60), 8px between nodes, 20px between lanes, a lane heading in 10.5px
uppercase. Nodes are rounded boxes on `--pane` with the name at 13px 600 and
a sublabel; a colleague on `--bg`, a company with a 2px `--ink3` edge,
a deal with an accent edge, and **a gap as a dashed amber box** naming the
missing role with "Assign" under it — the only drawing of an absence a
reader can count. Edges are cubic curves: a route is a way in and carries a
band (strong: 2.5px accent; developing: dashed accent; cold: dotted grey),
a membership is a 1px hairline (works there, on this deal); an unmeasured
relationship is omitted, never drawn as cold. Selecting a node lights its
strongest route in ink and fades everything unrelated to 35%; a 280px panel
on the right names the selection, the best route with its evidence, the
alternatives, and the one write the picture offers (record an acquaintance
the graph saw). On the contact's Network tab the lanes are our team, the
target, their company; on the company's Contacts tab they are our side, the
account with its deals, and their contacts **by buying role** in the product's
order (champion, economic buyer, influencer, blocker, user), with a gap node
where a critical role is unheld. The deal keeps its small decorative
committee picture beside the seat rows: our circle, their seats, 2px accent
threads only to the engaged, hollow for the quiet, a dashed ghost per
coverage gap.

### The Deal Room, two surfaces on one board

The Deal Room is the one thing in the product a contact outside it reaches,
so it is two pages that share one document board and nothing else.

**The seller's side** is a page inside Margince, under the deal: "Back to
the deal", the room's title, a facts line (its state and end date, invited
and signed in, when a buyer last looked, the steward), and the verbs View
as buyer · Room access · more. A state band under the head says the
lifecycle in one sentence ("Live. Access ends on 31 Oct.") with Pause, Set
an end date and Close beside it. Left: the title and welcome the buyer reads
first (editable), the document board in its four groups (Commercial, Legal,
Security & Privacy, Delivery & Operations) with a thread and a composer under
each document, then the room-wide threads. Right: Room access (every
participant with what they may do, view or comment, whether invited, active
or revoked, when last seen, how many downloads, a row menu to issue a new
one-time link, change the capability, revoke), what is shared and what is
hidden, and the lifecycle.

**The buyer's side is not a Margince screen.** It is a public page reached
from a one-time link: no rail, no top bar, no seat, no pricing internals,
nothing the link did not already name. It is the one page a client ever
sees, so it reads like a small, well-made site rather than a form, on the
lit ground with both glows, in a 1040px frame:

- **A hero.** The contact's mark and "Prepared for {company} by {steward}"
  on the left, the live pill with the end date on the right; the eyebrow
  "Deal Room", the room's title at 40px in the display face, the welcome at
  16px. Under it, on the agent's tint, **"New since your last visit"**: the
  answers that arrived and the revisions that replaced a file, drawn from
  the participant's last-seen time.
- **The documents as a gallery.** Two tiles per row: a page preview on
  `--bg3` with the file-type tile and an "N unanswered" mark, the title,
  group and page count, the thread count as a pill, Download. Hover lifts
  nothing; the edge turns accent.
- **Questions on the documents**, answered in place: each thread under its
  document's tile, with the composer for those who may comment; then the
  room-wide thread.
- **On the right: the contact** as a card (mark, name, role, "Write to Tim",
  **"Book a call"** into the product's booking page), **next steps written
  by the contact** as a small ladder (done, now, ahead), and who is in the
  room as chips.
- Sign out at the foot with what the reader may do; "Powered by Margince"
  fixed at the bottom right is the only product mark. The page has four honest states of its own, each with its way back
and its contact: a dead link (with the one form: the invitation's email and
"Send me a new link"), paused, ended, and closed (read-only, the record
stands). A seller previewing sees the same page with a banner and can never
write.

### The pages that are not records

Every other page is the same shell — the top bar, a display-face title, one
facts line, verbs with the more button, five readings where the page has
figures, panes below — with its own content: the **Worklist** (one ranked
list with a kind column, what happened / why now / at stake / the verb, scope
and filter pills, the side pane showing what the selected row is about, the
team board), **Reports** (three reports, "Explain this number" opening the
plan and the rows on an indigo dashed pane), **Ask Margince** (the ask field,
an answer as a 360-style pane with its citations, the two-tier contract, the
passport instructions), the **Project 360**, **Filters & views** (the predicate
tree beside the results, saved views as chips), **Settings** (its second
level beside the rail), the **Deal Room** (the document board with a thread
per document), the **⌘K palette** (records, actions, screens, "Ask Margince:
…" always last, over a scrim that covers the whole frame), and the **compose
drawer** (the agent's draft with its disclosure, edit then confirm, send
later, relink).

### What main changed, and how this structure absorbs it

The record pages on `main` were reworked onto shared parts (the reading, the
call card, the day's panel, the timeline thread) after this document's first
inventory. The structure above holds; these are the facts it now carries.

- **Every record page reads in the same order** on main: readings, then the
  call with the thread under it, then what needs a contact, then the pairs.
  That is the order here, with the thread as its own zone between the
  readings and the needs, because the 360 opens on what happened last.
- **The contact's strip on main is four readings**: Whose move (Yours / Theirs /
  Gone quiet / Never spoken, with "last from them"), Open promises (a count,
  late in red), Deals they decide (with "Open deals"), Next meeting (with
  "Open meetings"). Consent left the strip and lives in the panel's channel
  rows. The target strip (the contact record above) is seven and brings
  Consent back; the four on main are what it grows from. The call is always drawn, "Not shown" when withheld. Up to three open
  tasks join the needs list with the assignee as the mark.
- **The deal's readings moved onto the overview**, and Momentum opens "See the
  ledger". The stage stepper follows them. Tags are on the deal. The briefing
  splits into the call (standing, the because-sentence, the signals, the
  thread), the found move as a row of the needs list (the "What to do next"
  caption is gone; the row's own chrome says it), and "What this deal is" as
  its own block with "Written by" and "Write it again". Offers and the
  committee sit side by side on main; here they are Commercial and the
  committee zone.
- **The lead has a standing**: Your move (with the first-response due or
  overdue), Their move, In motion, Qualified, Closed, each saying what it
  rests on. The Source reading is gone; First response reads Answered / Still
  owed / due by. The score card and "What you know about this lead" moved
  out of the rail into the reading, so the rail is Details and Owner. The
  needs list carries "Answer {name} · First response owed" when one is owed,
  and the next task. A lead carries no agent suggestions; the "Ready to
  qualify" row on this page is a proposal, derived from the same evidence the
  qualify dialog derives its reason from.
- **The Brief draws five fixed readings**: Urgent, Meetings today, Leads owed
  a reply, Pipeline · Q3, Decisions waiting. One dense row — label, figure,
  basis fragment — where the whole cell is the door into the lane its figure
  counted, figures are in neutral ink unless the reading counts something
  breaching, and a source read to its limit marks its figures `8+` rather than
  adding a sentence under the row.
- **Tags** are a panel on company, contact and deal (four visible, "+N more",
  a split pill that opens the tag or its menu, "Add tag" opens a picker that
  cannot create a word), a Tags column on the contacts, companies and deals
  lists (two visible, not sortable), a tag on the board card, and a "Tags ·
  Any tag" filter chip. Leads are not tagged.
- **The email verb is an outlined button on every record**, in the same
  place, with the same word. The head's verbs are outlined; the filled verb
  is inside the needs list and it is the move the call names,
  inside the needs list. Qualify stays filled on a lead, because it is the
  lead's whole point.
- **The timeline thread has a failed state** ("The thread could not be read",
  with retry), states when earlier pages were cut ("More conversations before
  this"), and the silence gap is pluralised.


## 8. Components, restated in this language

The primitives in the catalog keep their names and props; this is how each one
looks now.

| Primitive | Treatment |
|---|---|
| `Button` primary | `--accent` fill, white text, 10px radius, 36px, weight 500. One per view. |
| `Button` secondary | `--pane` fill, `--line2` outline, `--ink` text, flat. Icon-only at 36×36 for the overflow. |
| `Button` ghost | No outline, `--ink2` text; hover `--bg3`. |
| `Button` danger | Outlined in `--bad`; fills only inside a `ConfirmModal`. |
| `TextInput` / `Select` | White, `--line2` outline, 36px, 10px radius; focus is a 2px emerald ring. Label above at 12px 500; helper below at 12px in `--ink3`. |
| `Badge` | One size (20px: an 18px line inside a 1px edge, 12px 500, full radius), never capitals. `soft` by default: the tone's tint behind the word in the tone's ink, edged in a hairline of the same tone — the record's standing badges beside its name, a status in a row. `primary` is the solid fill with its edge left clear, for a count and the one status that must not be missed. A glyph, when there is one, sits left of the word; the agent's badge always carries the sparkles. |
| `Chip` | A fact a reader can act on rather than a status: a pill on the elevated ground with a neutral hairline and a glyph, a link when the fact has somewhere to go, and a hover that says so. A badge is the other thing — a tinted status, edged in its tone, that nobody presses. |
| `Panel` | Becomes a **zone pane**: `--pane` with a `--paneEdge` and a 20px corner; inside, a display-face title with its count and its verb, a hairline, rows. `PanelPlate` (the inset well) becomes a row on `--bg3`. |
| `StatCard` | The **reading card**: `--pane` with the hairline and a 20px corner, 138px tall; the eyebrow as its label with the basis as dotted-underlined words at the label's end, the figure at 26px in the display face, tabular (down to 20px where five share a narrow row), the basis at 12.5px at the foot. |
| `FieldGrid` / `FieldRow` | The attribute row in the details panel: a 96px label with its glyph in `--ink3`, the value in `--ink`, "Add …" in `--ink4` when empty, the dotted evidence underline when a machine read it. |
| `ListTable` / `DataTable` | Headers at 11.5px 500 in `--ink3`; 44px rows; hairlines; figures right-aligned; the selected row on `--accentBg`. Edge to edge inside its zone. |
| `RecordTabs` | Quiet: no rule under the strip; the open tab in `--ink` with a 2px accent underline; counts at 11px in `--ink4`; the Details control at the right end. |
| `SegmentedControl` | `--bg3` track, white pressed segment on the resting shadow, 12px. |
| `Modal` | White, 8px radius, `--shadow-pop`, a scrim of `rgba(24,24,27,.4)`. The drawer form slides from the right with the same surface. |
| `EmptyState` | Left-aligned in the zone it belongs to, `--ink3`, one sentence and one verb. |
| `Callout` | An alert's anatomy on the pane's ground, and ONE of them: the tone's own glyph on the heading's first line, the heading beside it in the tone's ink, an optional body in ordinary ink under it, the verbs right-aligned at the end of that line and the dismiss after them. The heading is mandatory and everything else optional, so a bare heading is the commonest callout in the product; below 640px the verbs drop under the body. Five tones — `info`, `accent` (the emphatic ask, in the brand accent, and never in indigo, which claims a machine wrote what follows), `warn`, `danger`, `success` — and tone reaches the glyph and the heading and nothing else, never a filled coloured box. The dot it replaced was dots differing only in hue; the shape is what says which tone it is. No `className`: a notice that wanted its own edge was a second callout wearing this one's name. |
| `StagingCard` / `DecisionCard` | The agent's row: `--aiBg`, 14px radius, the indigo mark on its own tile (no label beside it: the tile is the claim), the verdict word at 15px 600 (amber when warn, green when calm), the sentence in `--ink`, "What this rests on · n sources" in `--aiText`, the agent's verb in `--ai`. A staged change is a row with a dashed `--aiLine` edge, Accept and Dismiss. |
| `Kbd` | 10.5px in `--ink4` with a `--line` outline, 4px radius. On the search field and the ask field. |
| `Spine` | The product's own spine: per stop a date, a 2px accent rule with a 9px dot at its start, a title, a detail; the gap stop at 1.5× width as a 26px amber day count over a dashed amber rule with no dot; today as a 2px black bar with TODAY and the date under it; dotted grey and hollow dots ahead of it. |
| `RecordTimeline` | The rail: a 76px date column in tabular figures, a 1px full-height rail with a mark per kind (solid, hollow, indigo, dashed indigo, circled glyph for a thread), the kind in uppercase, direction words, title, the message text clamped to three lines, a meta line; threads as a card on the rail. |
| `RelationshipMap` | Three lanes at 184/200/184 with 72px gutters; rounded nodes by kind, a dashed amber gap node; route edges banded strong / developing / cold, membership as hairline; selection lights the route and fades the rest; a 280px panel with the best route and the one write. |
| `EvidenceMark` / `Citations` | A **source chip**: `--bg3`, 11.5px, a note glyph and a short label ("email · 1 Sep"). On hover or focus it opens a popover with the quote it rests on, in a left-ruled indigo block, and the origin line under it. Every claim the agent makes carries them; they are how a reader checks the 360 without leaving it. |
| `Stepper` | Steps as 24px pills on one line with a `›` between: done on `--accentBg`, the current one filled `--accent`, the rest outlined; the rule ("A terminal stage asks first") at the right in `--ink4`. |

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

- Does not put a pane inside a pane. A zone is one pane; inside it a title, a
  hairline and rows.
- Does not cast a shadow of its own, a glow or a gradient. The resting layer
  is `--shadow-rest` and it comes from the token, never from a rule that spells
  its own; the two corner glows on the ground are the only other light; the
  only tinted thing inside a pane is the agent's row.
- Does not say a fact twice. One home per fact; the rest are links to it.
- Does not fill a button, a row or a surface with emerald except the one
  primary verb and a selected row's wash.
- Does not tint anything indigo that a contact wrote.
- Does not set a number in the proportional face.
- Does not centre a title, a section or a table.
- Does not use uppercase outside an eyebrow, or weight 700 outside the rare
  word that must outrank a heading.
- Does not add a fourth typeface, an uppercase label, an emoji glyph, a
  gradient hero, or a shadow under a shadow.
- Does not invent a component that the catalog already names. Grep first.

## 11. Adopting it

The step-by-step plan, PR by PR, with the gates each step must keep green,
the five record pages zone by zone with every state they carry, and the
features a restyle must not lose, is
[docs/how-to/adopt-the-design.md](docs/how-to/adopt-the-design.md). In one
line: tokens and type first, then atoms, then the record primitives and the
shell, then company as the reference page with its e2e rewritten, then
contact, deal, lead and project, then the sub pages and the maps, then the
rest one screen at a time.

## 12. Checklist for a new screen

- [ ] The display face on the name, the zone titles, the verdict and a
      reading's figure; nothing else uses it.
- [ ] Every figure is tabular (`t-num`), right-aligned in a column; mono only
      on code.
- [ ] Every zone is one pane with a title, a hairline and rows; the only
      shadow at rest is `--shadow-rest`, and no rule spells one by hand.
- [ ] One emerald-filled control in view.
- [ ] Anything an agent wrote is a row on `--aiLight` carrying the indigo mark,
      and a row that ASKS for something says "Margince suggests" — never over
      the answer that nothing needs doing, which is nobody's suggestion; a
      staged change carries a `--aiMed` dashed edge until accepted.
- [ ] A withheld section says "Hidden from you"; an empty one says what to do.
- [ ] Checked in both themes and with the sidebar and the details panel folded.
- [ ] The page carries every section the current screen renders (§7), or says
      which it dropped and why.
- [ ] Every control came from `frontend/src/design-system/`; the catalog table
      names anything new.
