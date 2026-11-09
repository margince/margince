// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../../testing/story-utils";
import { StandingExceptions } from "./standingexceptions";

const meta = {
  title: "Records/Contact 360/Standing exceptions",
  component: StandingExceptions,
  decorators: [
    (Story) => (
      <StoryProviders>
        <Story />
      </StoryProviders>
    ),
  ],
} satisfies Meta<typeof StandingExceptions>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Scoped: Story = {
  args: { contactId: "p-1" },
  render: (args) => {
    installFetchStub({
      "GET /contacts/p-1/consent/allow": () =>
        jsonResponse({
          data: [
            {
              override_id: "exception-1",
              category: "requested_followup",
              reason: "Asked for the next steps during the meeting.",
              authority: "user",
              recorded_at: "2026-06-12T10:00:00Z",
              recorded_by_name: "Alex",
              sending_entity_name: "Example Company",
              may_revoke: true,
            },
          ],
          page: { has_more: false, limit: 50 },
        }),
    });
    return <StandingExceptions {...args} />;
  },
  play: async ({ canvasElement }) => {
    await expect(
      within(canvasElement).findByText("Sending company: Example Company"),
    ).resolves.toBeVisible();
    await expect(
      within(canvasElement).findByRole("button", { name: "Revoke exception" }),
    ).resolves.toBeVisible();
  },
};
export const ScopedDark: Story = { ...Scoped, globals: { theme: "dark" } };
export const Historical: Story = {
  args: { contactId: "p-1" },
  render: (args) => {
    installFetchStub({
      "GET /contacts/p-1/consent/allow": () =>
        jsonResponse({
          data: [
            {
              override_id: "exception-older",
              category: "requested_followup",
              reason: "Requested a follow-up.",
              authority: "user",
              recorded_at: "2026-06-12T10:00:00Z",
              may_revoke: false,
            },
          ],
          page: { has_more: false, limit: 50 },
        }),
    });
    return <StandingExceptions {...args} />;
  },
};
