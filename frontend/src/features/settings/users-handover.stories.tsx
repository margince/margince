// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import { UTC_ZONE } from "../../format/timezone";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../../testing/story-utils";
import { HandoverSummary } from "./users-handover";
import type { User } from "./users-members";

// What a member still owns, as the deactivate dialog says it before they go.
// Their webhooks are counted with their records: each stops when they leave.

const MEMBER: User = {
  id: "10000000-0000-4000-8000-000000000001",
  display_name: "Robin Example",
  email: "robin@example.test",
  timezone: UTC_ZONE,
  status: "active",
  is_agent: false,
};

function renderSummary() {
  installFetchStub({
    [`GET /users/${MEMBER.id}/handover`]: () =>
      jsonResponse({
        owned: {
          lead: 4,
          contact: 0,
          company: 0,
          deal: 16,
          project: 0,
          task: 3,
          webhook: 2,
        },
      }),
  });
  return (
    <StoryProviders>
      <HandoverSummary member={MEMBER} enabled />
    </StoryProviders>
  );
}

const meta: Meta<typeof HandoverSummary> = {
  title: "Settings/Members/Handover summary",
  component: HandoverSummary,
};
export default meta;
type Story = StoryObj<typeof HandoverSummary>;

export const OwnsRecordsAndWebhooks: Story = {
  render: renderSummary,
  play: async ({ canvasElement }) => {
    await expect(
      await within(canvasElement).findByText(/2 webhooks/),
    ).toBeVisible();
  },
};

export const OwnsRecordsAndWebhooksDark: Story = {
  ...OwnsRecordsAndWebhooks,
  globals: { theme: "dark" },
};
