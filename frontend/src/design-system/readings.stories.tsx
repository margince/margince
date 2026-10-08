// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { Building2, Globe, Hash, Link2, MapPin, Users } from "lucide-react";
import { LocaleProvider } from "../i18n";
import { Badge } from "./atoms";
import { BarList, Chip, Meter, SegmentBar, Sparkline } from "./readings";

// The three reading primitives: a proportion, a series, an attribute.
const meta: Meta = {
  title: "Components/Text and data display/Readings",
  parameters: { layout: "padded" },
  decorators: [
    (Story) => (
      <LocaleProvider initial="en">
        <div style={{ maxWidth: 480, display: "grid", gap: "var(--space-5)" }}>
          <Story />
        </div>
      </LocaleProvider>
    ),
  ],
};
export default meta;

type Story = StoryObj;

// Value and max are the two halves of one fact, so the bar and the label
// beside it are drawn from the same pair and cannot disagree.
export const Meters: Story = {
  render: () => (
    <>
      <div>
        <p className="t-caption">7 of 9 inputs present</p>
        <Meter value={7} max={9} label="Dossier completeness" />
      </div>
      <div>
        <p className="t-caption">Payment behaviour — low is the bad end</p>
        <Meter value={3} max={10} label="Payment behavior" tone="warning" />
      </div>
      <div>
        <p className="t-caption">Nothing measured yet</p>
        <Meter value={0} max={0} label="Coverage" />
      </div>
      <div>
        <p className="t-caption">A reading with no low-is-bad end</p>
        <Meter value={6} max={8} label="Growth fit" flat />
      </div>
      <div>
        <p className="t-caption">
          A remainder that is itself a value — overdue against open, where what
          is left is money that is simply not late yet
        </p>
        <Meter
          value={24396}
          max={35203}
          label="Overdue share of the open balance"
          tone="danger"
          restTone="accent"
        />
      </div>
    </>
  ),
};

// dense against default, one under the other, because the size is only legible
// as a comparison: the default bar stands alone in a column and pays for its own
// interval above and below, while a dense one is the label's own bar in a row
// that owns its spacing. Read it in both themes — the track is `--bgInset` and a
// 6px band of it sits differently against the panel in dark.
export const DenseMeters: Story = {
  render: () => (
    <>
      <div>
        <p className="t-caption">Default — a bar standing on its own</p>
        <Meter value={6} max={8} label="Growth fit, default" flat />
        <Meter value={3} max={8} label="Transformation need, default" flat />
      </div>
      <div>
        <p className="t-caption">
          dense — one row per dimension, the label and its reading on one line
          and the bar under them
        </p>
        <p className="t-caption">Growth fit</p>
        <Meter value={6} max={8} label="Growth fit, dense" flat dense />
        <p className="t-caption">Transformation need</p>
        <Meter
          value={3}
          max={8}
          label="Transformation need, dense"
          flat
          dense
        />
      </div>
    </>
  ),
};

export const Sparklines: Story = {
  render: () => (
    <>
      <Sparkline
        points={[12, 9, 14, 11, 18, 7, 12]}
        label="Days paid after due, last six months"
      />
      <Sparkline points={[8, 8, 8, 8]} label="Unchanged over four months" />
    </>
  ),
};

export const Chips: Story = {
  render: () => (
    <div style={{ display: "flex", flexWrap: "wrap", gap: "var(--space-2)" }}>
      <Chip icon={Globe} href="https://glazedfrog.example">
        glazedfrog.example
      </Chip>
      <Chip icon={Link2} href="https://www.linkedin.com/company/example">
        LinkedIn
      </Chip>
      <Chip icon={MapPin}>London, UK</Chip>
      <Chip icon={Building2}>Building products</Chip>
      <Chip icon={Users}>51–200 employees</Chip>
    </div>
  ),
};

// The row a chip and a badge share: a name and the marks on it, one line.
const NAME_CELL = {
  display: "flex",
  alignItems: "center",
  gap: "var(--space-2)",
};

// dense against default, each in the cell it is drawn for, because the size is
// only legible as a comparison: on its own the default chip looks right, and it
// is beside the badge that it reads as the loudest thing in a cell whose subject
// is the NAME. Read it in both themes — the chip carries a border and the badge
// a fill, and the two sit differently against a dark ground.
export const DenseChips: Story = {
  render: () => (
    <>
      <div>
        <p className="t-caption">Default — the chip in a record's head</p>
        <span style={NAME_CELL}>
          <strong>Northwind Traders</strong>
          <Chip icon={Hash}>NWT-4</Chip>
          <Badge tone="warning">Archived</Badge>
        </span>
      </div>
      <div>
        <p className="t-caption">dense — the same chip in a table row</p>
        <span style={NAME_CELL}>
          <strong>Northwind Traders</strong>
          <Chip icon={Hash} dense>
            NWT-4
          </Chip>
          <Badge tone="warning">Archived</Badge>
        </span>
      </div>
    </>
  ),
};

// A ranking: several bars on ONE denominator, which is the whole difference
// between this and a column of Meters.
export const Bars: Story = {
  render: () => (
    <BarList
      label="Deals by stage"
      rows={[
        { key: "qualified", label: "Qualified", value: 42, amount: "42" },
        { key: "proposal", label: "Proposal", value: 18, amount: "18" },
        { key: "negotiation", label: "Negotiation", value: 7, amount: "7" },
        { key: "closing", label: "Closing", value: 2, amount: "2" },
      ]}
    />
  ),
};

// The caller's whole as the denominator: four stages drawn from open deals
// that total more than the bars add up to, so no bar claims to be everything.
export const BarsAgainstAWhole: Story = {
  render: () => (
    <BarList
      label="Open deals by stage"
      max={200}
      rows={[
        { key: "qualified", label: "Qualified", value: 80, amount: "€80,000" },
        { key: "proposal", label: "Proposal", value: 45, amount: "€45,000" },
        {
          key: "late",
          label: "Slipped",
          value: 12,
          amount: "€12,000",
          tone: "warning",
        },
      ]}
    />
  ),
};

// One row is a list of one, not a bar at full width with nothing to compare it
// to — worth seeing, because it is what a filtered report often produces.
export const BarsSingleRow: Story = {
  render: () => (
    <BarList
      label="Deals by stage"
      rows={[{ key: "only", label: "Qualified", value: 6, amount: "6" }]}
    />
  ),
};

// The empty report. A real answer, and the case where a shared denominator is
// a division by zero if nobody guarded it.
export const BarsEmpty: Story = {
  render: () => <BarList label="Deals by stage" rows={[]} />,
};

// A fill holding a stricter measure inside it, on one scale per group: the
// weighted worth inside the open value, the median inside the 75th percentile.
// The last pair is the edge — a part equal to its whole fills the bar solid.
export const MetersWithAPart: Story = {
  render: () => (
    <>
      <div>
        <p className="t-caption">Open value, weighted part solid</p>
        <Meter
          value={96_400}
          part={19_280}
          max={124_000}
          label="Qualify"
          dense
        />
        <Meter
          value={124_000}
          part={37_200}
          max={124_000}
          label="Discovery"
          dense
        />
      </div>
      <div>
        <p className="t-caption">75th percentile, median part solid</p>
        <Meter value={68} part={41} max={100} label="Won" dense />
        <Meter value={90} part={90} max={100} label="Lost" dense />
      </div>
    </>
  ),
};

// Disjoint parts of one total with a target marked across them. The call lands
// inside the parts in the first frame and past them in the second, which leaves
// recessed track between the last part and the mark.
export const SegmentBars: Story = {
  render: () => (
    <>
      <SegmentBar
        label="How the period is made up"
        parts={[
          { key: "won", label: "Already won", value: 92_000, amount: "€92K" },
          {
            key: "evidence",
            label: "Evidence",
            value: 148_000,
            amount: "€148K",
          },
          {
            key: "best",
            label: "Best case beyond evidence",
            value: 112_000,
            amount: "€112K",
          },
        ]}
        marker={{
          key: "call",
          label: "Current call",
          value: 200_000,
          amount: "€200K",
        }}
      />
      <SegmentBar
        label="How the period is made up"
        parts={[
          { key: "won", label: "Already won", value: 40_000, amount: "€40K" },
          { key: "evidence", label: "Evidence", value: 60_000, amount: "€60K" },
        ]}
        marker={{
          key: "call",
          label: "Current call",
          value: 180_000,
          amount: "€180K",
        }}
      />
    </>
  ),
};

export const LongStageNames: Story = {
  render: () => (
    <BarList
      label="Open pipeline by stage"
      rows={[
        {
          key: "legal",
          label: "Procurement, legal and security approval",
          value: 25000000,
          amount: "€250k",
        },
        {
          key: "pilot",
          label: "Technical pilot and stakeholder alignment",
          value: 17000000,
          amount: "€170k",
        },
      ]}
    />
  ),
};
