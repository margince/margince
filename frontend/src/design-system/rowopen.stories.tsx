// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { expect, userEvent, within } from "storybook/test";
import { DataTable } from "./datatable";
import { RowOpen } from "./rowopen";

type Template = { key: string; name: string; refusal?: string };

const TEMPLATES: Template[] = [
  { key: "renewal", name: "Renewal reminder" },
  { key: "recap", name: "Post-meeting recap draft" },
  {
    key: "search",
    name: "Search and retrieval",
    refusal: "Search and retrieval has no settings of its own.",
  },
];

function OpeningTable() {
  const [opened, setOpened] = useState("nothing yet");
  const open = (row: Template) => setOpened(row.name);
  return (
    <>
      <DataTable
        fold
        label="Templates"
        rows={TEMPLATES}
        rowKey={(row) => row.key}
        onRowClick={(row) => row.refusal === undefined && open(row)}
        columns={[
          { key: "name", header: "Name", render: (row) => row.name },
          {
            key: "open",
            header: "Open",
            headerHidden: true,
            align: "end",
            fold: "end",
            render: (row) => (
              <RowOpen
                label={`Open ${row.name}`}
                refusal={row.refusal}
                onOpen={() => open(row)}
              />
            ),
          },
        ]}
      />
      <p role="status">Opened: {opened}</p>
    </>
  );
}

const meta = {
  title: "Components/Text and data display/Row open",
  component: OpeningTable,
} satisfies Meta<typeof OpeningTable>;
export default meta;

type Story = StoryObj<typeof meta>;

export const Default: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", { name: "Open Renewal reminder" }),
    );
    await expect(canvas.getByRole("status")).toHaveTextContent(
      "Renewal reminder",
    );
  },
};

export const Dark: Story = { ...Default, globals: { theme: "dark" } };

export const Phone: Story = {
  ...Default,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};
