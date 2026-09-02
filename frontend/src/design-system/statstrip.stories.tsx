// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { Badge, StatCard } from "./atoms";
import { StatStrip } from "./statstrip";

// The readings row in the states a record page actually puts it in: a full row,
// a short one, and a row carrying verdicts rather than figures. What each story
// is really checking is that the row reads ACROSS — one plate, one type scale,
// rules and no gaps.
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
      <StatCard label="Consent" value="Allowed" tone="good" dot />
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
      <StatCard label="Health" value="Watch" tone="warn" dot />
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
      <StatCard label="Overdue" value="€48k" tone="danger" dot alert />
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
        <StatCard
          label="The momentum"
          value="Stalled 11 days"
          tone="warn"
          dot
        />
      </StatStrip>
      <StatStrip>
        <StatCard label="Budget" value="€240k" />
        <StatCard label="Spent" value="€181k" />
        <StatCard label="Remaining" value="€59k" />
        <StatCard label="Burn" value="€12k / wk" />
        <StatCard label="Runway" value="5 weeks" tone="warn" dot />
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
