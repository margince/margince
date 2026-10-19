// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { viewerZone } from "../format/timezone";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../testing/story-utils";
import { OpenActivityDrawer } from "./activitydrawer";

const id = "11111111-1111-4111-8111-111111111111";
const meta: Meta<typeof OpenActivityDrawer> = {
  title: "Records/Record 360/Activity source review",
  component: OpenActivityDrawer,
  args: { activityId: id, zone: viewerZone(), onClose: () => {} },
  decorators: [
    (Story) => (
      <StoryProviders>
        <Story />
      </StoryProviders>
    ),
  ],
};
export default meta;
type Story = StoryObj<typeof OpenActivityDrawer>;

export const HistoricalMeeting: Story = {
  render: (args) => {
    installFetchStub({
      "GET /me": meRoute({ activity: ["read"] }),
      [`GET /activities/${id}`]: () =>
        jsonResponse({
          id,
          kind: "meeting",
          subject: "Pricing discussion",
          body: "Pat: I will send revised pricing.",
          occurred_at: "2024-10-18T10:00:00Z",
          content_state: "available",
          links: [],
          commitment_admissions: [
            {
              locator: "pricing",
              summary: "Send revised pricing",
              source_quote: "I will send revised pricing.",
              outcome: "historical_without_current_deadline",
              current_work: false,
              rule_version: "1",
              evidence_ids: [id],
              evaluated_at: "2026-10-18T10:00:00Z",
            },
          ],
        }),
    });
    return <OpenActivityDrawer {...args} />;
  },
};

export const WithheldSource: Story = {
  render: (args) => {
    installFetchStub({
      "GET /me": meRoute({ activity: ["read"] }),
      [`GET /activities/${id}`]: () =>
        jsonResponse({
          id,
          kind: "meeting",
          content_state: "withheld",
          occurred_at: "2024-10-18T10:00:00Z",
          links: [],
        }),
    });
    return <OpenActivityDrawer {...args} />;
  },
};

export const MissingSource: Story = {
  render: (args) => {
    installFetchStub({
      "GET /me": meRoute({ activity: ["read"] }),
      [`GET /activities/${id}`]: () =>
        jsonResponse(
          {
            type: "urn:margince:problem:not_found",
            status: 404,
            code: "not_found",
            title: "Not found",
          },
          404,
        ),
    });
    return <OpenActivityDrawer {...args} />;
  },
};
export const MissingSourceDark: Story = {
  ...MissingSource,
  globals: { theme: "dark" },
};
export const WithheldSourceDark: Story = {
  ...WithheldSource,
  globals: { theme: "dark" },
};
