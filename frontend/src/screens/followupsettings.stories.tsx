// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import type { GrantSpec } from "../testing/mefixture";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../testing/story-utils";
import { FollowUpSettingsCard } from "./followupsettings";

function story(allow: GrantSpec) {
  return () => {
    installFetchStub({
      "GET /me": meRoute(allow),
      "GET /activities/follow-up-settings": () =>
        jsonResponse({ follow_up_after_days: 2 }),
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
} satisfies Meta<typeof FollowUpSettingsCard>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Admin: Story = {
  render: story({ installation_settings: ["read", "update"] }),
};

// A seat that may not change it reads why under the box.
export const Reader: Story = {
  render: story({ installation_settings: ["read"] }),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(
      await canvas.findByText(
        "Only an administrator or operations user can change this.",
      ),
    ).toBeVisible();
    await expect(canvas.getByTestId("follow-up-after-days")).toBeDisabled();
  },
};
