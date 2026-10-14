// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, screen, userEvent, within } from "storybook/test";
import { Button, Modal, OverflowMenu } from "./atoms";
import { Heading } from "./heading";

// The panel hangs off the trigger's end, so the trigger sits at the right edge
// with the height it drops into reserved.
const meta: Meta<typeof OverflowMenu> = {
  title: "Components/Overlays and layering/Overflow menu",
  component: OverflowMenu,
  parameters: { layout: "padded" },
  decorators: [
    (Story) => (
      <div
        style={{
          display: "flex",
          justifyContent: "flex-end",
          alignItems: "flex-start",
          minHeight: "18rem",
        }}
      >
        <Story />
      </div>
    ),
  ],
};
export default meta;

type Story = StoryObj<typeof OverflowMenu>;

// The items mount on the first press, into a panel portalled to the body, so
// they are found through `screen` rather than the canvas.
const openPanel: Story["play"] = async ({ canvasElement }) => {
  const trigger = await within(canvasElement).findByRole("button", {
    name: "More actions",
  });
  await userEvent.click(trigger);
  await screen.findByRole("button", { name: "Merge with…" });
  await expect(trigger).toHaveAttribute("aria-expanded", "true");
};

export const Closed: Story = {
  render: () => (
    <OverflowMenu label="More actions">
      <Button>Merge with…</Button>
      <Button variant="danger">Archive</Button>
    </OverflowMenu>
  ),
};

// Every item shape one menu holds: a label that wraps, a SET row it does not
// close under, a refused row, and the destructive verb.
export const Open: Story = {
  render: () => (
    <OverflowMenu label="More actions">
      <Button>Email everyone on this account</Button>
      <Button>Merge with…</Button>
      <Button aria-expanded="true">Runs</Button>
      <Button>
        Set up the partner programme for this account and its subsidiaries
      </Button>
      <Button reason="An archived account takes no writes.">Export</Button>
      <Button variant="danger">Archive</Button>
    </OverflowMenu>
  ),
  play: openPanel,
};

// A pending item leads with its busy mark, and every other label keeps the
// same left edge beside it.
export const WithAnItemInFlight: Story = {
  render: () => (
    <OverflowMenu label="More actions">
      <Button pending>Pause the room</Button>
      <Button>Merge with…</Button>
      <Button variant="danger">Archive</Button>
    </OverflowMenu>
  ),
  play: openPanel,
};

// Inside a drawer the panel opens over the drawer, where a pointer reaches it.
export const InsideADrawer: Story = {
  tags: ["uat-phone"],
  render: () => (
    <Modal open onClose={() => {}} labelledBy="menu-drawer" intent="drawer">
      <Heading size="large" id="menu-drawer" className="t-h2 modal-title">
        Senders
      </Heading>
      <OverflowMenu label="Actions for news@example.com">
        <Button>Mark as business</Button>
        <Button variant="danger">Exclude</Button>
      </OverflowMenu>
    </Modal>
  ),
  play: async () => {
    await userEvent.click(
      await screen.findByRole("button", {
        name: "Actions for news@example.com",
      }),
    );
    const item = await screen.findByRole("button", {
      name: "Mark as business",
    });
    const box = item.getBoundingClientRect();
    await expect(
      document.elementFromPoint(box.x + box.width / 2, box.y + box.height / 2),
    ).toBe(item);
  },
};
