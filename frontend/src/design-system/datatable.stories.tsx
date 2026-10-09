// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { type CSSProperties, useState } from "react";
import { Badge, EmptyState, SectionHeader, TableScroll } from "./atoms";
import { DataTable } from "./datatable";
import { Panel, PanelBody, PanelIntro } from "./panel";
import { Meter } from "./readings";

// A generic component takes no `component` here: Storybook would have to infer
// `Row` from nothing to derive the args table.
const meta: Meta = {
  title: "Components/Text and data display/Data table",
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

type DemoStage = {
  id: string;
  stage: string;
  deals: number;
  // Whole euros for the bar, and the same figures already spelled for the
  // cells — a story shows the table, not a formatter.
  open: number;
  weighted: number;
  openText: string;
  weightedText: string;
};

const DEMO_STAGES: DemoStage[] = [
  {
    id: "st_1",
    stage: "Qualify",
    deals: 14,
    open: 96_400,
    weighted: 19_280,
    openText: "€96,400.00",
    weightedText: "€19,280.00",
  },
  {
    id: "st_2",
    stage: "Proposal sent",
    deals: 7,
    open: 88_500,
    weighted: 53_100,
    openText: "€88,500.00",
    weightedText: "€53,100.00",
  },
  {
    id: "st_3",
    stage: "Contract sent",
    deals: 2,
    open: 27_800,
    weighted: 25_020,
    openText: "€27,800.00",
    weightedText: "€25,020.00",
  },
];

// Figures set against the end of their column, heading included, and one
// column that grows to hold a bar drawn against the largest row, the weighted
// part solid inside the open value. The bar is aria-hidden because the two
// cells beside it already say both of its figures; its heading still names it.
export const FiguresAndABar: Story = {
  render: () => {
    const scale = Math.max(...DEMO_STAGES.map((row) => row.open));
    return (
      <DataTable
        label="Open deals by stage"
        columns={[
          {
            key: "stage",
            header: "Stage",
            render: (row: DemoStage) => row.stage,
          },
          {
            key: "deals",
            header: "Deals",
            align: "end",
            render: (row: DemoStage) => row.deals,
          },
          {
            key: "bar",
            header: "Weighted share",
            grow: true,
            render: (row: DemoStage) => (
              <div aria-hidden="true">
                <Meter
                  value={row.open}
                  part={row.weighted}
                  max={scale}
                  label={row.stage}
                  dense
                />
              </div>
            ),
          },
          {
            key: "open",
            header: "Open value",
            align: "end",
            render: (row: DemoStage) => row.openText,
          },
          {
            key: "weighted",
            header: "Weighted",
            align: "end",
            render: (row: DemoStage) => row.weightedText,
          },
        ]}
        rows={DEMO_STAGES}
        rowKey={(row) => row.id}
      />
    );
  },
};

// `bleed`: the table stands straight in the Panel, between two bodies. Its rules
// reach the pane's edges and the first column keeps the intro's x.
export const InAPanel: Story = {
  render: () => (
    <Panel title="Deals">
      <PanelBody>
        <PanelIntro>Every open deal in this pipeline.</PanelIntro>
      </PanelBody>
      <DataTable
        bleed
        label="Deals"
        columns={DEAL_COLUMNS}
        rows={DEMO_DEALS}
        rowKey={(deal) => deal.id}
      />
      <PanelBody>
        <PanelIntro>Weighted by each stage's win rate.</PanelIntro>
      </PanelBody>
    </Panel>
  ),
};

// A hand-drawn table bleeds through `TableScroll` the same way, and one too wide
// for its pane scrolls sideways inside it.
export const WideInAPanel: Story = {
  render: () => (
    <div style={{ maxWidth: 420 }}>
      <Panel title="Open deals by stage">
        <PanelBody>
          <PanelIntro>Scroll the table sideways for every figure.</PanelIntro>
        </PanelBody>
        <TableScroll bleed label="Open deals by stage">
          <table className="table">
            <thead>
              <tr>
                <th>Stage</th>
                <th>Owner</th>
                <th>Open value</th>
                <th>Weighted</th>
                <th>Oldest</th>
              </tr>
            </thead>
            <tbody>
              {DEMO_STAGES.map((row) => (
                <tr key={row.id}>
                  <td>{row.stage}</td>
                  <td>{"Marek Janetzke"}</td>
                  <td>{row.openText}</td>
                  <td>{row.weightedText}</td>
                  <td>{"12 March 2026"}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </TableScroll>
      </Panel>
    </div>
  ),
};
