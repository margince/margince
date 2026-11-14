// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../testing/story-utils";
import { ChannelPanel } from "./channelpanel";

const meta: Meta<typeof ChannelPanel> = {
  title: "Records/Deal 360/Team chat",
  component: ChannelPanel,
};
export default meta;
type Story = StoryObj<typeof ChannelPanel>;

const DEAL_ID = "018f3a1b-0000-7000-8000-00000000c001";

const link = {
  id: "018f3a1b-0000-7000-8000-00000000c101",
  provider: "dispact",
  channel_slug: "deal-acme-renewal",
  channel_name: "Acme renewal",
  channel_url: "https://chat.example.com/acme/channels/deal-acme-renewal",
  verbosity: "important",
  status: "active",
  deal_id: DEAL_ID,
  created_at: "2026-11-02T09:00:00Z",
};

// One story per state the read can answer, each for a reader who may edit.
function stateStory(answer: unknown, canEdit = true) {
  return () => {
    installFetchStub({
      [`GET /channel-links/deal/${DEAL_ID}`]: () => jsonResponse(answer),
    });
    return (
      <StoryProviders>
        <ChannelPanel recordType="deal" recordId={DEAL_ID} canEdit={canEdit} />
      </StoryProviders>
    );
  };
}

export const NoBotConnected: Story = { render: stateStory({ state: "off" }) };

export const NoChannelYet: Story = { render: stateStory({ state: "none" }) };

export const Linked: Story = {
  render: stateStory({ state: "linked", link }),
};

export const LinkedReadOnly: Story = {
  render: stateStory({ state: "linked", link }, false),
};

export const Paused: Story = {
  render: stateStory({
    state: "paused",
    link: { ...link, status: "channel_gone" },
  }),
};

export const LinkedDark: Story = {
  globals: { theme: "dark" },
  render: stateStory({ state: "linked", link }),
};
