// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { expect, userEvent, within } from "storybook/test";
import { ListPopover, type ListPopoverOption } from "./listpopover";

const COLLEAGUES: readonly ListPopoverOption[] = [
  { id: "u-1", name: "Anna Weber", hint: "Sales" },
  { id: "u-2", name: "Otto Fischer", hint: "Delivery" },
  { id: "u-3", name: "Lena Brandt", hint: "Sales" },
  { id: "u-4", name: "Jonas Keller" },
  { id: "u-5", name: "Mira Hoffmann", hint: "Operations" },
];

const meta = {
  title: "Components/Overlays and layering/List popover",
  component: ListPopover,
  parameters: { layout: "padded" },
  args: {
    label: "Assign owner",
    title: "Colleagues",
    searchLabel: "Search colleagues",
    options: COLLEAGUES,
    selected: "u-2",
    onPick: (_: ListPopoverOption, done: () => void) => done(),
  },
} satisfies Meta<typeof ListPopover>;
export default meta;
type Story = StoryObj<typeof meta>;

// The panel is portalled out of the canvas, so it is found in the document.
const page = () => within(document.body);

async function open({ canvasElement }: { canvasElement: HTMLElement }) {
  await userEvent.click(
    within(canvasElement).getByRole("button", { name: "Assign owner" }),
  );
  await page().findByRole("dialog");
}

export const Closed: Story = {};

export const OpenWithList: Story = { play: open };

// The server is still answering: the search says so rather than showing an
// empty list that reads as "nobody matched".
export const SearchingPending: Story = {
  args: {
    options: undefined,
    search: () => new Promise<ListPopoverOption[]>(() => undefined),
  },
  play: async (context) => {
    await open(context);
    await userEvent.type(page().getByRole("combobox"), "ann");
    await page().findByText("Searching…");
  },
};

export const Empty: Story = {
  args: {
    options: [],
    empty: "Nobody in this workspace can own a project yet.",
  },
  play: open,
};

export const SearchRefused: Story = {
  args: {
    options: undefined,
    search: () => Promise.reject(new Error("offline")),
  },
  play: async (context) => {
    await open(context);
    await userEvent.type(page().getByRole("combobox"), "ann");
    await page().findByRole("alert");
  },
};

function PickedOwner() {
  const [owner, setOwner] = useState<string | null>(null);
  return (
    <p>
      <ListPopover
        label="Assign owner"
        title="Colleagues"
        searchLabel="Search colleagues"
        options={COLLEAGUES}
        onPick={(option, done) => {
          setOwner(option.name);
          done();
        }}
      />{" "}
      {owner && <span>Owner: {owner}</span>}
    </p>
  );
}

export const KeyboardPicked: Story = {
  render: () => <PickedOwner />,
  play: async (context) => {
    await open(context);
    await userEvent.keyboard("{ArrowDown}{ArrowDown}{Enter}");
    const canvas = within(context.canvasElement);
    await canvas.findByText("Owner: Otto Fischer");
    await expect(canvas.getByRole("button", { name: "Assign owner" })).toBe(
      document.activeElement,
    );
  },
};

export const PhoneSheet: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  play: open,
};

export const Dark: Story = {
  globals: { theme: "dark" },
  play: open,
};
