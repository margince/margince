// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, screen, userEvent, within } from "storybook/test";
import type { components } from "../../api/schema";
import { installFetchStub, StoryProviders } from "../../testing/story-utils";
import { InviteDialog } from "./dealroomaccess.dialogs";

type DealRoom = components["schemas"]["DealRoom"];

const ROOM: DealRoom = {
  id: "room-1",
  deal_id: "deal-1",
  title: "Acme expansion",
  state: "live",
  source: "manual",
  captured_by: "u-me",
  version: 1,
  created_at: "2026-08-22T09:00:00Z",
  updated_at: "2026-08-22T09:00:00Z",
};

const meta: Meta = {
  title: "Records/Deal room/Invite",
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj;

function invite() {
  installFetchStub({});
  return (
    <StoryProviders>
      <InviteDialog room={ROOM} open onClose={() => {}} />
    </StoryProviders>
  );
}

export const Empty: Story = { render: invite };

/** An address that is not one, refused on the Email field before sending. */
export const EmailRefused: Story = {
  render: invite,
  play: async () => {
    const dialog = within(await screen.findByRole("dialog"));
    await userEvent.type(dialog.getByLabelText(/^Name/), "Dana Buyer");
    const email = dialog.getByLabelText(/^Email/);
    await userEvent.type(email, "not-an-email");
    await userEvent.click(dialog.getByRole("button", { name: "Invite" }));
    await dialog.findByText("Enter an email address for Email.");
    await expect(email).toHaveAttribute("aria-invalid", "true");
  },
};
