// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { expect, within } from "storybook/test";
import { Button } from "./atoms";
import { Panel, PanelBody, PanelIntro } from "./panel";
import { createPortalSlot, SlotFill } from "./portalslot";

const DemoSlot = createPortalSlot();

// The host draws the box in its head; the body under it fills it.
function Host() {
  const [slot, setSlot] = useState<HTMLElement | null>(null);
  return (
    <Panel title="Host" titleAction={<span ref={setSlot} />}>
      <DemoSlot.Provider value={slot}>
        <PanelBody>
          <PanelIntro>The verb below is written here.</PanelIntro>
          <SlotFill slot={DemoSlot} className="demo-fill">
            <Button>Add</Button>
          </SlotFill>
        </PanelBody>
      </DemoSlot.Provider>
    </Panel>
  );
}

const meta = {
  title: "Components/Layout and structure/Portal slot",
  component: SlotFill,
  args: { slot: DemoSlot, className: "demo-fill", children: null },
} satisfies Meta<typeof SlotFill>;
export default meta;
type Story = StoryObj<typeof meta>;

export const InItsSlot: Story = {
  render: () => <Host />,
  play: async ({ canvasElement }) => {
    const head = canvasElement.querySelector(".panel-head");
    if (!(head instanceof HTMLElement)) throw new Error("no panel head");
    await expect(
      await within(head).findByRole("button", { name: "Add" }),
    ).toBeVisible();
  },
};

// No host: the content draws where it is written.
export const InPlace: Story = {
  render: () => (
    <SlotFill slot={DemoSlot} className="demo-fill">
      <Button>Add</Button>
    </SlotFill>
  ),
  play: async ({ canvasElement }) => {
    await expect(
      within(canvasElement).getByRole("button", { name: "Add" }),
    ).toBeVisible();
  },
};
