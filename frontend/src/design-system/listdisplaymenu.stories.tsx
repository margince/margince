// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { expect, screen, userEvent, waitFor, within } from "storybook/test";
import { LocaleProvider } from "../i18n";
import { viewMenuName } from "../testing/listview.names";
import { playOpenView, playOpenViewStep } from "../testing/listview.play";
import type { ListColumn } from "./listcolumn";
import { useDisplayRows } from "./listdisplaymenu";
import { ListViewMenu } from "./listviewmenu";
import "./listtable.css";

// A grid's own rows in View: which optional columns stand, in what order, and
// how tight the rows are.

type Deal = { name: string };

const COLUMNS: readonly ListColumn<Deal>[] = [
  { key: "stage", header: "Stage", cell: () => null },
  { key: "owner", header: "Owner", cell: () => null, initiallyHidden: true },
  { key: "amount", header: "Amount", cell: () => null },
];

function DisplayPanel({ columns }: Readonly<{ columns: readonly string[] }>) {
  const [open, setOpen] = useState(false);
  const [order, setOrder] = useState(
    COLUMNS.filter((column) => columns.includes(column.key)),
  );
  const [hidden, setHidden] = useState<ReadonlySet<string>>(new Set(["owner"]));
  const [dense, setDense] = useState(false);
  const { rows, toggles } = useDisplayRows({
    optional: order,
    declared: columns,
    hidden,
    onToggleColumn: (key) =>
      setHidden((prev) => {
        const next = new Set(prev);
        if (!next.delete(key)) next.add(key);
        return next;
      }),
    onReorder: (keys) =>
      setOrder(keys.flatMap((key) => order.filter((c) => c.key === key))),
    dense,
    onDense: () => setDense((prev) => !prev),
  });
  return (
    <ListViewMenu
      open={open}
      onOpenChange={setOpen}
      filterCount={0}
      changed={false}
      rows={rows}
      toggles={toggles}
    />
  );
}

const meta = {
  title: "Components/Text and data display/List columns and density",
  component: DisplayPanel,
  parameters: { layout: "padded" },
  args: { columns: ["stage", "owner", "amount"] },
  decorators: [
    (Story) => (
      <LocaleProvider initial="en">
        <Story />
      </LocaleProvider>
    ),
  ],
} satisfies Meta<typeof DisplayPanel>;
export default meta;

type Story = StoryObj<typeof meta>;

export const ColumnsStep: Story = {
  play: async ({ canvasElement }) => {
    const step = within(await playOpenViewStep(canvasElement, "Columns"));
    await expect(step.getByRole("checkbox", { name: "Stage" })).toBeChecked();
    const owner = step.getByRole("checkbox", { name: "Owner" });
    await expect(owner).not.toBeChecked();
    await userEvent.click(owner);
    await waitFor(() => expect(owner).toBeChecked());
  },
};

// Compact is a toggle at the panel's end: a pressed row that reads On or Off.
export const CompactToggle: Story = {
  play: async ({ canvasElement }) => {
    const panel = within(await playOpenView(canvasElement));
    const compact = await panel.findByRole("button", { name: "Compact" });
    await waitFor(() => expect(compact).toBeVisible());
    await expect(compact).toHaveAttribute("aria-pressed", "false");
    await userEvent.click(compact);
    await waitFor(() =>
      expect(compact).toHaveAttribute("aria-pressed", "true"),
    );
  },
};

// Off its default, Columns carries a dot, said as "changed".
export const ChangedColumns: Story = {
  play: async ({ canvasElement }) => {
    const columns = within(await playOpenViewStep(canvasElement, "Columns"));
    await userEvent.click(columns.getByRole("checkbox", { name: "Owner" }));
    await userEvent.click(columns.getByRole("button", { name: /^Columns/ }));
    const panel = within(
      await screen.findByRole("dialog", { name: viewMenuName() }),
    );
    const changed = await panel.findByRole("button", {
      name: "Columns, 3 of 3, changed",
    });
    await waitFor(() => expect(changed).toBeVisible());
  },
};

// A grid with no optional column offers the Compact toggle alone.
export const CompactOnly: Story = {
  args: { columns: [] },
  play: async ({ canvasElement }) => {
    const panel = within(await playOpenView(canvasElement));
    const compact = await panel.findByRole("button", { name: "Compact" });
    await waitFor(() => expect(compact).toBeVisible());
    await expect(panel.queryByRole("button", { name: /^Columns/ })).toBeNull();
  },
};
