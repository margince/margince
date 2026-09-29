// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { expect, userEvent, within } from "storybook/test";
import { SortableList } from "./sortablelist";

// Rows a reader puts in order by hand. The stories hold their own order in
// state, the way a screen holds it in its cache, so a drag or an arrow key
// visibly lands.

type Step = Readonly<{ key: string; name: string }>;

const STEPS: readonly Step[] = [
  { key: "q", name: "Qualified" },
  { key: "d", name: "Discovery" },
  { key: "p", name: "Proposal" },
  { key: "n", name: "Negotiation" },
];

const labels = {
  handle: (item: Step, position: number, count: number) =>
    `Move ${item.name}, step ${position} of ${count}`,
  moved: (item: Step, position: number, count: number) =>
    `${item.name} is now step ${position} of ${count}`,
};

function Ordered({
  reorderable = true,
  busy = false,
  selectedKey,
}: Readonly<{ reorderable?: boolean; busy?: boolean; selectedKey?: string }>) {
  const [order, setOrder] = useState<readonly Step[]>(STEPS);
  return (
    <div style={{ maxWidth: 480 }}>
      <SortableList
        label="Open stages"
        items={order}
        busy={busy}
        selectedKey={selectedKey}
        labels={labels}
        onReorder={
          reorderable
            ? (keys) =>
                setOrder(
                  keys.flatMap((key) => STEPS.filter((s) => s.key === key)),
                )
            : undefined
        }
        renderItem={(item) => <span>{item.name}</span>}
      />
    </div>
  );
}

const meta: Meta<typeof SortableList> = {
  title: "Design System/SortableList",
  component: SortableList,
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof SortableList>;

/** Drag a row by its grip, or focus the grip and press ↑ or ↓. */
export const Reorderable: Story = {
  render: () => <Ordered />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    canvas.getByRole("button", { name: "Move Proposal, step 3 of 4" }).focus();
    await userEvent.keyboard("{ArrowUp}");
    await expect(
      canvas.getByRole("button", { name: "Move Proposal, step 2 of 4" }),
    ).toHaveFocus();
    await expect(
      canvas.getByText("Proposal is now step 2 of 4"),
    ).toBeInTheDocument();
  },
};

/** A reader who may not reorder sees the rows and no grip. */
export const ReadOnly: Story = {
  render: () => <Ordered reorderable={false} />,
};

/** The row the surrounding surface has open, drawn as chosen. */
export const Selected: Story = {
  render: () => <Ordered selectedKey="d" />,
};

/** A write is out: the grips stay focusable and refuse. */
export const Busy: Story = {
  render: () => <Ordered busy />,
};

/** The plates and the grip in dark. */
export const Dark: Story = {
  globals: { theme: "dark" },
  render: () => <Ordered selectedKey="d" />,
};
