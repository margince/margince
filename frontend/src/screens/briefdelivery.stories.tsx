// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../testing/story-utils";
import { BriefDeliveryPanel } from "./briefdelivery";

function story(delivery: Record<string, string>) {
  return () => {
    installFetchStub({
      "GET /me": meRoute({}),
      "GET /me/brief-delivery": () => jsonResponse(delivery),
    });
    return (
      <StoryProviders>
        <BriefDeliveryPanel />
      </StoryProviders>
    );
  };
}

const meta = {
  title: "Settings/You/Notifications/Email summaries",
  component: BriefDeliveryPanel,
  parameters: { layout: "padded" },
} satisfies Meta<typeof BriefDeliveryPanel>;
export default meta;
type Story = StoryObj<typeof meta>;

// Never chosen: both rows sit on what happens today, which is by email.
export const NeverChosen: Story = { render: story({}) };

export const WeeklyOff: Story = {
  render: story({ morning_brief_delivery: "email", weekly_delivery: "none" }),
};

export const NeverChosenDark: Story = {
  globals: { theme: "dark" },
  render: story({}),
};

export const NeverChosenPhone: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  render: story({}),
};
