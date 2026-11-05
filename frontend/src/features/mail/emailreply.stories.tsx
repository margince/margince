// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../../api/schema";
import { Row } from "../../design-system/stack";
import { StoryProviders } from "../../testing/story-utils";
import { EmailDrawerActions } from "./emailreply";

const message: components["schemas"]["EmailPresentation"] = {
  id: "11111111-1111-4111-8111-111111111111",
  lifecycle: "delivered",
  occurred_at: "2026-09-02T09:12:00Z",
  occurred_precision: "instant",
  summary: {
    activity_id: "11111111-1111-4111-8111-111111111111",
    occurred_at: "2026-09-02T09:12:00Z",
    occurred_precision: "instant",
    display_status: "team",
    attachment_count: 0,
    move: "needs_reply",
    version: 1,
  },
  from: [],
  to: [],
  cc: [],
  bcc: [],
  bcc_withheld: false,
  attachments: [],
  links: [
    {
      entity_type: "contact",
      entity_id: "22222222-2222-4222-8222-222222222222",
    },
  ],
  access: {
    content_state: "available",
    display_status: "team",
    can_change: false,
    change_mode: "none",
  },
  can_reply: true,
  can_relink: true,
  version: 1,
};

const meta: Meta<typeof EmailDrawerActions> = {
  title: "Records/Record 360/Email actions",
  component: EmailDrawerActions,
  decorators: [
    (Story) => (
      <StoryProviders>
        <Row>
          <Story />
        </Row>
      </StoryProviders>
    ),
  ],
  args: {
    presentation: message,
    onReply: () => {},
    onForward: () => {},
    onRelink: () => {},
  },
};
export default meta;

type Story = StoryObj<typeof EmailDrawerActions>;
export const Available: Story = {};
export const CannotReply: Story = {
  args: { presentation: { ...message, can_reply: false } },
};
