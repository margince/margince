// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, waitFor, within } from "storybook/test";
import type { components } from "../api/schema";
import { en as englishCopy } from "../i18n/en";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../testing/story-utils";
import { ChannelDeliveryRecovery } from "./channeldeliveryreview";

const record = "018f3a1b-0000-7000-8000-00000000a001";
const link = "018f3a1b-0000-7000-8000-00000000a002";
const channel: components["schemas"]["ChannelLink"] = {
  id: link,
  provider: "slack",
  channel_slug: "C_FIXTURE",
  channel_name: "Acme account team",
  channel_url: "https://fixture.slack.com/archives/C_FIXTURE",
  verbosity: "important",
  status: "active",
  created_at: "2026-06-01T00:00:00Z",
};
const meta = {
  title: "Records/Team chat/Delivery review",
  component: ChannelDeliveryRecovery,
  decorators: [
    (Story) => (
      <StoryProviders>
        <Story />
      </StoryProviders>
    ),
  ],
} satisfies Meta<typeof ChannelDeliveryRecovery>;
export default meta;
type Story = StoryObj<typeof meta>;
function renderReview(paused: boolean) {
  return () => {
    installFetchStub({
      [`GET /channel-links/deal/${record}/delivery-review`]: () =>
        jsonResponse({
          link_id: link,
          overview_card_paused: paused,
          deliveries: paused
            ? []
            : [
                {
                  id: "018f3a1b-0000-7000-8000-00000000a003",
                  state: "unknown",
                  created_at: "2026-06-01T09:00:00Z",
                  overview_card: false,
                  update_count: 3,
                },
              ],
        }),
    });
    return (
      <ChannelDeliveryRecovery
        channel={{ ...channel, overview_card_paused: paused }}
        recordType="deal"
        recordId={record}
      />
    );
  };
}
export const UncertainUpdates: Story = {
  args: { channel, recordType: "deal", recordId: record },
  render: renderReview(false),
  play: async ({ canvasElement }) => {
    await userEvent.click(
      within(canvasElement).getByRole("button", {
        name: englishCopy["channelDelivery.review"],
      }),
    );
    const dialog = await within(document.body).findByRole("dialog", {
      name: englishCopy["channelDelivery.title"],
    });
    await waitFor(() => expect(dialog).toBeVisible());
  },
};
export const PausedOverview: Story = {
  ...UncertainUpdates,
  render: renderReview(true),
  play: async ({ canvasElement }) => {
    await userEvent.click(
      within(canvasElement).getByRole("button", {
        name: englishCopy["channelDelivery.restart"],
      }),
    );
    const dialog = await within(document.body).findByRole("dialog", {
      name: englishCopy["channelDelivery.restartTitle"],
    });
    await waitFor(() => expect(dialog).toBeVisible());
  },
};
export const UncertainUpdatesDark: Story = {
  ...UncertainUpdates,
  globals: { theme: "dark" },
};
