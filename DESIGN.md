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

1. **One ground, one line, one row.** A white page, a one-pixel hairline, and
   a row. There is no glass, no glow, no gradient, no shadow at rest, and no
   card unless the thing is genuinely a separate object on a board. Dark mode
   is the same page in a dark neutral, not a second design.
2. **Everything is a list.** A record's attributes are a list of label and
   value. What happened is a list. What needs you is a list. Money is a list.
   Because every list is the same list, a rep never learns a second layout.
3. **One face, three sizes, two weights.** Inter with tabular figures at 13px
   for everything, 12px for meta, 18px for a record's name; 400 and 500, with
   600 only on a name and a zone title. Figures are the same face, aligned.
4. **The record's details live in a left panel that folds.** The reading is on
   the right: the standing, the timeline, the needs, the money, the rest.
5. **Colour means something.** Emerald is the one filled verb and a link.
   Indigo is a tinted row that says an agent wrote it. Green, amber and red are
   a dot before a word. Nothing is coloured to look nice.

Rules 1 and 5 are already held by gates; this file adds 2, 3 and 4.

## 3. Colour

The semantic split is unchanged and remains pinned by `tokens.test.ts`:
`--accent` (emerald) is brand and primary action, `--ai*` (indigo) is agent
provenance, `--success` / `--warn` / `--danger` are status. What changes is
that the grounds go neutral and the surfaces go away.

### Light (the default)

| Token | Value | Role |
|---|---|---|
| `--bg` | `#ffffff` | The page. |
| `--bg2` | `#fafafa` | The sidebar, a hovered table row, the ask field. |
| `--bg3` | `#f4f4f5` | A pill, a keycap, the active sidebar row, a pressed segment. |
| `--line` | `#e9e9eb` | The one hairline: between rows, under headers, beside the panels. |
| `--line2` | `#d9d9dc` | A control's outline (a button, an input), the spine's axis. |
| `--ink` / `--ink2` / `--ink3` / `--ink4` | `#18181b` / `#3f3f46` / `#71717a` / `#a1a1aa` | Names and values / body / labels and meta / placeholders and dates. |
| `--accent` / `--accentText` / `--accentBg` | `#0b7a53` / `#0a6f4b` / `#e8f3ee` | The one filled verb; a link; a selected row or a done stage. |
| `--ai` / `--aiText` / `--aiBg` / `--aiLine` | `#5b61d6` / `#3f45b0` / `#eef0fb` / `#c9ccf3` | The agent's filled verb; its label; the tinted row; the dashed edge of a staged row. |
| `--ok` / `--warn` / `--bad` | `#15803d` / `#a16207` / `#b91c1c` | Status, as a dot before a word, or as a pill for the one status that must not be missed. |

### Dark

The same page in a dark neutral: `--bg #131316`, `--bg2 #18181b`, `--bg3
#1f1f23`, `--line #26262b`, `--line2 #34343a`, ink from `#f4f4f5` down to
`#5f5f68`; the accent lifts to `#2bb673` with dark ink on it; the indigo text
lifts to `#b3b7f5` on `#1d1e33`; the status colours lift one step. The
three-state theme pattern (`:root`, `prefers-color-scheme` guarded by
`:not([data-theme="light"])`, `[data-theme="dark"]`) is how they switch.

### How colour is spent

- **The page carries at most ONE filled control in view**, in emerald: the
  primary verb. Every other button is a white outline.
- **The sidebar is one step off the page** (`--bg2`), divided by the hairline.
  It carries no colour of its own.
- **Indigo is a fact, not a mood.** The agent's read is a row on `--aiBg`; a
  staged change is a row with a dashed `--aiLine` edge until a person accepts
  it. Nothing else is indigo.
- **Status is a dot before it is a pill.** A pill is reserved for a state the
  reader must not miss (Overdue, Unanswered, Won).
- **Monogram tones went.** Avatars are `--bg3` with `--ink2` initials; a
  record is told apart by its name, not by a colour.

## 4. Type

One family, which is well under the ceiling `check-font-lock.sh` holds.

| Role | Setting |
|---|---|
| Everything | **Inter**, `font-feature-settings: "cv11", "ss01", "tnum"` (the single-storey a, the open digits, tabular figures). |
| A record's name | 18px, 600, `-0.01em`. |
| A zone title, a list page title | 12.5px 600 and 18px 600. |
| A reading's figure | 18px 600, tabular; a reading's word 15px 600. |
| A row's lead | 13px 500. |
| Body | 13px 400, line-height 1.5; prose at 13.5px, 72ch. |
| Labels, meta, dates | 12px and 11.5px 400 in `--ink3` / `--ink4`. |

- **A figure is the same face as the word beside it**, aligned by `tnum`. A
  second mono face was one more thing to learn and went.
- **No uppercase anywhere.** Labels are sentence case in a lighter ink.
- **Weight 600 is for a name and a title.** Everything that must stand out in
  a row does it at 500.

## 5. Space, shape, depth

- **4px base**, the existing `--space-*` ladder. Rows are 36–40px; a table row
  38px; a sidebar row 30px; a control 30px (26px in a row).
- **Radii**: 6px for a control, a pill and a card; 8px for the agent's row and
  a board card; full for a monogram.
- **Depth**: none at rest. A hairline separates; a one-step ground (`--bg2`)
  hosts; a popover, menu or drawer takes the one shadow. A board card has a
  hairline and takes a faint shadow on hover, because it is being picked up.
- **Rows, not boxes.** A zone is a title with a hairline under it and rows
  under that. The only enclosed shapes on a record page are the agent's tinted
  row and a staged row's dashed edge.

## 6. The shell

```
┌────────┬───────────────────────────────────────────────────────────────┐
│ sidebar│ crumb                                            Details · me  │
│ (--bg2)├───────────────────────────────────────────────────────────────┤
│ Search │ mark  NAME  badges                          verbs, right       │
│ Home   │       facts line                                               │
│ RECORDS│ tabs ───────────────────────────────────────────────────────── │
│ …      │ ┌ details (280, folds) ┐ ┌ reading ───────────────────────────┐│
│ WORK   │ │ Ask                  │ │ 2 Where it stands  (cells on a rule)││
│ …      │ │ Details  label value │ │ 3 What happened    (spine + rows)  ││
│        │ │ People / Seats       │ │ 4 What needs you   (one list)      ││
│ ● agent│ │ Tags / Room / Docs   │ │ 5 Commercial / The score           ││
│        │ └──────────────────────┘ │ 6 About / Understanding / Committee││
└────────┴───────────────────────────────────────────────────────────────┘
```

- **Sidebar**: 224px expanded, 52px collapsed, and collapsed is the default.
  `--bg2` with a hairline. Workspace name and the fold at the top, the search
  field with `⌘K`, the navigation in groups under sentence-case group labels,
  the agent's status at the foot. Collapsed, labels become tooltips and only
  the task and approval counts stay, as small badges.
- **Top row**: 44px, the breadcrumb on the left, the Details toggle and the
  reader's monogram on the right.
- **Record head**: a 40px mark, the name at 18px with its badges on the same
  line, one line of facts under it, the verbs on the right.
- **Details panel**: 280px on the left of the reading, folds to nothing. It
  holds Ask, the attributes as label and value rows (with the evidence
  underline on a machine-read value and a lighter "Add …" on an empty one),
  and the short lists that describe the record (people, seats, tags, the Deal
  Room, documents).
- **Directions set aside.** A lit glass "Aurora", an emerald chrome, a deep
  ink chrome, an editorial paper, a monochrome command surface, a bento and a
  tactile set were built on the same page. Each added a kind of element. The
  brief that settled it was Attio: clean, simple, modern; one ground, one line,
  one row.

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

### The order of the zones

1. **Who.** Identity, the standing badges, one line of facts, the verbs.
2. **Where it stands.** The readings, and on a deal or a lead the stage
   stepper beside them.
3. **What happened.** The timeline, headed by the spine. It sits this high on
   every record because the 360 opens on what happened last: a rep who has
   been away for a week reads the gap before anything else.
4. **What needs you.** One list. The agent's read of the record is its lead
   row; the agent's finds, staged approvals, the reply that is owed, overdue
   and due tasks and the next meeting are the rows under it.
5. **The money or the score.** Commercial on a company and a deal; the score
   on a lead.
6. **The slow-changing rest.** About on a company, understanding on a
   contact, the buying committee on a deal, the details on a lead.

The context column is the same on every record: Ask at the top, then the
panes that answer "who and what is this" (people or seats, details, tags,
the Deal Room, documents, evidence).

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

The sidebar collapses to a 60px icon rail and that is the default. Labels
return on hover as tooltips, the counts that matter (tasks, approvals) stay as
small badges, the agent's orb stays at the foot. The fold control sits in the
rail's head. An expanded rail is the reader's choice and is remembered.

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
| `Button` primary | `--accent` fill, white text, 6px radius, 30px, weight 500. One per view. |
| `Button` secondary | White fill, `--line2` outline, `--ink` text. Icon-only at 30×30 for the overflow. |
| `Button` ghost | No outline, `--ink2` text; hover `--bg3`. |
| `Button` danger | Outlined in `--bad`; fills only inside a `ConfirmModal`. |
| `TextInput` / `Select` | White, `--line2` outline, 30px, 6px radius; focus is a 2px emerald ring. Label above at 12px 500; helper below at 12px in `--ink3`. |
| `Badge` | `quiet` by default: a 6px dot and a word. The pill (`--bg3`, 20px, 11.5px 500) is for the one status that must not be missed and for the record's standing badges beside its name. |
| `Chip` | The same pill. There is one pill. |
| `Panel` | Becomes a **zone**: a 12.5px 600 title with its count and its verb, a hairline, rows. No fill, no edge, no radius. `PanelPlate` (the inset well) becomes a row on `--bg2`. |
| `StatCard` | A **cell** on a rule: label at 11.5px, figure at 18px 600 tabular, basis at 12px; cells separated by a vertical hairline, not by boxes. |
| `FieldGrid` / `FieldRow` | The attribute row in the details panel: a 96px label with its glyph in `--ink3`, the value in `--ink`, "Add …" in `--ink4` when empty, the dotted evidence underline when a machine read it. |
| `ListTable` / `DataTable` | Headers at 11.5px 500 in `--ink3`; 38px rows; hairlines; figures right-aligned; the selected row on `--accentBg`. Edge to edge inside its zone. |
| `RecordTabs` | Text tabs on a rule; the open one in `--ink` with a 2px ink underline; counts at 11px in `--ink4`. |
| `SegmentedControl` | `--bg3` track, white pressed segment with a faint shadow, 12px. |
| `Modal` | White, 8px radius, the one shadow, a scrim of `rgba(24,24,27,.4)`. The drawer form slides from the right with the same surface. |
| `EmptyState` | Left-aligned in the zone it belongs to, `--ink3`, one sentence and one verb. |
| `Callout` | A row on `--bg2` with a dot in the tone's colour before its first words. Never a filled coloured box. |
| `StagingCard` / `DecisionCard` | The agent's row: `--aiBg`, 8px radius, the "Margince read this record" label at 11.5px 500 in `--aiText`, the verdict word at 15px 600 (amber when warn, green when calm), the sentence in `--ink`, "What this rests on · n sources" in `--aiText`, the agent's verb in `--ai`. A staged change is a row with a dashed `--aiLine` edge, Accept and Dismiss. |
| `Kbd` | 10.5px in `--ink4` with a `--line` outline, 4px radius. On the search field and the ask field. |
| `Spine` | Six stops on a `--line2` axis: a date at 11px, a 9px dot, a 12.5px 500 title, a 11.5px detail; the gap stop at 1.5× width with an amber dashed segment; today filled emerald; the future stop dashed. |
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

- Does not draw a box around a section. A zone is a title, a hairline and
  rows.
- Does not cast a shadow, a glow, a tint or a gradient at rest. A button is a
  flat shape; the only tinted thing on a page is the agent's row.
- Does not say a fact twice. One home per fact; the rest are links to it.
- Does not fill a button, a row or a surface with emerald except the one
  primary verb and a selected row's wash.
- Does not tint anything indigo that a person wrote.
- Does not set a number in the proportional face.
- Does not centre a title, a section or a table.
- Does not use uppercase outside an eyebrow, or weight 700 outside the rare
  word that must outrank a heading.
- Does not add a second typeface, an uppercase label, an emoji glyph, a
  gradient hero, or a shadow under a shadow.
- Does not invent a component that the catalog already names. Grep first.

## 11. Adopting it

The tokens above land in `frontend/src/design-system/tokens.css`, and
`tokens.test.ts` pins values, so a repaint is a change to both in one commit —
the test is what proves the canon moved rather than one screen. The order of
work that gets the most visible change for the least churn:

1. **The shell.** The sidebar to `--bg2` with a hairline and the fold, the
   top strip to a 44px row with the breadcrumb and the Details toggle. One file
   each (`shell.css`, `topbar.css`), and the whole product stops looking like a
   template.
2. **Type.** One family, Inter with `cv11`, `ss01` and `tnum`, and the scale
   in §4. `check-font-lock.sh` holds the count.
3. **Zones.** Restyle `Panel` from a bordered white card to a title, a
   hairline and rows. Every record page and settings page changes with it.
4. **Figures.** `tnum` on the whole page and `StatCard` as a cell on a rule;
   the mono face retires from figures and stays for identifiers only.
5. **Grounds.** Move the paper and surface values; re-check every `color-mix()`
   derivation in both themes with Storybook's theme control.

Each step is one PR and a Storybook pass in both themes. Verify contrast on
the new grounds with the existing axe suite before the values are pinned.

## 12. Checklist for a new screen

- [ ] One name at 18px 600; every other emphasis is 500.
- [ ] Every figure is tabular and right-aligned in a column.
- [ ] Every zone is a title, a hairline and rows; nothing casts a shadow at rest.
- [ ] One emerald-filled control in view.
- [ ] Anything an agent wrote is a row on `--aiBg` labelled "Margince read
      this record"; a staged change has a dashed edge until accepted.
- [ ] A withheld section says "Hidden from you"; an empty one says what to do.
- [ ] Checked in both themes and with the sidebar and the details panel folded.
- [ ] The page carries every section the current screen renders (§7), or says
      which it dropped and why.
- [ ] Every control came from `frontend/src/design-system/`; the catalog table
      names anything new.
