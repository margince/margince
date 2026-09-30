// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { CSSProperties } from "react";
import { useState } from "react";
import { Kbd, SegmentedControl } from "./atoms";

const meta: Meta<typeof SegmentedControl> = {
  title: "Components/Forms and input/Segmented control",
  component: SegmentedControl,
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj<typeof SegmentedControl>;

const row: CSSProperties = {
  display: "flex",
  gap: "0.75rem",
  alignItems: "center",
  flexWrap: "wrap",
};
const stack: CSSProperties = {
  display: "flex",
  flexDirection: "column",
  gap: "1rem",
};

const RANGES = ["month", "quarter", "year"] as const;
type Range = (typeof RANGES)[number];
const RANGE_LABELS: Record<Range, string> = {
  month: "Month",
  quarter: "Quarter",
  year: "Year",
};

const SIDES = ["owner", "team"] as const;
type Side = (typeof SIDES)[number];
const SIDE_LABELS: Record<Side, string> = { owner: "Owner", team: "Team" };

const TABS = ["overview", "contacts", "deals"] as const;
type Tab = (typeof TABS)[number];
const TAB_LABELS: Record<Tab, string> = {
  overview: "360",
  contacts: "Contacts",
  deals: "Deals",
};

// Controlled: without state here the buttons never move.
function ToolbarDemo() {
  const [range, setRange] = useState<Range>("quarter");
  const [side, setSide] = useState<Side>("owner");
  const [tab, setTab] = useState<Tab>("overview");
  return (
    <div style={stack}>
      <div style={{ ...row, justifyContent: "space-between" }}>
        <SegmentedControl
          options={RANGES}
          value={range}
          onChange={setRange}
          labels={RANGE_LABELS}
          label="Reporting range"
        />
        <span className="t-caption">
          Press <Kbd>/</Kbd> to search, <Kbd>Ctrl</Kbd> <Kbd>K</Kbd> for the
          command bar, <Kbd>Esc</Kbd> to close.
        </span>
      </div>
      <SegmentedControl
        options={SIDES}
        value={side}
        onChange={setSide}
        labels={SIDE_LABELS}
        label="Target amount"
      />
      {/* Partial counts: an option with none is not the explicit zero beside
          it. */}
      <SegmentedControl
        options={TABS}
        value={tab}
        onChange={setTab}
        labels={TAB_LABELS}
        counts={{ contacts: 6, deals: 0 }}
        label="Record section"
      />
    </div>
  );
}

// Two options and three: a two-up control has no middle segment, which is
// where divider rules written for three break.
export const Toolbar: Story = {
  render: () => <ToolbarDemo />,
};

const RECORD_TABS = ["overview", "research", "documents"] as const;
type RecordTab = (typeof RECORD_TABS)[number];
const RECORD_TAB_LABELS: Record<RecordTab, string> = {
  overview: "Overview",
  research: "Data and tools",
  documents: "Documents",
};

function MarkedTabsDemo() {
  const [tab, setTab] = useState<RecordTab>("overview");
  return (
    <SegmentedControl
      options={RECORD_TABS}
      value={tab}
      onChange={setTab}
      labels={RECORD_TAB_LABELS}
      label="Record sections"
      marks={{ research: true }}
    />
  );
}

// The dot is `aria-hidden` and never the only carrier: the surface it points
// at states the fact in words.
export const MarkedOption: Story = {
  render: () => <MarkedTabsDemo />,
};
