import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, waitFor, within } from "storybook/test";
import type { components } from "../api/schema";
import { en as englishCopy } from "../i18n/en";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../testing/story-utils";
import { ChannelOutboundPreview } from "./channeloutboundpreview";

const record = "018f3a1b-0000-7000-8000-00000000a001";
const channel: components["schemas"]["ChannelLink"] = {
  id: "018f3a1b-0000-7000-8000-00000000a002",
  provider: "slack",
  channel_slug: "C_PILOT",
  channel_name: "Pilot delivery",
  channel_url: "https://fixture.slack.com/archives/C_PILOT",
  status: "active",
  verbosity: "important",
  message_detail: "full",
  created_at: "2026-06-01T00:00:00Z",
};
const preview: components["schemas"]["ChannelDeliveryPreview"] = {
  link_id: channel.id,
  version: 7,
  provider: "slack",
  workspace: "Sales team",
  channel_name: channel.channel_name,
  audience: [{ id: "U_JORDAN", name: "Jordan Lee" }],
  overview_text: "Berlin pilot: Legal approval is required before starting.",
  update_text:
    "Jules accepted 14 June. Security now requests 12 June; Jules has not accepted the earlier date.",
  pending_updates: 2,
  excluded_updates: 1,
  verbosity: "important",
  message_detail: "full",
};
const meta = {
  title: "Records/Team chat/Outbound preview",
  component: ChannelOutboundPreview,
  decorators: [
    (Story) => {
      installFetchStub({
        [`GET /channel-links/deal/${record}/delivery-preview`]: () =>
          jsonResponse(preview),
        [`GET /messenger/slack/deal/${record}/team`]: () =>
          jsonResponse({
            members: [
              {
                crm_id: record,
                kind: "user",
                name: "Jordan Lee",
                member_id: "U_JORDAN",
                state: "matched",
              },
            ],
          }),
      });
      return (
        <StoryProviders>
          <Story />
        </StoryProviders>
      );
    },
  ],
  args: {
    channel,
    recordType: "deal",
    recordId: record,
    busy: false,
    error: null,
    onClose: fn(),
    onConfirm: fn(),
  },
} satisfies Meta<typeof ChannelOutboundPreview>;
export default meta;
type Story = StoryObj<typeof meta>;
export const FullOutcomeAndAudience: Story = {
  play: async () => {
    const outcome = await within(document.body).findByText(
      /Jules accepted 14 June/,
    );
    await waitFor(() => expect(outcome).toBeVisible());
    const audience = within(document.body).getByRole("list", {
      name: englishCopy["channelPreview.audience"],
    });
    await expect(within(audience).getByText("Jordan Lee")).toBeVisible();
  },
};
export const FullOutcomeAndAudienceDark: Story = {
  ...FullOutcomeAndAudience,
  globals: { theme: "dark" },
};
