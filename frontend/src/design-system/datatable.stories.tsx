// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { type CSSProperties, useState } from "react";
import { expect, userEvent, waitFor, within } from "storybook/test";
import { Badge, Button, EmptyState, SectionHeader, TableScroll } from "./atoms";
import { DataTable, type DataTableColumn } from "./datatable";
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
      <Panel title="Pipeline">
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

type DemoMember = {
  id: string;
  name: string;
  email: string;
  role: string;
  lastActive: string;
  invited: boolean;
};

const DEMO_MEMBERS: DemoMember[] = [
  {
    id: "us_1",
    name: "Ana Weber",
    email: "ana.weber@globex.example",
    role: "Admin",
    lastActive: "Today",
    invited: false,
  },
  {
    id: "us_2",
    name: "Marek Janetzke-Hollenstein",
    email: "marek.janetzke-hollenstein@globex.example",
    role: "Sales rep",
    lastActive: "3 days ago",
    invited: false,
  },
  {
    id: "us_3",
    name: "Linh Tran",
    email: "linh@globex.example",
    role: "Viewer",
    lastActive: "",
    invited: true,
  },
];

const MEMBER_COLUMNS: DataTableColumn<DemoMember>[] = [
  {
    key: "name",
    header: "Name",
    render: (member) => (
      <button type="button" className="cell-link" aria-haspopup="dialog">
        {member.name}
      </button>
    ),
  },
  { key: "email", header: "Email", render: (member) => member.email },
  { key: "role", header: "Role", render: (member) => member.role },
  {
    key: "active",
    header: "Last active",
    render: (member) => member.lastActive,
  },
  {
    key: "status",
    header: "Status",
    fold: "end",
    render: (member) =>
      member.invited ? <Badge tone="info">Invited</Badge> : null,
  },
  {
    key: "verbs",
    header: "Actions",
    headerHidden: true,
    fold: "end",
    render: (member) => (
      <Button>{`Remove ${member.name.split(" ")[0]}`}</Button>
    ),
  },
];

function MembersPanel() {
  return (
    <Panel title="Members">
      <PanelBody>
        <PanelIntro>Everyone who can sign in to this workspace.</PanelIntro>
      </PanelBody>
      <DataTable
        fold
        bleed
        label="Members"
        columns={MEMBER_COLUMNS}
        rows={DEMO_MEMBERS}
        rowKey={(member) => member.id}
      />
    </Panel>
  );
}

function cellBox(row: Element, selector: string) {
  return [...row.querySelectorAll(`${selector}:not(:empty)`)].map((cell) =>
    cell.getBoundingClientRect(),
  );
}

// `fold` at a phone's width. The name, the trailing badge and the verb share
// line one, the rest run under them as a caption, and nothing scrolls sideways.
// An empty cell (no status, never active) leaves no gap, and a caption cell
// that wraps starts its line where the caption does.
export const FoldedRecordsPhone: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  render: () => <MembersPanel />,
  play: async ({ canvasElement }) => {
    const box = canvasElement.querySelector(".table-scroll-fold");
    await expect(box).not.toBeNull();
    await expect(box?.scrollWidth).toBeLessThanOrEqual(box?.clientWidth ?? 0);
    const root = document.documentElement;
    await expect(root.scrollWidth).toBeLessThanOrEqual(root.clientWidth);
    const head = canvasElement.querySelector("thead")?.getBoundingClientRect();
    await expect(head?.height).toBeLessThanOrEqual(1);
    for (const row of canvasElement.querySelectorAll("tbody tr")) {
      const [title] = cellBox(row, '[data-fold="title"]');
      for (const end of cellBox(row, '[data-fold="end"]')) {
        await expect(end.top).toBeLessThan(title.bottom);
        await expect(end.left).toBeGreaterThanOrEqual(title.right);
      }
      const rests = cellBox(row, '[data-fold="rest"]');
      for (const rest of rests) {
        await expect(rest.top).toBeGreaterThanOrEqual(title.bottom);
      }
      for (const [index, rest] of rests.entries()) {
        const before = rests[index - 1];
        if (before && rest.top >= before.bottom) {
          await expect(rest.left).toBe(rests[0].left);
        }
      }
    }
  },
};

// The same table with room for its columns keeps one line per row, and the
// name's text starts on the edge its heading does.
export const FoldedRecordsWide: Story = {
  render: () => <MembersPanel />,
  play: async ({ canvasElement }) => {
    const head = canvasElement.querySelector("thead")?.getBoundingClientRect();
    await expect(head?.height).toBeGreaterThan(1);
    const [row] = canvasElement.querySelectorAll("tbody tr");
    const tops = cellBox(row, "td").map((cell) => Math.round(cell.top));
    await expect(new Set(tops).size).toBe(1);
    const heading = canvasElement.querySelector("th");
    const name = row.querySelector(".cell-link")?.getBoundingClientRect();
    if (heading === null || name === undefined) {
      throw new Error("the members table drew no heading or no name");
    }
    const inset = Number.parseFloat(getComputedStyle(heading).paddingLeft);
    await expect(name.left).toBe(heading.getBoundingClientRect().left + inset);
  },
};

type DemoUsage = {
  id: string;
  task: string;
  calls: string;
  input: string;
  output: string;
  cached: string;
  spend: string;
};

const DEMO_USAGE: DemoUsage[] = [
  {
    id: "u_1",
    task: "Drafting a reply",
    calls: "1,204",
    input: "3.1M",
    output: "412k",
    cached: "1.9M",
    spend: "€41.20",
  },
  {
    id: "u_2",
    task: "Reading a mailbox",
    calls: "8,930",
    input: "22.4M",
    output: "1.2M",
    cached: "15.0M",
    spend: "€188.75",
  },
  {
    id: "u_3",
    task: "Meeting brief",
    calls: "96",
    input: "640k",
    output: "88k",
    cached: "210k",
    spend: "€7.10",
  },
];

const USAGE_FIGURES = [
  ["calls", "Calls"],
  ["input", "Input tokens"],
  ["output", "Output tokens"],
  ["cached", "Cached tokens"],
  ["spend", "Spend"],
] as const;

const USAGE_COLUMNS: DataTableColumn<DemoUsage>[] = [
  { key: "task", header: "Task", render: (row) => row.task },
  ...USAGE_FIGURES.map(([key, header]) => ({
    key,
    header,
    align: "end" as const,
    render: (row: DemoUsage) => row[key],
  })),
];

// `stickyFirst`: the play scrolls the box to its far end, and the first column
// stays put on an opaque ground with an edge.
export const PinnedFirstColumn: Story = {
  render: () => (
    <div style={{ maxWidth: 420 }}>
      <Panel title="AI spend">
        <PanelBody>
          <PanelIntro>Scroll the table sideways; the task stays.</PanelIntro>
        </PanelBody>
        <DataTable
          bleed
          stickyFirst
          label="Spend by task"
          columns={USAGE_COLUMNS}
          rows={DEMO_USAGE}
          rowKey={(row) => row.id}
        />
      </Panel>
    </div>
  ),
  play: async ({ canvasElement }) => {
    const box = canvasElement.querySelector<HTMLElement>(
      ".table-scroll-sticky",
    );
    if (box === null) {
      throw new Error("the pinned table drew no scroll box");
    }
    box.scrollLeft = box.scrollWidth;
    await waitFor(() => expect(box.scrollLeft).toBeGreaterThan(0));
    const edge = box.getBoundingClientRect().left;
    const [row] = box.querySelectorAll("tbody tr");
    const [first, second] = row.querySelectorAll("td");
    await expect(Math.round(first.getBoundingClientRect().left)).toBe(
      Math.round(edge),
    );
    await expect(second.getBoundingClientRect().left).toBeLessThan(
      first.getBoundingClientRect().right,
    );
    await expect(getComputedStyle(first).backgroundImage).not.toBe("none");
  },
};

type DemoRun = { id: string; rule: string; when: string; result: string };
const RUNS: DemoRun[] = [
  {
    id: "r1",
    rule: "Welcome new leads",
    when: "Today, 09:12",
    result: "Sent 3 emails",
  },
  {
    id: "r2",
    rule: "Nudge a quiet deal",
    when: "Yesterday, 17:40",
    result: "Skipped: deal was won",
  },
  {
    id: "r3",
    rule: "Tag inbound replies",
    when: "Yesterday, 08:05",
    result: "Tagged 12 messages",
  },
];

// `detail`: each row opens a full-width row under itself, from its trailing
// chevron or a press anywhere else on the row.
function RunsPanel() {
  return (
    <Panel title="Rule runs">
      <DataTable<DemoRun>
        bleed
        fold
        label="Rule runs"
        rows={RUNS}
        rowKey={(run) => run.id}
        columns={[
          { key: "rule", header: "Rule", render: (run) => run.rule },
          { key: "when", header: "When", render: (run) => run.when },
        ]}
        detail={{
          header: "Detail",
          toggleLabel: (run) => `Show the run of ${run.rule}`,
          render: (run) => <p>{run.result}</p>,
        }}
      />
    </Panel>
  );
}

export const RowDetail: Story = {
  render: () => <RunsPanel />,
  play: async ({ canvasElement }) => {
    const toggle = await within(canvasElement).findByRole("button", {
      name: "Show the run of Welcome new leads",
    });
    await userEvent.click(toggle);
    await expect(toggle.getAttribute("aria-expanded")).toBe("true");
  },
};
export const RowDetailDark: Story = {
  ...RowDetail,
  globals: { theme: "dark" },
};
export const RowDetailPhone: Story = {
  ...RowDetail,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};
