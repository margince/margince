// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import { en as englishCopy } from "../../i18n/en";
import { StoryProviders } from "../../testing/story-utils";
import { configuredAutomation } from "./automations.fixtures";
import { AutomationRecipientSummary } from "./automations.recipient";

const meta: Meta<typeof AutomationRecipientSummary> = {
  title: "Settings/Data/Automations/Recipient",
  component: AutomationRecipientSummary,
  decorators: [
    (Story) => (
      <StoryProviders>
        <Story />
      </StoryProviders>
    ),
  ],
};
export default meta;
type Story = StoryObj<typeof AutomationRecipientSummary>;

export const RecordOwner: Story = {
  args: {
    action: "create_task",
    automation: {
      ...configuredAutomation("au-2", Date.parse("2026-12-01T10:00:00Z")),
      key: "renewal_reminder",
      params: { recipient: "record_owner" },
    },
  },
  play: async ({ canvasElement }) => {
    await expect(
      within(canvasElement).getByText(
        englishCopy["auto.recipient.tasksToOwner"],
      ),
    ).toBeVisible();
  },
};
export const RecordOwnerDark: Story = {
  ...RecordOwner,
  globals: { theme: "dark" },
};

/** A rule whose tasks go to its authorizer, before anyone authorizes it. */
export const NoAuthorizerYet: Story = {
  args: {
    action: "create_task",
    automation: {
      ...configuredAutomation("au-2", Date.parse("2026-12-01T10:00:00Z")),
      key: "renewal_reminder",
      params: { recipient: "authorizer" },
      owner_id: undefined,
    },
  },
  play: async ({ canvasElement }) => {
    await expect(
      within(canvasElement).getByText(
        englishCopy["auto.recipient.tasksToAuthorizer"],
      ),
    ).toBeVisible();
  },
};
