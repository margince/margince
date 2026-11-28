// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { DealDeliveryHandoff } from "./dealdeliveryhandoff";

const deal = "018f3a1b-0000-7000-8000-00000000a001";
const project = "018f3a1b-0000-7000-8000-00000000a002";
const channel = {
  id: "018f3a1b-0000-7000-8000-00000000a003",
  provider: "slack",
  channel_name: "Pilot delivery",
  channel_slug: "C_DELIVERY",
  channel_url: "https://fixture.slack.com/archives/C_DELIVERY",
  status: "active",
  verbosity: "important",
  message_detail: "full",
  created_at: "2026-06-01T00:00:00Z",
};
const meta = {
  title: "Records/Deal 360/Delivery handoff",
  component: DealDeliveryHandoff,
  decorators: [
    (Story, context) => {
      installFetchStub({
        "GET /me": meRoute({}),
        [`GET /channel-links/deal/${deal}`]: () =>
          jsonResponse({ state: "none" }),
        [`GET /channel-links/project/${project}`]: () =>
          jsonResponse({
            state: "linked",
            link: {
              ...channel,
              ...(context.parameters.incomingOnly
                ? {
                    send_crm_updates: false,
                    can_send_crm_updates: false,
                    provider: "teams",
                  }
                : {}),
            },
          }),
        [`GET /channel-links/project/${project}/delivery-preview`]: () =>
          jsonResponse({
            link_id: channel.id,
            version: 7,
            provider: "slack",
            workspace: "Account team",
            channel_name: channel.channel_name,
            audience: [{ id: "U_DELIVERY", name: "Delivery owner" }],
            overview_text:
              "Berlin pilot: Legal approval is still required before starting.",
            update_text: "The requested Monday start is not yet confirmed.",
            pending_updates: 1,
            excluded_updates: 1,
            verbosity: "important",
            message_detail: "full",
          }),
        [`GET /projects/${project}/handoff`]: () =>
          jsonResponse({
            project_id: project,
            name: "Berlin pilot",
            phase: "planning",
            as_of: "2026-06-01T00:00:00Z",
            owner_name: "Delivery owner",
            target_end_date: "2026-07-01",
            deals: [],
            stakeholders: [],
            sections_omitted: ["stakeholders"],
            gaps: [],
            open_commitments: [
              {
                task_id: "task-one",
                subject: "Obtain Legal approval before starting",
                state: "open",
                source: "task",
                due_at: "2026-06-28",
                due_has_time: false,
                about: [],
              },
            ],
          }),
        [`GET /deals/${deal}/commitments`]: () =>
          jsonResponse({ data: [], complete: true, has_more: false }),
      });
      return (
        <StoryProviders>
          <Story />
        </StoryProviders>
      );
    },
  ],
} satisfies Meta<typeof DealDeliveryHandoff>;
export default meta;
type Story = StoryObj<typeof meta>;
export const ExistingProjectRoom: Story = {
  args: {
    dealId: deal,
    projectId: project,
    busy: false,
    error: null,
    onClose: () => {},
    onConfirm: () => {},
  },
};
export const ExistingProjectRoomDark: Story = {
  ...ExistingProjectRoom,
  globals: { theme: "dark" },
};

export const IncomingOnlyProjectRoom: Story = {
  ...ExistingProjectRoom,
  parameters: { incomingOnly: true },
};
