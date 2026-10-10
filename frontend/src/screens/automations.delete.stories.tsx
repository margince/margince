// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import { DeleteAutomationAction } from "./automations.delete";
import { configuredAutomations } from "./automations.fixtures";
import { installFetchStub, StoryProviders } from "./story-utils";

function Delete() {
  installFetchStub({
    "DELETE /automations/au-2": () => new Response(null, { status: 204 }),
  });
  return (
    <StoryProviders>
      <DeleteAutomationAction
        automation={configuredAutomations(Date.now())[1]}
      />
    </StoryProviders>
  );
}

const meta = {
  title: "Settings/AI/Automations/Delete an automation",
  component: Delete,
} satisfies Meta<typeof Delete>;
export default meta;

type Story = StoryObj<typeof meta>;

// The confirm names the rule; nothing is written until it is answered.
export const Confirm: Story = {
  play: async ({ canvasElement }) => {
    const user = userEvent.setup();
    await user.click(
      await within(canvasElement).findByRole("button", { name: "Delete" }),
    );
    const dialog = await within(canvasElement.ownerDocument.body).findByRole(
      "dialog",
    );
    await expect(dialog).toHaveTextContent("Renewal reminder");
  },
};

export const ConfirmDark: Story = { ...Confirm, globals: { theme: "dark" } };

// A confirm becomes a bottom sheet on a phone, so its two verbs must stay in reach.
export const ConfirmPhone: Story = {
  ...Confirm,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};
