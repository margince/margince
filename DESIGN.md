<!-- prose:plain -->
# `DESIGN.md`: the look of Margince

Every new screen is designed against this file. It states the look the product is moving to and the
tokens that carry it. It shows the parts of each kind of page, and the rules that keep the look
from going back. How to build a screen (which primitive, which file, which gate) is in
[`frontend/src/design-system/README.md`](frontend/src/design-system/README.md). This file decides
*how it should look*; the catalog decides *what you build it from*.

Read this before you design anything a user can see, and again when a screen does not match the
others.

## 1. Five things the language refuses

Each rule below stops one of these problems.

- **Everything as a box**. A panel inside a panel, each with its own border, corner and shadow, on a
  ground of almost the same colour as the panels. When every block is a card, every block has the
  same level and the eye has nowhere to land. A page has *sections*, and few of them are cards.
- **The chrome in the colour of the content**. A sidebar, a page and a card in the same white are three
  surfaces that each ask to be first. The frame must step back from what it holds.
- **One typeface at one size for every job**. Titles, labels, values and body text may change only by a few pixels and one weight step.
  Then a record's name and a field label read as the same kind of thing.
- **Numbers set like words**. Money and counts in a proportional face do not line up. A column of
  amounts then turns into a block of text instead of a table you can read down.
- **Colour spent on nothing**. Here the accent is a button and a link, and the rest is grey. A
  product with a strong view about *who did this* (a human or an agent) then shows that view only at
  chip size.

Each one is fixed by order and level, without adding colour.

## What the best products do

The products users name when they say "that looks good" (Linear, Attio, Vercel, Stripe,
Raycast, Notion) disagree on colour and agree on almost everything else. Each way of working below comes
back in every one of them. Each is stated here as the rule this language takes, so you can check a
screen against it. The sources are listed at the end of the section.

1. **Depth comes from a ladder of surfaces**. Linear and Raycast draw no drop shadow at all.

   A card is one step lighter than the page, and a row under the pointer is one step lighter again.
   That order is the whole system of height.

   Vercel puts the border first. A 1px hairline marks
   every element that does not move, and a real shadow is kept for something above the page. Rule: paper → surface → raised is the ladder, and it keeps the parts apart.

   A surface at
   rest adds one tight layer on top (`--shadow-rest`, 1px down and 2px of blur at 5%). That gives it
   a top side, and it still sits on the page. Only a popover, a menu or a drawer casts a shadow a reader
   would call one. Never two shadows on one element.
2. **Cast shadows are tinted and in layers**. In Stripe, shadows are tinted.

   The value is `rgba(50,50,93,.25)`, because its brand is navy, with a second tight layer close
   to the element. A plain black shadow makes a card look pasted on.

   Rule: there are three depth
   tokens, all from one light source above. `--shadow-rest` is one tight layer for a thing with a
   top side. `--shadow-well` is that same layer turned `inset` for a field, which has a floor
   instead. `--shadow-pop` is soft and far, for what sits above the page. All three follow the
   theme: the theme decides the ink, and the shape is fixed.
3. **A top-edge light makes a fill look built**. In Raycast, buttons and keycaps carry
   `inset 0 1px 0 rgba(255,255,255,.1)`.

   The design guides put the same light line, one pixel high, on every "high-end" control. **Rule:** this
   language does not take it. Buttons are flat (section 2, sentence 4), and `--shadow-rest` gives a
   filled control its top side.
4. **Corners inside corners follow one rule**. The most common reason a screen looks "off": the corner of a
   control does not match its card.

   Inner radius = outer radius − padding. Rule: a 20px pane with an 8px inset holds 12px controls;
   `--r-lg` 20, `--r-control` 12, and a keycap 4. The ladder runs in fours (4 / 8 / 12 / 16 / 20
   and the pill). It is twice as large under `corner-shape: squircle`, where a superellipse of radius R
   looks about as round as a plain round corner of R/2.
5. **Text is near-black slate**; greys carry the hue. Stripe sets text in deep slate on an
   almost white ground, never plain black.

   The Refactoring UI rule is that a grey far from middle lightness needs colour, or it looks
   flat. Rule: `--ink #101a15` and the whole ladder of ink and ground sit on the emerald hue
   at low chroma. So the page does not read as a grey page with a green accent.
6. **One accent colour, spent almost nowhere**. In Linear, lavender is the brand, the primary
   button and the focus ring, and nothing else.

   The primary action in Raycast is plain white. The accent colour in Vercel is a status colour and a focus
   ring. Rule: emerald is the chrome, the one primary verb in view, a link and the focus ring. The
   cards on the board, the readings and the rows have no colour.
7. **Type: tight letter space, tabular figures everywhere**. In Linear, the weights run 600 → 400,
   with `-0.6px` on a 28px title.

   Vercel sets large heads at `-0.04em`. Every guide makes `font-variant-numeric: tabular-nums` a
   must for money and tables. Raycast turns on a stylistic set, so its Inter stops looking like
   the Inter in every other app. Rule: headings in the display face at 700 with `-0.025em`, body at 400 and
   500. Every figure is tabular in the body face. The display face is the one place the type has a
   look of its own.
8. **A full screen is a feature**. The whole Attio product is a close grid with strong labels,
   light surfaces and no brand elements that stand out.

   Current design guides ask every element you can see to have a reason to be there. A full screen and a
   clear screen live together through strong views about the workflow. Rule: rows are 44–48px, and a table shows 8
   columns before it scrolls. The record page answers the rep's first five questions above the
   fold. A full screen comes from saying each thing once; the space between rows stays.
9. **The keys are in view**. Linear, Raycast and Attio show a command field with its shortcut.
   They show the key beside every verb in a menu.

   A keycap draws as a small real key (a one-step gradient, a 4px corner). Rule: `⌘K` on the
   command field, `/` on the ask field, and the shortcut on every menu row.
10. **Motion: transform and opacity, four times**. One easing family, and success states and
    figures move.

    Stripe moves a number to its new value, and marks success in a quiet way. Every guide holds hover at
    150–200ms and larger moves at ~300ms, and every guide says layout must never move. Rule: the
    current 90 / 140 / 200 / 360ms set, `--ease-out` everywhere, and `--ease-spring` on release
    only. A reading that changes counts up over `--dur-move`, and a saved row lights
    `--accentWash` once.
11. **Space shows what goes together**. Space inside a group is at most half the space around it.

    The label sits 4–8px from its field, fields sit 12–16px apart, and sections sit 32–48px apart.
    Rule: use the `--space-*` ladder, and put the 2:1 test to any group that looks "too open".
12. **The eye beats the ruler**. Take an icon beside a label, the shape on
    a start button, or the dot on a chip. Each is centred by eye and moved by a pixel.

    Rule: when a row looks off by a pixel, it is. Move it, and say so in a comment.

Sources for this section:

- the write-up on the new Linear UI, and the LogRocket article on "Linear design";
- the notes on the Raycast and Linear design systems in the awesome-design-md repository;
- the articles that take apart Vercel Geist and the Stripe dashboard;
- Refactoring UI, as its readers explain it;
- the styleseed rules on how a screen should look, and the notes by Emil Kowalski on design and code.

## 2. The language in five sentences

1. **A lit ground, and one pane per zone**. The page is light green paper, lit from two corners.

   An emerald glow sits behind the sidebar, and an indigo one at the top of the far edge. Each zone
   of a record is one white pane on it, with a hairline edge and a 20px corner. Inside a pane there
   is only a title, a rule and rows. Dark is the same room with the lights down.
2. **Everything is a list**. The fields of a record are a list of label and value, in a panel on the
   left that folds.

   What happened is a list. What needs you is a list. Money is a list. Because every list is the
   same list, a rep never learns a second layout.
3. **One display face carries identity**. The record name, the zone title and the agent's verdict are
   set in the display face.

   Everything else, figures included, is one quiet sans with tabular figures.
4. **Buttons are flat, and one is filled**. No gradient, no light line on the edge, no glow.

   A filled emerald verb is for the move the page names, and white outlines are for the rest.
   Every button carries the resting layer, `--shadow-rest`.
5. **Colour means something**. The emerald accent is the one filled verb, a link and the light behind the
   sidebar.

   The indigo tint is a row that says an agent wrote it, and the light at the top of the far edge.
   Green, amber and red are a soft tint behind a word. Nothing has colour just to look good.

Gates already hold rules 1 and 5; this file adds 2, 3 and 4.

## 3. Colour

The meaning of each colour stays as it is. `--accent` (emerald) is the brand and the primary
action. `--ai*` (indigo) says an agent made something. The five state hues report how something
turned out. These names are the ones `tokens.test.ts` pins, together with `--ai`, `--aiLight`, `--aiMed`
and `--aiText`. What changes is the ground, which is lit, and the surface, which is one see-through
pane per zone.

**Five states, and never one more**. A screen may not make up a new one.

- *Information*: the neutral report and the work still running (an `info` mark, the ring of a
  spinner, a row a job is still writing).
- *Success*: a good end to the work.
- *Warning*: a note before the fact, the sentence that stops an error while it can still be
  stopped.
- *Danger*: the serious one, or the one that nobody can take back.
- *Discovery*: what is new to this reader (onboarding, a feature they do not know yet). It is the one
  state that is not a verdict about the record.

Each state has one base, and it is the only value someone picks: `--info` `#0485f7`, `--success`
`#17c964`, `--warning` `#f5a524`, `--danger` `#ff383c`, `--discovery` `#964ac0` in light. Warning,
danger and discovery step to a lighter tone that the dark ground can carry. Everything else in a
family comes from that base in `tokens.css` and nowhere else:

- the ink (`--<state>Text`): the base moved in OKLCh lightness until it clears 4.5:1 on every ground
  it lands on and on its own tint;
- the solid badge tint (`--<state>Surface`);
- the see-through wash (`--<state>Bg`);
- the hairline (`--<state>Border`).

A filled control stands on the ink. A base is set to be seen at the size of a bar, and no one
ink reads on all five bases. `tokens.test.ts` reads the five states off the sheet, measures every pair
in both themes, and fails when a state is missing from one of them.

**The neutral ink is two tokens**. `--textPrimary` is the ink that carries (names, headings, body),
at `#15201b` in light and `#fff` in dark. `--textSecondary` is the one that supports, at
`lch(40% 1 282)` in light and `lch(63.304% 1.425 272)` in dark, and a placeholder is the first thing
that uses it. They take the place of the `--ink` / `--ink2` / `--ink3` / `--ink4` row in the table below,
which answers "how quiet is this?" four ways. `tokens.test.ts` measures every pair the product draws
against every ground it lands on.

The tables below are the design target, and they name some steps the tree does not carry yet:
`--ink4`, `--aiBg`, `--aiLine`, `--ok`, `--bad`. Read a name that does not appear in `frontend/src`
as a value still to come. Today the shipped name of the agent tint is `--aiLight`, its edge is
`--aiMed`, and the state hues are the five named above.

### Light (the default)

| Token | Value | Role |
|---|---|---|
| `--bg` | `#f1f5f2` | The paper the page is read on. |
| `--glowA` / `--glowB` | `rgba(24,190,120,.06)` / `rgba(91,97,214,.10)` | The emerald light at the top-left corner behind the sidebar; the indigo light at the top right. Round fills of 900×620, and the only thing on the page that is there for looks. |
| `--pane` / `--paneEdge` | `rgba(255,255,255,.72)` + `blur(12px)` / `rgba(16,26,21,.08)` | A zone, the details panel, a board card, a control at rest. |
| `--bg2` / `--bg3` | `rgba(255,255,255,.55)` / `rgba(16,26,21,.05)` | The sidebar (glass over the glow, `blur(20px)`); a pill, a keycap, the open sidebar row. |
| `--bgChip` | `rgba(16,26,21,.07)` | The shipped name of the `--bg3` pill and keycap fill: the fill under a badge, a keycap, a segmented strip, the floor of a meter. It is see-through, so a chip reads one step deeper than any ground it lands on; a solid value would be lost on a plate of the same grey. `.07` and no deeper: `--accentText` on this fill reads 4.57:1 over `--bgCard`, the ground with the lowest contrast of the four a chip lands on, and `.08` would drop that to 4.49:1, under the floor. |
| `--line` / `--line2` | `rgba(16,26,21,.08)` / `.16` | The hairline between rows; the outline of a control, the axis of the spine. |
| `--ink` / `--ink2` / `--ink3` / `--ink4` | `#101a15` / `#33403a` / `#66736c` / `#9aa59f` | Names and values / body / labels and meta / placeholders and dates. |
| `--accent` / `--accentText` / `--accentBg` | `#0b7a53` / `#0a6f4b` / `#e8f3ee` | The one filled verb; a link; a selected row or a done stage. |
| `--ai` / `--aiText` / `--aiBg` / `--aiLine` | `#5b61d6` / `#3f45b0` / `rgba(91,97,214,.09)` / `.35` | The agent's filled verb; its label; the tinted row; the dashed edge of a staged row. |
| `--info` / `--success` / `--warning` / `--danger` / `--discovery` | `#0485f7` / `#17c964` / `#f5a524` / `#ff383c` / `#964ac0` | The five states, as a soft badge (the `Surface` of the tone behind a word, in its `Text` ink, with an edge in its `Border`), or as a solid one for a count and the one state that must not be missed. |

### Dark

The same room with the lights down. The ground is `--bg #0c1311`, a small step above the `#0a100e` of the
mock. So the hover step of the sidebar still has room under the page. The glows are stronger (`.10` /
`.20`) because they are the only light. Each pane is `rgba(255,255,255,.045)` with a `.09` edge, and ink
runs from `#eef3ef` down to `#5c6862`.

 The accent goes up to `#2bb673` with dark ink on it, and the
indigo text to `#b3b7f5`. The chip turns around instead of copying the light theme: `--bgChip` becomes
`rgba(255,255,255,.09)`, because a chip on a dark ground can step only one way. The theme switch in three states moves between
them: `:root`, then `prefers-color-scheme` inside `:not([data-theme="light"])`,
then `[data-theme="dark"]`.

### How colour is spent

- **The two glows are the chrome**. The sidebar is glass over the emerald light; there is no
  coloured bar on any screen.
- **At most one filled control in view**, in emerald: the move the page names. Every other button is
  a pane with an outline.
- **The indigo tint marks the agent's work**. The agent's read is a row on `--aiBg`. A staged change is a row
  with a dashed `--aiLine` edge until a human accepts it. Nothing else is indigo.
- **States are soft badges first**. A column of states is a column of soft badges, one weight down
  the page. The solid fill is for a count and the one state a reader must not miss.
- **A monogram is the one soft gradient**. Most records never get a logo. So the first letters of a
  contact or company sit on a quiet mesh of two close hues, keyed on the record's id. The mesh is
  never indigo, never danger red, and never moves.
- **Every chip is round**, a company's included. A reader tells records apart by their names, and the mesh is
  what makes it easy to find. A logo takes the place of the mesh in full: it waits on the neutral card ground and
  never on a gradient.

## 4. Type

Three families, which is the limit `check-font-lock.sh` holds. The third is for code alone. A mono
face made an amount or a date look like machine text. In a full row, it pulled the eye too much. `tabular-nums` gives a column the lined-up figures that mono was once used for.

**Size answers the context; level answers the structure**. The size of a heading says what it
leads into, and where. Brand and sales pages sit at the top of the ladder. The title of a product
page comes below that, and the title of a component at the bottom. 

Which of `<h1>`–`<h6>` it is written as says where
it sits in the page. A user of a screen reader moves through the page that way: one `<h1>`, then
down, with no level skipped. Size may not decide level, and level may not decide size.

Body text has three levels on the same base. Each one carries the paragraph space that separates two
blocks of its own text. 

Weight is meaning. Both text families ship three weights, so a fourth would
be one the browser made up. Weight 400 is for prose. Weight 500 is for text beside a line icon, and
for most text inside a component. Weight 700 is for a heading, or for emphasis the text needs.

Everything is rem on the 1rem = 16px of the browser, so when a reader makes that larger, the product
grows with them. The three weights are named tokens (`--fontWeightRegular` / `Medium` / `Bold`) that
the size tokens read. All of the type (size, line height, weight, letter space) is declared in
`tokens.css` and in no other file. `design-system/type-source.test.ts` fails a sheet or a `style`
value in code that spells any of the four by value.

**Almost none of it is in use yet**. `body` in `app.css` reads `--fontBody`, and that is all of it.
There is no `h1`–`h6` mapping and no class hook. The `.t-*` names in `base.css` are role hooks with
no rules on them. 

The tokens and the rules for using them live in
[`frontend/src/design-system/README.md`](frontend/src/design-system/README.md), which is what a
builder reads. So every size, weight, letter space and neutral ink in other parts of this file is a
target for that new build. The shipped sheet may not match it.

| Role | Family | Where |
|---|---|---|
| Heading | **Outfit** | Every heading, and the one line that names a record: its name, the Brief welcome line, the title of a zone, the agent's verdict word, the word of a reading and its figure. |
| Body and UI | **Geist** | Everything else, prose included. |
| Figures | **Geist**, tabular (`.t-num`) | Every amount, count, share in %, length of time and date in a row or a cell. An id is plain body type. |
| Code | **Geist** `Mono` | `<pre>`, `<code>` and `<samp>` through one rule in `base.css`, and the `.code-block` surface; nothing else. A `<kbd>` is body type. |

## 5. Space, shape, depth

- **4px base**, the current `--space-*` ladder, spent freely. The page gutter is 32px, and panes sit
  24px apart. A pane has 20px above and below its content and 26px at its sides. A zone title has
  12px under it.
- **Rows have room**. A list row has 13px above and below its content (about 48px tall with two
  lines). A table row is 44px, a sidebar row 38px, and a control 32px (28px in a row). The field
  rows in the details panel are 32px. The test for "gedrungen": if a reader could take two rows
  for one, the row is too short.
- **Type at rest is 13.5px on 1.55**, so a row's second line does not touch its first. Prose is 14px
  on 1.65 at 72 letters to a line.
- **A role with a class uses the class**: `.t-caption`, `.t-sub`, `.t-label`, `.t-eyebrow`, `.t-h3`
  and the rest, all in `base.css`. What each one draws is the new build in section 4, one role at a
  time.
- **Corners by role**:
  - 20px for a pane, the details panel, a reading card and a modal;
  - 16px for a board card and the agent's row;
  - 12px (`--r-control`) for a control;
  - 8px for a chip, and 4px for a keycap;
  - full for a pill and a monogram.
- **Depth is light first, shadow second**. A pane is see-through over the lit ground, with a
  hairline edge; that is its height. On top of it, the shadow turns on what the thing is:
  - A surface (a pane, a card, a reading, a board card) and a control with a fill have a top side.
    They take `--shadow-rest`, one tight 1px/2px 5% layer. A ghost button casts one, because the
    pane is its fill. An icon button with nothing around it has no fill and casts nothing, and a text control casts
    nothing too.
  - A field is a place to put something, so it has a floor and takes `--shadow-well`, the same layer
    turned inside. That covers a text box, a textarea, a field shell, and anything that draws as a field,
    such as the search in the top bar. A reader reads the shape.
  - The `Select` button that opens the list breaks this rule. It holds a closed face instead of a place to type, so it
    is shaped like a verb and takes the resting layer.
  - A control closes the gap on hover, on press and on focus. A verb presses into the page, and a
    field's floor comes up to meet the pointer.
  - A popover, menu or drawer takes `--shadow-pop` instead.

  Nothing at rest glows, the monogram mesh is the only gradient, and nothing stacks a shadow under a
  shadow.
- **No pane inside a pane**. A zone is one pane; the things in it are a title, a rule and rows. The
  only closed shapes inside a pane are the agent's tinted row and a staged row's dashed edge.

### More, and less

One layout holds a record with nothing on file and a record with 40 deals. A pane is there only
when it has something to say. An empty zone is one line in its place with its verb ("No deals yet ·
New deal"), and a withheld zone is its sentence. The order of zones never changes with content.

A thin company leads with the deep-read card where the 360 would be. A thin contact shows its readings
as "Not shown" without a tone, and says it has captured nothing. A full record has a limit, and never
grows past it:

- three rows and "N more" into the tab;
- five readings and no more;
- three claims in the 360 sentence;
- six stops on the spine, five in the folded thread;
- the lead row plus three in What needs you;
- `at least N` on a page that stopped short.

### Screen sizes

One ladder for every screen, on the product's current breakpoints:

- At 1400px the figures drop a size, and five readings stay in one row.
- Under 1200px the readings wrap and the two columns stack. The details panel becomes a drawer over
  the page, and the tab strip scrolls in its own box.
- Under 720px the head stacks (mark and name, facts, then primary + more), and readings go two per
  row. The spine and the map scroll from side to side in their own boxes.
- At 390px the phone bar holds, and so does the rule that a page never scrolls to the side.

Fixed widths are only for the details panel and the map; everything else is `fr`, `minmax(0, …)`
and `min-width: 0`. A page never scrolls to the side; the thing that is too wide scrolls inside itself.

## 6. The shell

```
┌────────┬───────────────────────────────────────────────────────────────┐
│ sidebar│ crumb        [ Search or run a command  ⌘K ]                me │
│ (glass ├───────────────────────────────────────────────────────────────┤
│  over  │ mark  NAME (display)  badges                  verbs, right     │
│  the   │       facts line                                               │
│  glow) │ tabs ──────────────────────────────────────────── [Details] │
│ Brief  │ ┌ THE 360 ─────────────────────────────┐ ┌ details (300) ────┐│
│ RECORDS│ │ ● {name} · 360                        ││ │ Ask               ││
│ …      │ │ VERDICT  because · sources (hover)    ││ │ Details           ││
│ WORK   │ │ readings ┊ readings ┊ readings        ││ │ Contacts / Seats    ││
│ …      │ │ spine · · · gap · today · ahead       ││ │ Tags / Room / Docs││
│ ● agent│ │ the thread, newest first              ││ └───────────────────┘│
│        │ └───────────────────────────────────────┘│                      │
│        │ ┌ What needs you ┐ ┌ Commercial ┐ ┌ About ┐                     │
└────────┴───────────────────────────────────────────────────────────────┘
```

- **The sidebar**: 224px open, 64px folded, and folded is the default. It is glass over the emerald
  glow, with a hairline on its right. The workspace name and the fold control sit at the top. Then come the
  product's own rows in its own order: Brief; Records (Contacts, Companies, Leads, Filters & views);
  Work (Worklist, Pipeline, Projects); `Intelligence` (Reports, Ask Margince).
- The sidebar has no badges. The approval and task lists are lanes of the Worklist, which reports
  its counts on the page. Settings is not a row; it opens from the account menu.
- While a settings route is open, the sidebar shows the settings second level: Overview, then the
  subject groups. It has its own width and its own head. It is the same column showing a
  different list. The level names itself with the heading over its first group, and the way out of
  it reads "Back to app". The agent's orb stays at the foot.
- **Top bar**: 48px, glass, with a hairline under it. The breadcrumb is on the left, the command
  field in the middle (`⌘K`), and the reader's monogram on the right. It is the app's one bar, and
  every screen has it. Nothing of a record sits in it: the Details control is in the
  page.
- **Record head**: a 56px mark, and the name in the display face with its badges on the same line.
  One line of facts sits under it, and the verbs sit on the right. The verbs are the record type's base actions,
  never the task of the day:
  - a company: Write email · Log activity · Add task · more;
  - a contact: Write email · Call · Add task · more;
  - a deal: Write email · Log activity · Edit deal · more;
  - a lead: Qualify · Write email · Edit · Disqualify · more;
  - a project: Log activity · Edit project · more.

  **None of them is filled**. The head's verbs all have outlines, so the page has one filled verb.
  That verb is the move the call names ("Confirm the rate", "Reply on her thread"), inside What
  needs you, where its reason sits beside it. A lead is the one case apart: its Qualify is filled,
  because to qualify is the whole job of the record. The **more** button (a 36px square with an
  outline and the three dots) holds Archive, Share, Merge and the rest. The **Details** control sits
  at the right end of the tab row, in the page.
- **Details panel**: 300px on the right of the reading, one pane, closed until the Details control
  at the end of the tab row opens it. It holds Ask, and the fields as label and value rows. A
  value a machine read has the evidence underline, and an empty one has a lighter "Add …". It also
  holds the short lists that tell what the record is (contacts, seats, tags, the Deal Room, documents).
  The current product keeps its context column there, so a rep's hand does not move.
- **The glows**: the emerald light at the top left is low (`.06` in light, `.10` in dark). The
  indigo one at the top right is a step stronger. They set the feel of the page, and the eye should not find
  them.

## 7. How the tool feels, and how a page is built

**The feel**. Calm, lit, and easy to read from far away. A rep opens an account, and in one screen
knows where it stands, what they owe, and what happened. The agent writes in one place on the page,
and says what it rests on. Every figure is in the same face and lines up.

Nothing glows, and the one
shadow anything rests on is too light to see as a shadow. It should feel like one folder open on a table in good
light; a dashboard is the wrong model.

**Five rules of structure**. Each one stops a shape a record page takes on as it grows. One such
shape is a fact with more than one home. A second is verdicts stacked above the list they should
lead:

1. **Every fact has one home**. If a deal appears in the readings, it does not also appear in the
   rail.

   If contacts are in the context column, they are not also a section in the work column.
2. **The zones are numbered in reading order**. A record page is five zones with a heading each.

   The number is the order a rep reads an account in, and a colleague can then say "look at 3".
3. **One list of what needs you**. The agent's read of the record is the lead row of that list, with
   no separate card above it.

   The agent's finds, the moves made by hand, the late tasks and the next meeting are rows below it, in
   order of need. A rep has one place to look and one place to clear.
4. **One timeline**. The spine (the axis that draws the gap to its width) heads the list of what
   happened, as one component.
5. **Context column: who and what is this**. Ask at the top, then the contacts, the fields, the
   tags.

   State and work stay in the work column, where the reading order is.

### Glance, then depth

The overview with the most room is the one that shows less. A record's overview has two depths, and
the one with less depth is the default.

**The glance** is the whole first screen. It keeps the open feel of the version with less on it. It
takes the product's own features back in small form: each one there, and none at full depth.

- The name at 32px in the display face sits beside a 56px mark. Both are centred on one axis, so a
  facts line that wraps never breaks the head. Then the facts with the live dot
  (`In conversation · Hamburg · Freight forwarding · 240 employees · Owner`), and three verbs. No
  badges past the standing, and no second line.
- **A quiet tab row** under the head (no pane, no rule, a 2px accent line under the current tab). So
  every tab page (History, Contacts, Deals, Tasks, Finance, Documents, Profile, Partner) is one click
  away. The links inside the panes ("All deals", "Full history") open the same tabs.
- **Five readings as cards with room**: 138px tall, with an upper-case label. Each has one figure at
  26px in the display face with tabular figures (22px under 1400px). One line of basis sits at the foot, with
  empty space between the cards.
  - **Every reading carries its evidence**: an "evidence" chip at the right of the label opens a
    popover on hover or focus. It shows what the figure rests on: the rows it adds up, the date it
    was read, and the account it is from.
  - **Every reading is a door**: the card opens the tab page that holds its rows. Open pipeline →
    Deals, Invoiced → Finance, Conversation and Last touch → History, Next → Tasks. The evidence
    chip does not.
  - When the details panel is open, they get smaller, to 21px figures, and never wrap.
- **The 360 as the first pane**, full width, on the same hairline as every other pane. It carries the
  record's name and "· 360" as the pane title, and the agent's word at 34px. Beside the word is one
  sentence whose claims are sources you can hover. The chip sits in the line on the agent's tint, and
  hover opens what it rests on. Then come the three rated measures and the spine.
- The thread is folded: "Read the thread · 5" opens the five latest messages inside the pane, and
  "Full history" is the History tab.
- **Left column**: What needs you, one list. Your move leads it as the agent's row: a title in the
  display face, one sentence, its sources, and the filled verb that does it. What the agent suggests
  is the second row of the same list. Then come the tasks, with the call as the task of getting ready
  for it. Below it, Deals, with three rows.
- **Right column**:
  - Ask as a pane of **prepared questions**: four the record can answer today (`What changed since I last looked?`,
    `Why is Rostock stalled?`). Each one is a row of full width on `--bg3` that turns
    indigo on hover. No free-text field until the answers are good enough to need one.
  - About: the lead sentence, one paragraph, its sources, and "Profile" for the rest.
  - Contacts as chips with a "+N".
- **The details panel is hidden until asked**. The Details control at the end of the tab row opens it
  on the right at 300px, with the fields, the contacts and the tags. It is the only thing on the
  glance that starts closed.
- No zone numbers. Commercial and the rest of About are the deep overview, one click away, with the
  same panes.

**One home per fact, one word per screen**. The glance says each thing once. The readings carry the
figures, the 360 sentence carries the because, the spine carries the time, and the needs list
carries the verbs. "Your move" is a reading, a spine stop and the lead row's eyebrow. It is never a
second verdict word.

The 360 owns the only word in the display face on the screen, and that word is about
the record ("Good", "Live", "Promise overdue"). The needs list's lead is a sentence about you. The
way in sits on the facts line, because that is the first thing a rep needs before a call.

**The same depth on every record**. Contact, deal and lead follow the company glance line for line;
what changes is the content of each slot.

| Slot | Company | Contact | Deal | Lead |
|---|---|---|---|---|
| Live dot | In conversation | Your move (warning) | Your move (warning) | In motion |
| Facts | city · industry · size · owner · way in | title · employer · email · phone · way in | account · value · stage · close · owner · partner | title · company · email · source · owner |
| Verbs (base, never the task) | Write email · Log activity · Add task · more | Write email · Call · Add task · more | Write email · Log activity · Edit deal · more | Qualify · Write email · Edit · Disqualify · more |
| Readings | Open pipeline · Invoiced · Conversation · Last touch · Next | Whose move · Open promises · Deals she decides · Next meeting · She answers in | The money · The close · Stage · The contacts · Momentum | Company · Score · First response · Next · Your move |
| 360 word | Good | Promise overdue | Live | In motion |
| In the 360 | the spine | the spine | the stage stepper, then the spine | the ladder, then the spine |
| Needs-list lead | Your move | The move the call names | Margince suggests, then the staged change | Ready to qualify (no agent tint: a lead carries nothing the agent suggests) |
| Second left pane | Deals | The deal she decides, with the room | The buying committee, with the cover gap | The score as factors |
| Right column | Ask (prepared questions) · About · Contacts | Ask · Understanding her · Around her | Ask · What this deal is · Offers · Deal Room | Ask · If she is qualified · What she asked for |
| Tabs | Overview · History · Contacts · Deals · Tasks · Finance · Documents · Profile · Partner | Overview · History · Network · Deals · Meetings · Data & tools · Documents | Overview · History · Documents | Overview · History |

The test: a rep back from 7 days away reads the glance in ten seconds. They know where the account
stands, what happened last, what they owe, and what the agent suggests. Anything they do not need in that
time goes to the depth.

### The order of the zones

1. **Who**. Identity, the standing badges, one line of facts, the verbs.
2. **The 360**. One element, and the one place the page may look like a feature.

   It shows where the record stands and what happened, together. It opens with the indigo tile and
   "{name} · 360". The tile is the claim of who wrote it, so no sentence beside it says so again. Then,
   in order:
   - the verdict word in the display face, with its because-sentence and its sources;
   - the readings as cells;
   - the stage stepper or the ladder, where the record has one;
   - the spine and the thread, newest first;
   - a foot that says who wrote it.

   Its edge is the same hairline as every other pane, with no wider border and no coloured rule.
   The indigo tile and a light indigo wash in one corner are all it has for looks. You can hover every
   claim in it: a source chip opens the quote it rests on, and where the quote is from.
3. **What needs you**. One list. The move the call names is its lead row.

   The agent's finds, staged approvals, the reply that is owed, late and due tasks, and the next
   meeting are the rows under it.
4. **The money or the score**. Commercial on a company and a deal; the score on a lead.
5. **The slow-changing rest**. About on a company, understanding on a contact, and what the deal is
   and the buying committee on a deal.

The details panel is the same on every record: Ask at the top, then the panes that answer "who and
what is this".

**Tab pages**. Every tab in the strip opens a real page, in the same panes. On a company:

- History: the filter strip, then the **rail timeline** below.
- Contacts: the cover band, **the committee map**, and the list of contacts as a table with list, board and map
  cuts.
- Deals: the commercial band, and the deals table with won and lost.
- Tasks: tick, put off, open.
- Finance: five readings with their evidence, then three charts. **Invoiced by month** is bars, and the
  **overdue share** is a meter with its key. `Payment behaviour` is a line of days late per closed
  invoice, oldest first. Then come the recent invoices table and the line that names the
  provider and the last sync. Each reason finance cannot be read shows as a row, never as a zero.
- Documents: contracts, then files with category and origin.
- Profile: the details form, what they do, the facts Margince read with confirm and fix, linked
  records, data and tools.
- Partner: the partner program and its deals.

On a contact:

- History;
- Network (the best route, the ways in, **the relationship map**, what changed);
- Deals (the seats she holds);
- Meetings (next, with brief and room; past);
- Data & tools (the provider data at one point in time, and what Margince read);
- Documents.

On a deal: Documents and History. On a lead: History.

### The deep overview, record by record

Each numbered list below is a deep overview: everything a record can say, in order. The glance
draws its panes from them, and the tab pages carry the rest, one more click in.

### The company record

1. **Who**. Mark, name, lifecycle and relationship badges, and one line of facts.

   The facts are site · industry · size · owner · way in. Then one quiet line (last contact ·
   date added · captured by), and the verbs: Write email, Log activity, Add task, more.
2. **Where this account stands**. Health carries the call: the standing word, whose move it is, and
   the three rated measures as dots.

   Beside it sit Open pipeline, Invoiced and Conversation, each a door into its tab. The glance
   draws five readings (see the table above), with the call of Health in the 360. `Not shown` and
   `Not assessed` stay apart.
3. **What happened**. The spine, then the recent rows. "N new since your last visit" sits in the zone
   head.
4. **What needs you**. The moment as the lead row.

   That row has an indigo ground, the verdict word in the display face, what it rests on, and the
   agent's action in the agent's fill. Then come the agent's finds, the late and due tasks, and the
   next meeting. The foot names what was hidden from the reader.
5. **Commercial**. Contract state, won and lost on one line. Then each open deal with its one short status
   line, then the project it is part of.
6. **About**. The dossier lead and paragraph, its source and how old it is, and the signals.

   Then the fit verdict, with a link to how Margince decided it.

Context column: Ask (with three prepared questions), Contacts (three, with who is in touch from our
side), Details (9 fields, mail capture included), Tags.

### The contact record

1. **Who**. Round monogram, name, the buying-role badge, title and employer.

   Then the glyph line (email, phone, city, LinkedIn, owner). Then the verbs, all with outlines:
   Write email (named for the way to send it when there is only one), Call, Add task, more.
2. **Where you stand with her**. One strip of seven.

   Overall (the pulse verdict, in its colour only when not withheld), Last inbound, Last outbound,
   In · out as counts, `Colleagues`, Next meeting, Consent.
3. **What was said**. `Conversation memory`, with its All / Email / Meetings / Calls / Notes cut.

   Each row shows replied or unanswered, and has a reply verb.
4. **What needs you**. The moment as the lead row, with its actions and how ready they are.

   Then the commitments and open loops as rows, with a read-only tick.
5. **The deal she decides**. Title, amount, stage, close, owner, the buying committee.
6. **Understanding her**. The relationship brief, then Priorities, Objections and Success as three
   rows, then the source and `Correct something`.

Context column: Ask, the waiting email, Details, Who knows her, `Consent & channels`.

### The deal record

1. **Who**. Mark, name, the status badge and the project chip, and the company and partner line.

   Then the three facts (Value, Stage, Owner) and the pulse sentence
   (`It's your move. They wrote last on 1 Sep, 3 days ago.`). Verbs: Write email, Log activity, Edit deal, more (Archive, Share,
   Reopen), all with outlines. The owed reply is the filled verb in What needs you.
2. **Where this deal stands**. The money (with the newest offer and its status).

   The close (days, forecast group, provisional or waiting). The contacts (engaged of total,
   champion named, single-threaded). The momentum (days since the last contact, stalled). Under them
   sits the stage stepper. Done stages are tinted, the current one is filled, and terminal stages
   come last. Beside it stands the rule that a terminal stage asks first.
3. **What happened**. The spine (meetings, calls, offers, the gap, today, the expected close).

   Then the rows, the agent's own stage change included.
4. **What needs you**. The Deal360 brief as the lead row.

   It holds the verdict word (Live / Drifting / Blocked / Cold), and the because-sentence with its
   citations in the line. Then the cover signals as chips, and "What to do next" with its one verb.
   Then `Read the full briefing`, the line that names who wrote it, and the verb to write it again. Then any staged approval (dashed,
   Accept / Dismiss), then the reply that is owed.
5. **Commercial**. The offers table (number, version, status, value, sent), with the FX basis in the
   zone head.

   Then its offer actions. Forecast and wait-until are edited in Details; custom fields have their
   own Details section.
6. **The buying committee**. The map: our circle, their seats, threads only to the engaged, and a
   dashed ghost per cover gap.

   Then the stakeholder table with role, contact, in touch or not, dates and edit. Add stakeholder sits in the
   zone head.

Context column: Ask, then Deal Room (state, invited, signed in, last seen, open). Then Who is on
this deal: seats with Engaged / No two-way contact, and who of ours carries each one. Then Related evidence,
and Documents.

### The lead record

A lead is kept apart from the contact graph by design, and the page says so. The ground carries the
accent tint as its marker, the badge reads "Lead", and the sentence that keeps it apart sits under
the identity. Nothing on a lead is indigo. The tint says "not a contact yet", which is a different
claim from "an agent did this".

1. **Who**. The monogram, name, the Lead badge, and title and company as text (a lead has no company
   record).

   Then email, LinkedIn, source and date, and owner. Verbs: Qualify (filled), Edit, Disqualify, more
   (Share).
2. **Where this lead stands**. Score (with its top two factors), and Status.

   Status shows how it was set: by hand, or on its own from a reply or a meeting. Then First
   response (the SLA verdict and the target), and Source. Under them sits the ladder: New ›
   Contacted › Engaged › Qualified › Disqualified. The terminal two open a dialog.
3. **What happened**. The spine from the web form to the demo on the calendar.

   Then the rows, the status change made on its own included.
4. **What needs you**. "Ready to qualify" is the lead row once the product has its evidence.

   That evidence is a reply or a meeting.

   That row carries the reason behind it, and `Qualify and open deal`. Then the reply that
   is owed, then the tasks.
5. **The score**. Explain this score: each factor with its points, decay and activity count, and the
   line that adds them up.

   Then what you know about this lead (the three questions, how you know, add to the score). Override
   score sits in the zone head.
6. **Details** live in the context column with Owner and the qualify preview.

   The preview shows `Merges into nobody` and the deal it suggests. They live there because a lead has
   few fields, and the page's work is the ladder and the score.

### The shell around every record

The sidebar folds to a 64px icon rail, and that is the default. Labels come back on hover as
tooltips, and the agent's orb stays at the foot. The details panel on the right of the reading opens
from the Details control at the end of the tab row, and starts closed.

### The timeline, as the tool draws it

History is a **rail** of entries. Each entry is a grid of four parts:

- the date in tabular figures, aligned right in a 76px column, with the time under it in `--ink4`;
- a 20px rail column with a mark on it, carrying a 1px `--line2` line that runs the full height,
  so the entries connect;
- the body;
- the verb slot.

The mark says what kind of thing happened. A solid 8px dot is a message, a hollow one a field
change, and an indigo one a change the agent made. A dashed indigo ring is a staged change, and a
24px glyph in a circle is a thread. The body opens with the kind in 10.5px upper case, the direction
words ("they wrote", "we sent", "both sides") and the contacts. Then comes the title at 13.5px 600.

Then comes **the text of the message itself**, cut to three lines. A timeline of subject lines is
a list of things you cannot read. Then a meta line (who wrote the summary, the file sent with it,
Restore).

A thread is one entry: its mark on the rail, and a card on the body side with the count and
the contacts. The messages sit inside it, newest first, each with its avatar, direction and two lines
of text. A hairline under each entry is the only line between them. There are no day headings; the date column
is the axis. The same rail, without the message text, is the folded thread in the 360.

### The spine, as the tool draws it

The spine keeps the product's own shape. Each stop is a column of four rows on one base line. They are the
date at 11.5px in `--ink3`, the **rail**, the title at 13px, and the detail. The rail is a 2px `--accent`
rule with a 9px filled dot at its start. So the rule to the right of a dot is the time that stop
covers.

The gap stop takes 1.5× the width, because on this axis the width is the waiting. Its date row is the
day count at 26px 600 in amber in the display face. It is the one count on the spine outside the body
face, because it is read as a word ("12 days") and set against nothing. Its rule is dashed amber, and
it has no dot, because nothing happened.

**Today is a marker**: a 2px × 15px black bar on the rail, with TODAY in 10.5px upper case and the
date under it. Its column is only as wide as its label. Right of it, the rule turns dotted grey and the
dots turn hollow with an accent ring. These stops have not happened yet.

A solid rule to a date nobody has reached would draw a day that has not come like one that has. An event dated today (the 14:00 call) is
an ahead stop after the marker.

### The relationship map

One drawing primitive, the product's `RelationshipMap`, on three pages, with its own shape:

- three columns of 184 / 200 / 184px with 72px gutters and 16px padding (744px; it scrolls to the side
  instead of getting smaller);
- node heights by kind: a colleague 40, a contact 60, a company 48, a deal 44, a gap 60;
- 8px between nodes, 20px between lanes, and a lane heading in 10.5px upper case.

Each node is a box with round corners on `--pane`, with the name at 13px 600 and a second label. A colleague
sits on `--bg`, a company has a 2px `--ink3` edge, and a deal an accent edge.

**A gap is a dashed amber box** naming the missing role, with `Assign` under it. It is the only
place the map draws something missing in a way a reader can count.

Edges are `cubic` curves. A route is a way in, and carries a band (strong: 2.5px accent; developing:
dashed accent; cold: dotted grey). Being part of a company or a deal is a 1px hairline (works there, on this
deal). A relationship nobody has measured is left out; the map never draws it as cold.

Selecting a node lights its strongest route in ink, and fades everything else to 35%. A 280px panel
on the right names the selected node, the best route with its evidence, and the other routes. It also
holds the one write the map offers: record that two contacts know each other, when the graph has seen it.

On the contact's Network tab, the lanes are our team, the target, and their company. On the
company's Contacts tab, they are our side, the account with its deals, and their contacts **by buying
role**. The roles come in the product's order: champion, `economic buyer`, influencer, blocker, user. A gap node marks
a key role nobody holds.

The deal keeps its small committee drawing beside the seat rows, for
looks only. It shows our circle and their seats, with 2px accent threads only to the engaged. The
quiet ones are hollow, and each cover gap is a dashed ghost.

### The Deal Room, two surfaces on one board

The Deal Room is the one thing in the product that someone outside it reaches. So it is two pages
that share one document board and nothing else.

**The side of the seller** is a page inside Margince, under the deal. Its head holds "Back to the deal",
the room's title, and a facts line. The facts are its state and end date, invited and signed in,
when a buyer last looked, and the steward. The verbs are View as buyer · Room access · more. A state band under the
head says the lifecycle in one sentence (`Live. Access ends on 31 Oct.`), with Pause, Set an end date
and Close beside it.

On the left: the title and welcome the buyer reads first (you can edit them). Then comes the
document board in its four groups: Commercial, Legal, Security & Privacy, Delivery & Operations. A thread and a
composer sit under each document, then the threads for the whole room. 

On the right: Room access,
what is shared and what is hidden, and the lifecycle. Room access lists every participant with what
they may do (view or comment), and whether they are invited, in use or revoked. It also shows when
each was last seen, and how many times they saved a file. A row menu sends a new one-time link, changes what they
may do, or revokes.

**The buyer's side is a public page** outside the Margince shell, reached from a one-time
link. It has no rail, no top bar, no seat, no inner price data, and nothing the link did not already
name. It is the one
page a client sees. So it reads like a small site that looks good, not like a form, on the lit
ground with both glows, in a 1040px frame:

- **A hero**. The contact's mark and "Prepared for {company} by {steward}" sit on the left, and the
  live pill with the end date on the right. Then the eyebrow "Deal Room", the room's title at 40px in
  the display face, and the welcome at 16px. Under it, on the agent's tint, sits **"New since your
  last visit"**. That lists the new answers and the new versions of a file, read from the time
  the participant was last seen.
- **The documents as tiles**. Two tiles per row. Each has a page preview on `--bg3` with the file-type
  tile and an "N unanswered" mark. Then the title, group and page count, the thread count as a pill,
  and `Download`. On hover nothing moves; the edge turns accent.
- **Questions on the documents**, answered in place: each thread under its document's tile, with the
  composer for a participant who may comment. Then the thread for the whole room.
- **On the right: the contact** as a card (mark, name, role, `Write to Tim`, and **`Book a call`**
  into the page where a buyer picks a meeting time). Below it, **next steps written by the contact** as a small ladder
  (done, now, ahead), and who is in the room as chips.
- Sign out sits at the foot, with what the reader may do. `Powered by Margince`, fixed at the bottom
  right, is the only product mark.

The page has four states of its own, each with its way back and its contact:

- a link that does not work any more, with the one form (the email that was invited, and
  `Send me a new link`);
- paused;
- ended;
- closed (read-only; the record stands).

A seller who looks at the preview sees the same page with a line at the top, and can never
write.

### The pages that are not records

Every other page is the same shell. It has the top bar, a title in the display face, one facts line,
and verbs with the more button. It has five readings where the page has figures, and panes below. Each has its own content:

- the **Worklist**: one list in order of need, with a kind column (what happened / why now / what is
  at risk / the verb). It has scope and filter pills, the side pane about the selected row, and the
  team board;
- **Reports**: three reports, with "Explain this number" opening the plan and the rows on an indigo
  dashed pane;
- **Ask Margince**: the ask field, an answer as a pane like the 360 with its citations, the two-tier
  contract, and the passport steps;
- the **Project 360**;
- **Filters & views**: the predicate tree beside the records it finds, and saved views as chips;
- **Settings**: its second level beside the rail;
- the **Deal Room**: the document board with a thread per document;
- the `⌘K` palette: records, actions, screens, and "Ask Margince: …" always last, over a scrim that
  covers the whole frame;
- the drawer to write a message: the agent's draft with the note that an agent wrote it. You edit
  then confirm, send later, or link it again.

## 8. Components, said again in this language

The primitives in the catalog keep their names and `props`; this is how each one looks now.

| Component | Look |
|---|---|
| `Button` primary | `--accent` fill, white text, `--r-control` (12px) radius, `--controlHeight` (32px), `--fontWeightMedium`. One per view. |
| `Button` secondary | `--pane` fill, `--line2` outline, `--ink` text, flat. With only an icon, it is the same `--controlHeight` square for the more menu: there is one control size. |
| `Button` ghost | No outline, `--ink2` text; hover `--bg3`. |
| `Button` danger | An outline in `--bad`; fills only inside a `ConfirmModal`. |
| `TextInput` / `Select` | White, `--line2` outline, `--controlHeight`, `--r-control` (12px) radius; focus is a 2px emerald ring. Label above at 12px 500; help text below at 12px in `--ink3`. |
| `Badge` | One size (20px: an 18px line inside a 1px edge, 12px 500, full radius), never in upper case. `soft` by default: the tint of the tone behind the word, in the ink of the tone, with a hairline of the same tone as its edge (the record's standing badges beside its name, a status in a row). `primary` is the solid fill with its edge left clear, for a count and the one status that must not be missed. A glyph, when there is one, sits left of the word; the agent's badge always carries the `Sparkles` glyph. |
| `Chip` | A fact a reader can use: a pill on the raised ground with a neutral hairline and a glyph, a link when the fact has a place to go, and a hover that says so. A badge is a tinted status, with an edge in its tone, that nobody presses. |
| `Panel` | Becomes a **zone pane**: `--pane` with a `--paneEdge` and a 20px corner; inside, a title in the display face with its count and its verb, a hairline, and rows. `PanelPlate` (the inset well) becomes a row on `--bg3`. |
| `StatCard` | The **reading card**: `--pane` with the hairline and a 20px corner, 138px tall. The eyebrow is its label, with the basis as words with a dotted underline at the end of the label. The figure is at 26px in the display face, tabular (down to 20px where five share a small row). The basis is at 12.5px at the foot. |
| `FieldGrid` / `FieldRow` | The field row in the details panel: a 96px label with its glyph in `--ink3`, the value in `--ink`, "Add …" in `--ink4` when empty, and the dotted evidence underline when a machine read it. |
| `ListTable` / `DataTable` | Headers at 11.5px 500 in `--ink3`; 44px rows; hairlines; figures aligned right; the selected row on `--accentBg`. Edge to edge inside its zone. |
| `RecordTabs` | Quiet: no rule under the strip; the open tab in `--ink` with a 2px accent underline; counts at 11px in `--ink4`; the Details control at the right end. |
| `SegmentedControl` | `--bg3` track, the pressed part white on the resting shadow, 12px. |
| `Modal` | White, `--r-lg` (20px) radius, `--shadow-pop`, a scrim of `rgba(24,24,27,.4)`. Five intents, each one a fixed width, but never wider than the glass: a confirm at 440px, a form at 600px, a drawer at 560px, a reading drawer at 880px, and the lightbox at 1240px. The drawers come in from the right with the same surface. On a phone a confirm stays a card over the scrim, and the rest become a full-screen sheet. On a desktop, a form, or a drawer with no bands whose body is one form stack followed by its action row, scrolls that stack, so the title and the actions stay put. A drawer with bands scrolls its body between its head and foot. |
| `EmptyState` | Aligned left in its own zone, `--ink3`, one sentence and one verb. |
| `Callout` | The parts of an alert on the ground of the pane: the glyph of the tone and the heading on one line, the heading in the ink of the tone, and a body in plain ink below if there is one. The verbs are aligned right at the end of the heading line, with the dismiss after them. A callout must have a heading; a heading alone is the most common callout. Below 640px the verbs drop under the body. Seven tones: `info`, `accent` (the strong ask, in the brand accent, never indigo), `success`, `warning`, `danger`, `discovery` (new to this reader; no verdict) and `ai`. The tone colours the glyph and the heading only, never a filled box. No `className`. |
| `StagingCard` / `DecisionCard` | The agent's row: `--aiBg`, 14px radius, the indigo mark on its own tile (no label beside it: the tile is the claim), the verdict word at 15px 600 (amber when warning, green when calm), the sentence in `--ink`, `What this rests on · n sources` in `--aiText`, and the agent's verb in `--ai`. A staged change is a row with a dashed `--aiLine` edge, Accept and Dismiss. |
| `Kbd` | 10.5px in `--ink4` with a `--line` outline, 4px radius. On the search field and the ask field. |
| `Spine` | The product's own spine: per stop a date, a 2px accent rule with a 9px dot at its start, a title, and a detail. The gap stop is at 1.5× width, as a 26px amber day count over a dashed amber rule with no dot. Today is a 2px black bar with TODAY and the date under it, and dotted grey and hollow dots come after it. |
| `RecordTimeline` | The rail: a 76px date column in tabular figures, and a 1px rail of full height with a mark per kind (solid, hollow, indigo, dashed indigo, a glyph in a circle for a thread). Then the kind in upper case, direction words, title, the message text cut to three lines, and a meta line; threads as a card on the rail. |
| `RelationshipMap` | Three lanes at 184/200/184 with 72px gutters; nodes with round corners by kind, and a dashed amber gap node. Route edges have bands, strong / developing / cold, and being part of something is a hairline. A selected node lights its route and fades the rest. A 280px panel holds the best route and the one write. |
| `EvidenceMark` / `Citations` | A **source chip**: `--bg3`, 11.5px, a note glyph and a short label ("email · 1 Sep"). On hover or focus it opens a popover with the quote it rests on, in an indigo block ruled on the left, and the origin line under it. Every claim the agent makes carries them; they are how a reader checks the 360 without leaving it. |
| `Stepper` | Steps as 24px pills on one line, with a `›` between: done on `--accentBg`, the current one filled `--accent`, and the rest with outlines. The rule ("A terminal stage asks first") sits at the right in `--ink4`. |

## 9. Motion

The current times and curves stay (`--dur-tap` 90ms, `--dur-state` 140ms, `--dur-move` 200ms,
`--dur-enter` 360ms with a 40ms stagger, `--ease-out`, `--ease-spring`). What this language adds is
where motion is *spent*:

- A page's sections come in with the stagger, from a resting state you can see (an 8px move up and a
  fade), never from `opacity: 0`.
- A row lights up on hover in `--dur-state`; nothing else on a row moves.
- The agent's card, when it arrives, draws its dashed edge in over `--dur-enter`. That is the one
  show of motion, and it reports a fact: a proposal just landed.
- A reading whose value changed counts to the new figure over `--dur-move`. A row just saved lights
  `--accentWash` once and fades over `--dur-enter`.
- Only `transform` and `opacity` move, with one case apart: a page column that arrives or leaves
  (the sidebar rail, the record's details pane). It moves on its own grid track, and the gutter beside
  it, over `--dur-move`.
- That move is short, and a reader makes it many times a day. The work column next to it has to change its layout
  as the track goes, instead of a pane moving over it and covering it. Nothing else
  moves its own layout.
- `prefers-reduced-motion` takes every one of these to its end state at once.

## 10. Restraint: what a screen in this language does not do

- Does not put a pane inside a pane. A zone is one pane; inside it a title, a hairline and rows.
- Does not cast a shadow of its own, a glow or a gradient (the mesh of a monogram, section 3, is the
  one gradient). The resting layer is `--shadow-rest`, from the token, and never from a rule that spells
  its own. The two corner glows on the ground are the only other light, and the only tinted thing
  inside a pane is the agent's row.
- Does not say a fact twice. One home per fact; the rest are links to it.
- Does not fill a button, a row or a surface with emerald. The one primary verb and the wash of a
  selected row are the only cases.
- Does not tint anything indigo that a human wrote.
- Does not set a number in the proportional face.
- Does not centre a title, a section or a table.
- Does not use upper case outside an eyebrow, a reading label and the kind in the timeline. Does not use
  weight 700 outside a heading and a few cases of emphasis.
- Does not add a fourth typeface, an emoji glyph, a gradient hero, or a shadow under a shadow.
- Does not make a component that the catalog already names. Search the tree first.

## 11. Putting it in place

The step-by-step plan, PR by PR, is
[docs/how-to/adopt-the-design.md](docs/how-to/adopt-the-design.md). It names the gates each step
must keep green, and the five record pages zone by zone with every state they carry. It also names
the features a new look must not drop. The order:

1. tokens and type;
2. the small parts (`atoms`);
3. the record primitives and the shell;
4. company as the reference page, with its `e2e` tests written again;
5. contact, deal, lead and project;
6. the tab pages and the maps;
7. the rest, one screen at a time.

## 12. Check list for a new screen

- [ ] The display face on the name, the zone titles, the verdict and a reading's figure; nothing else
      uses it.
- [ ] Every figure is tabular (`t-num`), aligned right in a column; mono only on code.
- [ ] Every zone is one pane with a title, a hairline and rows. The only shadow at rest is
      `--shadow-rest`, and no rule spells one by hand.
- [ ] One control in view with an emerald fill.
- [ ] Anything an agent wrote is a row on `--aiLight` carrying the indigo mark. A row that asks for
      something says "Margince suggests", but never over the answer that nothing needs doing. A
      staged change carries a `--aiMed` dashed edge until a human accepts it.
- [ ] A withheld section says "Hidden from you"; an empty one says what to do.
- [ ] Checked in both themes, and with the sidebar and the details panel folded.
- [ ] The page carries every section the current screen draws (section 7), or says which it dropped
      and why.
- [ ] Every control is from `frontend/src/design-system/`; the catalog table names anything new.
