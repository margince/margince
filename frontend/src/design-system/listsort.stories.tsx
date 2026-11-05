// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { expect, userEvent, waitFor, within } from "storybook/test";
import { LocaleProvider, useT } from "../i18n";
import { playOpenViewStep } from "../testing/listview.play";
import { SortStep, sortSummary } from "./listsort";
import type { SortOption } from "./listsurface.dials";
import { ListViewMenu } from "./listviewmenu";
import "./listtable.css";

// The Sort step of a list's View panel holds one order at a time. The one in
// force shows its direction, and the server's own order comes last.

const OPTIONS: readonly SortOption[] = [
  { field: "name", label: "Name" },
  { field: "created_at", label: "Created" },
  { field: "amount", label: "Amount", numeric: true },
];

function SortPanel({ initial }: Readonly<{ initial: string }>) {
  const t = useT();
  const [open, setOpen] = useState(false);
  const [value, setValue] = useState(initial);
  const sort = { value, onChange: setValue };
  return (
    <ListViewMenu
      open={open}
      onOpenChange={setOpen}
      filterCount={0}
      changed={false}
      rows={[
        {
          key: "sort",
          label: t("table.sort"),
          value: sortSummary(sort, OPTIONS, t),
          step: () => <SortStep sort={sort} options={OPTIONS} />,
        },
      ]}
    />
  );
}

const meta = {
  title: "Components/Forms and input/List sort step",
  component: SortPanel,
  parameters: { layout: "padded" },
  decorators: [
    (Story) => (
      <LocaleProvider initial="en">
        <Story />
      </LocaleProvider>
    ),
  ],
} satisfies Meta<typeof SortPanel>;
export default meta;

type Story = StoryObj<typeof meta>;

// Pressing the order in force turns it the other way.
export const NewestFirst: Story = {
  args: { initial: "-created_at" },
  play: async ({ canvasElement }) => {
    const step = within(await playOpenViewStep(canvasElement, "Sort"));
    const created = step.getByRole("radio", { name: "Created descending" });
    await expect(created).toBeChecked();
    await userEvent.click(created);
    await waitFor(() =>
      expect(
        step.getByRole("radio", { name: "Created ascending" }),
      ).toBeChecked(),
    );
  },
};

export const ServerOrder: Story = {
  args: { initial: "" },
  play: async ({ canvasElement }) => {
    const step = within(await playOpenViewStep(canvasElement, "Sort"));
    await expect(
      step.getByRole("radio", { name: "Default order" }),
    ).toBeChecked();
    await userEvent.click(step.getByRole("radio", { name: "Amount" }));
    await waitFor(() =>
      expect(
        step.getByRole("radio", { name: "Amount descending" }),
      ).toBeChecked(),
    );
  },
};
