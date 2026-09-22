// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { type CSSProperties, useState } from "react";
import { Badge, EmptyState, SectionHeader } from "./atoms";
import { DataTable } from "./datatable";

// A generic component takes no `component` here: Storybook would have to infer
// `Row` from nothing to derive the args table.
const meta: Meta = {
  title: "Design System/DataTable",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

const stack: CSSProperties = {
  display: "flex",
  flexDirection: "column",
  gap: "1rem",
};

type DemoDeal = {
  id: string;
  name: string;
  stage: string;
  weighted: string;
};

const DEMO_DEALS: DemoDeal[] = [
  {
    id: "dl_1",
    name: "Globex renewal",
    stage: "Proposal",
    weighted: "48,000 EUR",
  },
  {
    id: "dl_2",
    name: "Initech platform",
    stage: "Qualify",
    weighted: "12,500 EUR",
  },
  {
    id: "dl_3",
    name: "Umbrella expansion",
    stage: "Negotiation",
    weighted: "156,000 EUR",
  },
];

const DEAL_COLUMNS = [
  { key: "name", header: "Deal", render: (deal: DemoDeal) => deal.name },
  {
    key: "stage",
    header: "Stage",
    render: (deal: DemoDeal) => <Badge tone="accent">{deal.stage}</Badge>,
  },
  {
    key: "weighted",
    header: "Weighted",
    render: (deal: DemoDeal) => <span className="t-num">{deal.weighted}</span>,
  },
];

// onRowClick is what turns a row into a link, so the story has to supply one
// and show that it fired — a cursor change alone is not evidence.
function DealTableDemo() {
  const [opened, setOpened] = useState<DemoDeal | null>(null);
  return (
    <div style={stack}>
      <DataTable
        label={"Deals"}
        columns={DEAL_COLUMNS}
        rows={DEMO_DEALS}
        rowKey={(deal) => deal.id}
        onRowClick={setOpened}
      />
      <span className="t-caption">
        {opened
          ? `Row opened: ${opened.name}`
          : "Click a row — onRowClick is what makes it a link."}
      </span>
    </div>
  );
}

// Rows and no rows. The empty table is the state a screen actually reaches
// first, and it is header-only by design: DataTable never invents a message,
// so the screen pairs it with an EmptyState of its own.
export const Tables: Story = {
  render: () => (
    <div style={stack}>
      <DealTableDemo />
      <SectionHeader title="No rows" />
      <DataTable
        label={"Deals"}
        columns={DEAL_COLUMNS}
        rows={[]}
        rowKey={(deal) => deal.id}
      />
      <EmptyState>No deals in this pipeline yet.</EmptyState>
    </div>
  ),
};
