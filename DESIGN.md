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
   page answers the rep's first five questions above the fold. Dense is not
   the same as cramped: the density comes from saying each thing once, not
   from shaving the space between rows.
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

1. **A lit ground, and one pane per zone.** The page is a pale green paper lit
   from two corners, an emerald glow behind the sidebar and an indigo one
   behind the far edge. Each zone of a record is one white pane on it, with a
   hairline edge and a 14px corner, and inside a pane there is only ever a
   title, a rule and rows. Dark is the same room with the lights down.
2. **Everything is a list.** A record's attributes are a list of label and
   value in a panel on the left that folds. What happened is a list. What
   needs you is a list. Money is a list. Because every list is the same list,
   a rep never learns a second layout.
3. **One display face carries identity.** The name of a record, a zone's
   title and the verdict the agent speaks are set in the display face; every
   figure is set in the mono face, tabular; everything else is one quiet sans.
4. **Buttons are flat and there is one filled one.** No gradient, no rim, no
   glow: a filled emerald verb for the move the page names, white outlines for
   the rest.
5. **Colour means something.** Emerald is the one filled verb, a link and the
   light behind the sidebar. Indigo is a tinted row that says an agent wrote
   it, and the light behind the far edge. Green, amber and red are a dot
   before a word. Nothing is coloured to look nice.

Rules 1 and 5 are already held by gates; this file adds 2, 3 and 4.

## 3. Colour

The semantic split is unchanged and remains pinned by `tokens.test.ts`:
`--accent` (emerald) is brand and primary action, `--ai*` (indigo) is agent
provenance, `--success` / `--warn` / `--danger` are status. What changes is the
ground, which is lit, and the surface, which is one translucent pane per zone.

### Light (the default)

| Token | Value | Role |
|---|---|---|
| `--bg` | `#f1f5f2` | The paper the page is read on. |
| `--glowA` / `--glowB` | `rgba(24,190,120,.16)` / `rgba(91,97,214,.12)` | The emerald light at the top-left corner behind the sidebar; the indigo light at the bottom-right. Radials of 900×620, the only decoration on the page. |
| `--pane` / `--paneEdge` | `rgba(255,255,255,.72)` + `blur(12px)` / `rgba(16,26,21,.08)` | A zone, the details panel, a board card, a control at rest. |
| `--bg2` / `--bg3` | `rgba(255,255,255,.55)` / `rgba(16,26,21,.05)` | The sidebar (glass over the glow, `blur(20px)`); a pill, a keycap, the active sidebar row. |
| `--line` / `--line2` | `rgba(16,26,21,.08)` / `.16` | The hairline between rows; a control's outline, the spine's axis. |
| `--ink` / `--ink2` / `--ink3` / `--ink4` | `#101a15` / `#33403a` / `#66736c` / `#9aa59f` | Names and values / body / labels and meta / placeholders and dates. |
| `--accent` / `--accentText` / `--accentBg` | `#0b7a53` / `#0a6f4b` / `#e8f3ee` | The one filled verb; a link; a selected row or a done stage. |
| `--ai` / `--aiText` / `--aiBg` / `--aiLine` | `#5b61d6` / `#3f45b0` / `rgba(91,97,214,.09)` / `.35` | The agent's filled verb; its label; the tinted row; the dashed edge of a staged row. |
| `--ok` / `--warn` / `--bad` | `#15803d` / `#a16207` / `#b91c1c` | Status, as a dot before a word, or as a pill for the one status that must not be missed. |

### Dark

The same room with the lights down: `--bg #0a100e`, the glows brighter
(`.22` / `.24`) because they are the only light, panes at
`rgba(255,255,255,.045)` with a `.09` edge, ink from `#eef3ef` down to
`#5c6862`, the accent lifted to `#2bb673` with dark ink on it, the indigo text
lifted to `#b3b7f5`. The three-state theme pattern (`:root`,
`prefers-color-scheme` guarded by `:not([data-theme="light"])`,
`[data-theme="dark"]`) is how they switch.

### How colour is spent

- **The two glows are the chrome.** The sidebar is glass over the emerald
  light; there is no coloured bar anywhere.
- **The page carries at most ONE filled control in view**, in emerald: the
  move the page names. Every other button is a pane with an outline.
- **Indigo is a fact, not a mood.** The agent's read is a row on `--aiBg`; a
  staged change is a row with a dashed `--aiLine` edge until a person accepts
  it. Nothing else is indigo.
- **Status is a dot before it is a pill.**
- **Avatars are neutral.** `--bg3` with `--ink2` initials; a record is told
  apart by its name.

## 4. Type

Three families, which is the ceiling `check-font-lock.sh` holds.

| Role | Family | Where |
|---|---|---|
| Display | **Bricolage Grotesque** 600 | A record's name at 24px (`-0.025em`), the home greeting at 30px, a zone's title at 16px, the agent's verdict word at 19px, a reading's word at 17px. |
| Body and UI | **Geist** | 13px 400 for everything, 500 for a row's lead and a control, 12px in `--ink3` for meta and labels. Prose at 13.5px, 72ch. |
| Figures | **Geist Mono** 500, tabular | A reading's figure at 22px (`-0.03em`), and every amount, count, date and identifier in a row or a cell. |

- **Weight 600 is for the display face.** Everything that must stand out in a
  row does it at 500 in the body face.
- **No uppercase anywhere.** Labels are sentence case in a lighter ink.

## 5. Space, shape, depth

- **4px base**, the existing `--space-*` ladder, spent generously. The page
  gutter is 32px; panes sit 24px apart; a pane has 20px above and below its
  content and 26px at its sides; a zone title has 12px under it.
- **Rows breathe.** A list row is 13px above and below its content (about
  48px tall with two lines); a table row 44px; a sidebar row 34px; a control
  32px (28px in a row); the attribute rows in the details panel 32px. The
  test for "gedrungen": if two rows could be mistaken for one, the row is too
  short.
- **Type at rest is 13.5px on 1.55**, so a row's second line does not touch
  its first; prose is 14px on 1.65 at 72 characters.
- **Radii by role**: 16px for a pane and the details panel, 12px for a board
  card and the agent's row, 6px for a control and a pill, full for a monogram.
- **Depth is light, not shadow.** A pane is translucent over the lit ground
  with a hairline edge; that is its whole elevation. Nothing at rest casts a
  shadow, glows or has a gradient. A popover, menu or drawer takes the one
  shadow; a board card takes a faint one on hover, because it is being picked
  up.
- **No pane inside a pane.** A zone is one pane; the things in it are a
  title, a rule and rows. The only enclosed shapes inside a pane are the
  agent's tinted row and a staged row's dashed edge.

## 6. The shell

```
┌────────┬───────────────────────────────────────────────────────────────┐
│ sidebar│ crumb        [ Search or run a command  ⌘K ]                me │
│ (glass ├───────────────────────────────────────────────────────────────┤
│  over  │ mark  NAME (display)  badges                  verbs, right     │
│  the   │       facts line                                               │
│  glow) │ tabs ──────────────────────────────────────────── [Details] │
│ Brief  │ ┌ THE 360 ─────────────────────────────┐ ┌ details (288) ────┐│
│ RECORDS│ │ ● Margince read this record · {name} ││ │ Ask               ││
│ …      │ │ VERDICT  because · sources (hover)    ││ │ Details           ││
│ WORK   │ │ readings ┊ readings ┊ readings        ││ │ People / Seats    ││
│ …      │ │ spine · · · gap · today · ahead       ││ │ Tags / Room / Docs││
│ ● agent│ │ the thread, newest first              ││ └───────────────────┘│
│        │ └───────────────────────────────────────┘│                      │
│        │ ┌ What needs you ┐ ┌ Commercial ┐ ┌ About ┐                     │
└────────┴───────────────────────────────────────────────────────────────┘
```

- **Sidebar**: 224px expanded, 52px collapsed, and collapsed is the default.
  Glass over the emerald glow, a hairline on its right. Workspace name and the
  fold at the top, then the product's own ten rows in its own order: Brief;
  Records — Contacts, Companies, Leads, Filters & views; Work — Worklist,
  Pipeline, Projects; Intelligence — Reports, Ask Margince. No badges: the
  queues that used to badge (approvals, tasks) are lanes of the Worklist, which
  reports its counts on the page. Settings is not a row; it opens from the
  account menu and publishes its own second level (You / Admin settings) as a
  210px column beside the rail. The agent's orb stays at the foot.
- **Top bar**: 48px, glass, a hairline under it. The breadcrumb on the left,
  the command field in the middle (`⌘K`), the reader's monogram on the right.
  It is the application's one bar and every screen has it. Nothing that
  belongs to a record sits in it: the Details toggle is in the page.
- **Record head**: a 40px mark, the name in the display face with its badges
  on the same line, one line of facts under it, the verbs on the right — the
  outlined verbs, the one filled verb the call names, then the **more** button
  (an outlined 40px square with the ellipsis) for everything else. The
  **Details** control sits at the right end of the tab row, in the page.
- **Details panel**: 300px on the right of the reading, one pane, closed
  until the Details control at the end of the tab row opens it. It holds Ask, the attributes as label and
  value rows (with the evidence underline on a machine-read value and a
  lighter "Add …" on an empty one), and the short lists that describe the
  record (people, seats, tags, the Deal Room, documents). It is where the
  current product keeps its context column, so a rep's hand does not move.
- **The glows**: the emerald light at the top-left is faint (`.06` in light,
  `.10` in dark); the indigo one at the bottom-right a step stronger. They are
  atmosphere, not a feature, and the eye should not find them.

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

### Glance, then depth

The overview that breathes is the one that shows less. A record's overview
has two depths, and the shallow one is the default.

**The glance** is the whole first screen. It keeps the air of the sparse
version and brings the product's own features back in reduced form — each
one present, none at full depth:

- The name at 44px in the display face, one line of facts with the live dot
  ("In conversation · Hamburg · Freight forwarding · 240 employees · Owner"),
  and three verbs. No badges beyond the standing, no second line.
- **A quiet tab row** under the head — no pane, no rule, a 2px accent under
  the current tab — so every sub page (History, People, Deals, Tasks,
  Finance, Documents, Profile, Partner) is one click away. The links inside
  the panes ("All deals", "Full history") open the same tabs.
- **Five readings as roomy cards**, 160px tall, an uppercase label, one
  figure at 30px in the mono face, one line of basis at the foot, and empty
  space between them. When the details panel is open they shrink to 21px
  figures and never wrap.
- **The 360 as the first pane**, full width, on the same hairline as every
  other pane: the record's name and "· 360" as the pane title, the agent's word
  at 40px, one sentence beside it whose claims are hoverable sources (the
  chip sits inline on the agent's tint; hover opens what it rests on), the
  three rated dimensions, then the spine. The thread is folded — "Read the
  thread · 5" opens the five most recent exchanges inside the pane; "Full
  history" is the History tab.
- **Left column**: What needs you, one list. Your move leads it as the
  agent's row (a headline in the display face, one sentence, its sources,
  the filled verb that does it); the agent's suggestion is the second row
  of the same list, not a card beside it; then the tasks, the call as the
  task of preparing it. Below it Deals, three rows.
- **Right column**: the ask field, About (the lead sentence, one paragraph,
  its sources, "Profile" for the rest), People as chips with a "+N".
- **The details panel is hidden until asked**, and the Details control in
  the top bar opens it on the right at 300px with the fields, the people and
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
| Filled verb | New deal | Reply on her thread | Confirm the rate | Qualify |
| Readings | Open pipeline · Invoiced · Conversation · Last touch · Next | Whose move · Open promises · Deals she decides · Next meeting · She answers in | The money · The close · Stage · The people · Momentum | Company · Score · First response · Next · Your move |
| 360 word | Good | Promise overdue | Live | In motion |
| In the 360 | the spine | the spine | the stage stepper, then the spine | the ladder, then the spine |
| Needs-list lead | Your move | The move the call names | Margince found this, then the staged change | Ready to qualify (no agent tint: a lead carries no suggestions) |
| Second left pane | Deals | The deal she decides, with the room | The buying committee, with the cover gap | The score as factors |
| Right column | Ask · About · People | Ask · Understanding her · Around her | Ask · What this deal is · Offers · Deal Room | Ask · If she is qualified · What she asked for |
| Tabs | Overview · History · People · Deals · Tasks · Finance · Documents · Profile · Partner | Overview · History · Network · Deals · Meetings · Data & tools · Documents | Overview · History · Documents | Overview · History |

The test: a rep back from a week away reads the glance in ten seconds and
knows where the account stands, what happened last, what they owe, and what
the agent proposes. Whatever those ten seconds do not need goes to the depth.


### The order of the zones

1. **Who.** Identity, the standing badges, one line of facts, the verbs.
2. **The 360.** One element, and the one place the page is allowed to look
   like a feature: where the record stands and what happened, together. It
   opens with the indigo tile and "Margince read this record · {name} · 360",
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
History (the filter strip, then the **rail timeline** below), People (the
coverage band, **the committee map**, the roster as a table with list, board
and map cuts),
Deals (the commercial band, the deals table with won and lost), Tasks (tick,
snooze, open), Finance (the readings, the invoice table, the provider), Documents
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
   created · captured by), and the verbs: Write email (filled), Log activity,
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

Context column: Ask (with three prepared questions), People (three, with who is
in touch from our side), Details (nine fields including mail capture), Tags.

**What merged, moved or went.** The call card merged into Health. Next steps,
the agent's suggestions, the manual moves and the tasks merged into one list.
The record spine merged into the timeline, and the timeline moved up to third.
Work in flight, the commercial figures and projects merged into Commercial. The
dossier, signals and growth fit merged into About. Active deals left the rail;
key people became the rail's People; Ask moved from the foot of the page to
the top of the column. Nothing the page could say is gone; each thing is said
once.

### The contact record

1. **Who.** Round monogram, name, the buying-role badge, title and employer,
   the glyph line (email, phone, city, LinkedIn, owner), and the verbs: Write
   email (the transport, filled), Call, Add task, more.
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
   people (engaged of total, champion named, single-threaded), The momentum
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
   the FX basis in the zone head, then forecast, wait-until and the custom
   fields as rows.
6. **The buying committee.** The map (our circle, their seats, threads only to
   the engaged, a dashed ghost per coverage gap), then the stakeholder table
   with role, person, talking, dates and edit. Add stakeholder in the zone
   head.

Context column: Ask, Deal Room (state, invited, signed in, last seen, open),
Who is on this deal (seats with Engaged / No two-way contact and who of ours
carries it), Related evidence, Documents.

**What merged, moved or went.** The stage stepper moved beside the readings.
Deal360, the approvals queue and the email box merged into the needs list.
Offers, the FX line and custom fields merged into Commercial. The committee map,
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

The sidebar collapses to a 52px icon rail and that is the default. Labels
return on hover as tooltips, the agent's orb stays at the foot. The details
panel on the right of the reading opens from the Details control at the end
of the tab row, and starts closed.

### The timeline, as the tool draws it

History is a **rail**, not a list of rows. Each entry is a grid of four: the
date in the mono face, right-aligned in a 76px column with the time under it
in `--ink4`; a 20px rail column carrying a 1px `--line2` line that runs the
full height so consecutive entries join, with a mark on it; the body; the
verb slot. The mark says what kind of thing happened: a solid 8px dot for an
exchange, a hollow one for a field change, an indigo one for a change the
agent made, a dashed indigo ring for a staged change, and a 24px circled
glyph for a thread. The body opens with the kind in 10.5px uppercase, the
direction words ("they wrote", "we sent", "both sides") and the people; then
the title at 13.5px 600; then **the text of the message itself**, clamped to
three lines, because a timeline of subject lines is a list of things you
cannot read; then a meta line (the summary's author, the attachment, Restore).
A thread is one entry: its mark on the rail, a card on the body side with the
count and the people, and the messages inside it newest first, each with its
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
the display face, its rule is dashed amber, and it has no dot, because
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
by kind (a colleague 40, a person 60, an organization 48, a deal 44, a gap
60), 8px between nodes, 20px between lanes, a lane heading in 10.5px
uppercase. Nodes are rounded boxes on `--pane` with the name at 13px 600 and
a sublabel; a colleague on `--bg`, an organization with a 2px `--ink3` edge,
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
target, their company; on the company's People tab they are our side, the
account with its deals, and their people **by buying role** in the product's
order (champion, economic buyer, influencer, blocker, user), with a gap node
where a critical role is unheld. The deal keeps its small decorative
committee picture beside the seat rows: our circle, their seats, 2px accent
threads only to the engaged, hollow for the quiet, a dashed ghost per
coverage gap.

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
  call with the thread under it, then what needs a person, then the pairs.
  That is the order here, with the thread as its own zone between the
  readings and the needs, because the 360 opens on what happened last.
- **The contact's strip is four readings**: Whose move (Yours / Theirs /
  Gone quiet / Never spoken, with "last from them"), Open promises (a count,
  late in red), Deals they decide (with "Open deals"), Next meeting (with
  "Open meetings"). Consent left the strip and lives in the panel's channel
  rows. The call is always drawn, "Not shown" when withheld. Up to three open
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
- **Home draws five fixed readings**: Customer waiting, Meetings ahead,
  Promises due, Lead response, Quota pace. Two of them say what they cannot
  answer ("promises are not tracked yet", "no target is set") rather than
  showing a zero, and the floor line under the row says when a source was read
  to its limit. The old four cards are gone.
- **Tags** are a panel on company, contact and deal (four visible, "+N more",
  a split pill that opens the tag or its menu, "Add tag" opens a picker that
  cannot create a word), a Tags column on the people, companies and deals
  lists (two visible, not sortable), a tag on the board card, and a "Tags ·
  Any tag" filter chip. Leads are not tagged.
- **The email verb is an outlined button on every record**, in the same
  place, with the same word. The filled verb is the move the call names,
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
| `Button` primary | `--accent` fill, white text, 6px radius, 30px, weight 500. One per view. |
| `Button` secondary | `--pane` fill, `--line2` outline, `--ink` text, flat. Icon-only at 30×30 for the overflow. |
| `Button` ghost | No outline, `--ink2` text; hover `--bg3`. |
| `Button` danger | Outlined in `--bad`; fills only inside a `ConfirmModal`. |
| `TextInput` / `Select` | White, `--line2` outline, 30px, 6px radius; focus is a 2px emerald ring. Label above at 12px 500; helper below at 12px in `--ink3`. |
| `Badge` | `quiet` by default: a 6px dot and a word. The pill (`--bg3`, 20px, 11.5px 500) is for the one status that must not be missed and for the record's standing badges beside its name. |
| `Chip` | The same pill. There is one pill. |
| `Panel` | Becomes a **zone pane**: `--pane` with a `--paneEdge` and a 14px corner; inside, a display-face title with its count and its verb, a hairline, rows. `PanelPlate` (the inset well) becomes a row on `--bg3`. |
| `StatCard` | A **cell** inside the standing pane: label at 11.5px, figure at 22px mono, basis at 12px; cells separated by a vertical hairline, not by boxes. |
| `FieldGrid` / `FieldRow` | The attribute row in the details panel: a 96px label with its glyph in `--ink3`, the value in `--ink`, "Add …" in `--ink4` when empty, the dotted evidence underline when a machine read it. |
| `ListTable` / `DataTable` | Headers at 11.5px 500 in `--ink3`; 38px rows; hairlines; figures right-aligned; the selected row on `--accentBg`. Edge to edge inside its zone. |
| `RecordTabs` | Text tabs on a rule; the open one in `--ink` with a 2px ink underline; counts at 11px in `--ink4`. |
| `SegmentedControl` | `--bg3` track, white pressed segment with a faint shadow, 12px. |
| `Modal` | White, 8px radius, the one shadow, a scrim of `rgba(24,24,27,.4)`. The drawer form slides from the right with the same surface. |
| `EmptyState` | Left-aligned in the zone it belongs to, `--ink3`, one sentence and one verb. |
| `Callout` | A row on `--bg2` with a dot in the tone's colour before its first words. Never a filled coloured box. |
| `StagingCard` / `DecisionCard` | The agent's row: `--aiBg`, 8px radius, the "Margince read this record" label at 11.5px 500 in `--aiText`, the verdict word at 15px 600 (amber when warn, green when calm), the sentence in `--ink`, "What this rests on · n sources" in `--aiText`, the agent's verb in `--ai`. A staged change is a row with a dashed `--aiLine` edge, Accept and Dismiss. |
| `Kbd` | 10.5px in `--ink4` with a `--line` outline, 4px radius. On the search field and the ask field. |
| `Spine` | The product's own spine: per stop a date, a 2px accent rule with a 9px dot at its start, a title, a detail; the gap stop at 1.5× width as a 26px amber day count over a dashed amber rule with no dot; today as a 2px black bar with TODAY and the date under it; dotted grey and hollow dots ahead of it. |
| `RecordTimeline` | The rail: a 76px mono date column, a 1px full-height rail with a mark per kind (solid, hollow, indigo, dashed indigo, circled glyph for a thread), the kind in uppercase, direction words, title, the message text clamped to three lines, a meta line; threads as a card on the rail. |
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
- Does not cast a shadow, a glow or a gradient at rest. The two corner glows
  on the ground are the only light; a button is a flat shape; the only tinted
  thing inside a pane is the agent's row.
- Does not say a fact twice. One home per fact; the rest are links to it.
- Does not fill a button, a row or a surface with emerald except the one
  primary verb and a selected row's wash.
- Does not tint anything indigo that a person wrote.
- Does not set a number in the proportional face.
- Does not centre a title, a section or a table.
- Does not use uppercase outside an eyebrow, or weight 700 outside the rare
  word that must outrank a heading.
- Does not add a fourth typeface, an uppercase label, an emoji glyph, a
  gradient hero, or a shadow under a shadow.
- Does not invent a component that the catalog already names. Grep first.

## 11. Adopting it

The tokens above land in `frontend/src/design-system/tokens.css`, and
`tokens.test.ts` pins values, so a repaint is a change to both in one commit —
the test is what proves the canon moved rather than one screen. The order of
work that gets the most visible change for the least churn:

1. **The shell.** The two glows on the ground, the sidebar as glass with the
   fold, the top strip to a 44px row with the breadcrumb and the Details
   toggle. One file
   each (`shell.css`, `topbar.css`), and the whole product stops looking like a
   template.
2. **Type.** The three families and the scale in §4. `check-font-lock.sh`
   holds the count at three.
3. **Zones.** Restyle `Panel` to one translucent pane per zone with a
   display-face title, a hairline and rows. Every record page and settings
   page changes with it.
4. **Figures.** The mono face, tabular, on every figure through `StatCard`
   cells, `ListTable` cells and `FieldGrid` values.
5. **Grounds.** Move the paper and surface values; re-check every `color-mix()`
   derivation in both themes with Storybook's theme control.

Each step is one PR and a Storybook pass in both themes. Verify contrast on
the new grounds with the existing axe suite before the values are pinned.

## 12. Checklist for a new screen

- [ ] The display face on the name, the zone titles and the verdict; nothing
      else uses it.
- [ ] Every figure is mono and tabular, right-aligned in a column.
- [ ] Every zone is one pane with a title, a hairline and rows; nothing casts
      a shadow at rest.
- [ ] One emerald-filled control in view.
- [ ] Anything an agent wrote is a row on `--aiBg` labelled "Margince read
      this record"; a staged change has a dashed edge until accepted.
- [ ] A withheld section says "Hidden from you"; an empty one says what to do.
- [ ] Checked in both themes and with the sidebar and the details panel folded.
- [ ] The page carries every section the current screen renders (§7), or says
      which it dropped and why.
- [ ] Every control came from `frontend/src/design-system/`; the catalog table
      names anything new.
