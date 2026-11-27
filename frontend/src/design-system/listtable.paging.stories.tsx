// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { expect, within } from "storybook/test";
import { type ListColumn, ListTable } from "./listtable";

// The pager under a list, in each state of what the caller knows about the
// list's length. It knows a count the server sent, the rows in hand, or neither.
const meta: Meta = {
  title: "Components/Text and data display/List table paging",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

type Contact = { id: string; name: string };

const COLUMNS: readonly ListColumn<Contact>[] = [
  { key: "name", header: "Name", cell: (row) => row.name, fixed: true },
];

const contacts = (from: number, count: number): Contact[] =>
  Array.from({ length: count }, (_, index) => ({
    id: `c${from + index}`,
    name: `Contact ${from + index + 1}`,
  }));

const COUNTED = 3412;

// A list the server counted, on the page the caller holds. A page past the
// first read comes from the rows the caller read at that page's offset.
function Counted({
  opensOn,
  reading = false,
}: Readonly<{ opensOn: number; reading?: boolean }>) {
  const [page, setPage] = useState(opensOn);
  const from = Math.floor(((page - 1) * 25) / 50) * 50;
  return (
    <ListTable<Contact>
      rows={contacts(0, 50)}
      columns={COLUMNS}
      rowKey={(row) => row.id}
      unit="contacts"
      hasMore
      total={COUNTED}
      page={page}
      onPage={setPage}
      jumped={
        from >= 50
          ? {
              from,
              rows: reading ? [] : contacts(from, Math.min(50, COUNTED - from)),
              settled: !reading,
            }
          : undefined
      }
    />
  );
}

// "1 2 3 … 137": the last page the count names is numbered from the start.
export const CountedFirstPage: Story = {
  render: () => <Counted opensOn={1} />,
  play: async ({ canvasElement }) => {
    await expect(
      within(canvasElement).getByRole("button", { name: "Page 137" }),
    ).toBeVisible();
  },
};

// "1 … 4 5 6 … 137": page one and the last page either side of the window.
export const CountedMiddlePage: Story = {
  render: () => <Counted opensOn={5} />,
};

// "1 … 135 136 137", and Next is off on the last page.
export const CountedLastPage: Story = {
  render: () => <Counted opensOn={137} />,
  play: async ({ canvasElement }) => {
    await expect(
      within(canvasElement).getByRole("button", { name: "Next" }),
    ).toBeDisabled();
  },
};

// The last page while its read at the offset is under way: its number is busy
// and the rows are placeholders.
export const CountedPageBeingRead: Story = {
  render: () => <Counted opensOn={137} reading />,
};

// No count, and more rows to fetch: the pages in hand, then a gap, and Next
// reads on.
export const NotCounted: Story = {
  render: () => (
    <ListTable<Contact>
      rows={contacts(0, 50)}
      columns={COLUMNS}
      rowKey={(row) => row.id}
      unit="contacts"
      hasMore
      onLoadMore={() => undefined}
    />
  ),
};

// Three pages, every row in hand: each page numbered, no gap.
export const ThreePages: Story = {
  render: () => (
    <ListTable<Contact>
      rows={contacts(0, 70)}
      columns={COLUMNS}
      rowKey={(row) => row.id}
      unit="contacts"
    />
  ),
};
