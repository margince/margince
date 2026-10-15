// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../testing/story-utils";
import { FollowUpSettingsCard } from "./followupsettings";

// How long a sent message may go unanswered before its sender's Home asks for a
// follow-up: one number, written as it is committed.

function story(allow: Parameters<typeof meRoute>[0]) {
  return () => {
    installFetchStub({
      "GET /me": meRoute(allow),
      "GET /activities/follow-up-settings": () =>
        jsonResponse({ follow_up_after_days: 3 }),
    });
    return (
      <StoryProviders>
        <FollowUpSettingsCard />
      </StoryProviders>
    );
  };
}

const meta = {
  title: "Settings/Company/Company profile/Follow-ups",
  component: FollowUpSettingsCard,
  parameters: { layout: "padded" },
} satisfies Meta<typeof FollowUpSettingsCard>;
export default meta;

type Story = StoryObj<typeof meta>;

export const Editable: Story = {
  render: story({ installation_settings: ["read", "update"] }),
};

// The number stays readable; the box refuses the change and the panel says why
// once.
export const ReadOnly: Story = {
  render: story({ installation_settings: ["read"] }),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(
      await canvas.findByText(
        "Only an administrator or operations user can change these settings.",
      ),
    ).toBeVisible();
    await expect(canvas.getByTestId("follow-up-after-days")).toBeDisabled();
  },
};
