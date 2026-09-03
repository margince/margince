// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { Building2, Globe, Link2, MapPin, Users } from "lucide-react";
import { LocaleProvider } from "../i18n";
import { BarList, Chip, Meter, Sparkline } from "./readings";

// The three reading primitives: a proportion, a series, an attribute.
const meta: Meta = {
  title: "Design System/Readings",
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
        <Meter value={3} max={10} label="Payment behaviour" tone="warn" />
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
// that owns its spacing. Read it in both themes — the track is `--bgCard` and a
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

// The caller's whole as the denominator: four stages of a pipeline that holds
// more than they add up to, so no bar claims to be everything.
export const BarsAgainstAWhole: Story = {
  render: () => (
    <BarList
      label="Open pipeline by stage"
      max={200}
      rows={[
        { key: "qualified", label: "Qualified", value: 80, amount: "€80,000" },
        { key: "proposal", label: "Proposal", value: 45, amount: "€45,000" },
        { key: "late", label: "Slipped", value: 12, amount: "€12,000", tone: "warn" },
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
