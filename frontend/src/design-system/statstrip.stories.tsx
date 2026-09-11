// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { Badge, StatCard } from "./atoms";
import { StatStrip } from "./statstrip";

// The readings row in the states a record page actually puts it in: a full row,
// a short one, and a row carrying verdicts rather than figures. What each story
// is really checking is that the row reads ACROSS — equal slots, one type scale
// and air between them.
//
// The scale is the TILE's (atoms.css), not this component's: a slot here draws
// exactly like a free-standing card, which is why none of these stories passes
// a size or a variant. The strip used to carry a figure size of its own and a
// `hero` flag carried a third for the Brief; one reading in three spellings is
// the defect, and the row's own job is only how many slots and where it folds.
const meta: Meta<typeof StatStrip> = {
  title: "Design System/StatStrip",
  component: StatStrip,
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj<typeof StatStrip>;

// Six slots is the row at full width: figures and sentences side by side at one
// size, which is the claim the shape makes.
export const SixSlots: Story = {
  render: () => (
    <StatStrip>
      <StatCard label="Last inbound" value="21 days" />
      <StatCard label="Last outbound" value="Never" />
      <StatCard label="Reciprocity" value="1 in · 0 out" />
      <StatCard label="Open deal" value="None" />
      <StatCard label="Next meeting" value="None" />
      <StatCard label="Consent" value="Allowed" tone="good" />
    </StatStrip>
  ),
};

// Four slots, because this record has four readings — not six with two blank.
// The plate ends where the row ends rather than reserving grey cells.
export const FewerSlots: Story = {
  render: () => (
    <StatStrip>
      <StatCard label="Pipeline" value="€95k" detail="2 open deals" />
      <StatCard
        label="Net invoiced · 12 mo"
        value="€1.2m"
        detail="offline_demo"
      />
      <StatCard label="Payment behaviour" value="typically 4 days early" />
      <StatCard label="Health" value="Watch" tone="warn" onOpen={() => {}} />
    </StatStrip>
  ),
};

// A slot whose figure has a source names it on the label, and a slot that is
// itself bad news tints the whole tile — both inside the row, so the row's one
// scale survives contact with them.
export const SourcedAndAlerting: Story = {
  render: () => (
    <StatStrip>
      <StatCard
        label="Net invoiced"
        value="€1.2m"
        source={<Badge>offline_demo</Badge>}
      />
      <StatCard label="Overdue" value="€48k" tone="danger" alert />
      <StatCard label="Coverage" value="1 colleague" />
    </StatStrip>
  ),
};

// The fold, at the width where it happens. Four readings over three columns and
// five over three are the two counts on record pages today, and neither divides
// — the last slot takes the rest of its row rather than sitting alone beside
// empty cells under a stub of rule.
//
// Narrow the Storybook viewport below 68rem to see it fold; at full width both
// strips are one even row and nothing is stretched.
export const FoldsWithoutAnOrphan: Story = {
  parameters: { viewport: { defaultViewport: "tablet" } },
  render: () => (
    <div style={{ display: "grid", gap: "var(--space-6)" }}>
      <StatStrip>
        <StatCard label="The ask" value="€95k" />
        <StatCard label="The date" value="14 Mar" />
        <StatCard label="The room" value="3 of 5 roles" />
        <StatCard label="The momentum" value="Stalled 11 days" tone="warn" />
      </StatStrip>
      <StatStrip>
        <StatCard label="Budget" value="€240k" />
        <StatCard label="Spent" value="€181k" />
        <StatCard label="Remaining" value="€59k" />
        <StatCard label="Burn" value="€12k / wk" />
        <StatCard label="Runway" value="5 weeks" tone="warn" />
      </StatStrip>
    </div>
  ),
};

// A caveat that belongs to the whole row rather than to one slot: a source read
// to its limit makes EVERY figure above a floor. Attached to one figure it would
// invite the reading where the other three are exact, which is why the plate
// carries it and no slot does.
export const QualifiedRow: Story = {
  render: () => (
    <StatStrip floor="A source was read to its limit, so every figure above is a floor.">
      <StatCard
        label="Customer waiting"
        value="14"
        detail="waiting on an answer"
      />
      <StatCard label="Meetings ahead" value="4" detail="1 needs prep" />
      <StatCard
        label="Promises due"
        value="—"
        detail="promises are not tracked yet"
      />
      <StatCard label="Lead response" value="3" detail="owed a first answer" />
    </StatStrip>
  ),
};

// A SLOT THAT IS ITSELF THE DOOR. Where a reading's cell leads somewhere, the
// slot is the link or the button and the card inside it is the whole of what a
// reader sees: hover one and the pane answers, not a word inside it.
//
// The alternative is what the Brief drew until now — an "Open →" line in every
// card's foot, which is a decorative row on a plate whose argument is that a
// row of readings is taken in at one glance, and five doors all reading "Open"
// are five identical rows in a screen reader's list. A cell that is the control
// announces the reading's own words.
//
// Both kinds are here because they are two different claims: a LINK is an
// address a reader can paste or open in a new tab, and a BUTTON is a move the
// page makes. Neither brings chrome of its own, so the row still reads across.
//
// WHAT TO CHECK, in both themes: no green, no underline, and no difference at
// all between a door and the pipeline button beside it until you point at one.
// base.css draws the product's prose links through `a:not([class])`, which
// scores exactly what `.stat-strip > a` did — so which sheet won came down to
// bundler order, and the Brief shipped five green underlined hyperlinks where
// its readings should be. Hover: the pane's ground and hairline both shift,
// because a ground shift alone on a translucent surface is almost invisible in
// dark.
export const SlotsThatAreDoors: Story = {
  render: () => (
    <StatStrip>
      <a href="#/worklist?filter=all">
        <StatCard
          label="Urgent"
          value="4"
          tone="warn"
          detail="somebody waiting or a promise breaking"
        />
      </a>
      <a href="#/worklist?filter=meetings">
        <StatCard label="Meetings today" value="4" detail="1 needs prep" />
      </a>
      <a href="#/worklist?filter=leads">
        <StatCard
          label="Leads owed a reply"
          value="3"
          detail="owed a first answer"
        />
      </a>
      <button type="button">
        <StatCard
          label="Pipeline · Q3"
          value="€420k"
          detail="€168k weighted · 11 priced"
        />
      </button>
      {/* A figure the read could not finish counting: the `+` is the caveat, on
          the figure it qualifies rather than in a sentence under the row. */}
      <a href="#/worklist?filter=decisions">
        <StatCard
          label="Decisions waiting"
          value="8+"
          detail="somebody is blocked until you answer"
        />
      </a>
    </StatStrip>
  ),
};

// THE NARROW SHAPE: one full-width ROW per reading, and only for a strip whose
// slots are `compact`.
//
// Five 190px boxes two-up is 600px of readings before a phone reader reaches
// what the page is for. As rows the same five facts are ~60px each, the eye
// runs down one column of figures instead of hunting two, and the plate draws
// one hairline between them rather than five borders — a column of bordered
// panes reads as cards to swipe rather than a list to scan.
//
// Read off the CHILDREN with `:has(.stat-card-compact)`, not taken as a second
// prop, so the plate and its slots cannot disagree about which shape this is.
// Nineteen other strips in the product keep their 2-column fold at this width,
// and `SixSlots` above is one of them — open both at the phone viewport and the
// difference is the whole contract.
export const CompactFoldsToRows: Story = {
  name: "Compact slots — the narrow shape",
  globals: { viewport: { value: "phone" } },
  render: () => (
    <StatStrip>
      <a href="#/worklist?filter=all">
        <StatCard
          label="Urgent"
          value="8+"
          tone="warn"
          detail="somebody waiting or a promise breaking"
          density="compact"
        />
      </a>
      <a href="#/worklist?filter=meetings">
        <StatCard
          label="Meetings today"
          value="0"
          detail="on today's calendar"
          density="compact"
        />
      </a>
      <a href="#/worklist?filter=leads">
        <StatCard
          label="Leads owed a reply"
          value="19+"
          detail="owed a first answer"
          density="compact"
        />
      </a>
      <button type="button">
        <StatCard
          label="Pipeline · Q3"
          value="€5m"
          detail="€3.7m weighted · 33 priced"
          density="compact"
        />
      </button>
      <a href="#/worklist?filter=decisions">
        <StatCard
          label="Decisions waiting"
          value="4+"
          detail="somebody is blocked until you answer"
          density="compact"
        />
      </a>
    </StatStrip>
  ),
};
