// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import type { components } from "../../api/schema";
import { Panel } from "../../design-system/panel";
import {
  installFetchStub,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { MeetingRow } from "./contactmeetings.rows";

type Activity = components["schemas"]["Activity"];

// One meeting on the contact's Meetings tab: its date tile, title and the
// span it ran. A meeting logged with its date alone has no span to show.
const meeting: Activity = {
  id: "0198f011-cccc-7000-8000-000000000001",
  kind: "meeting",
  subject: "Depot walkthrough",
  occurred_at: "2026-09-24T08:00:00Z",
  occurred_precision: "instant",
  offers_email: false,
  duration_seconds: 1800,
  meeting_status: "held",
  is_done: false,
  source: "manual",
  captured_by: "human:host",
  content_state: "available",
  created_at: "2026-09-01T09:00:00Z",
  updated_at: "2026-09-01T09:00:00Z",
};

const meta: Meta<typeof MeetingRow> = {
  title: "Records/Contact 360/Meeting row",
  component: MeetingRow,
  decorators: [
    (Story) => {
      installFetchStub({ "GET /me": meRoute({ activity: ["read"] }) });
      return (
        <StoryProviders>
          <Panel title="Meetings">
            <Story />
          </Panel>
        </StoryProviders>
      );
    },
  ],
};
export default meta;
type Story = StoryObj<typeof MeetingRow>;

export const WithItsSpan: Story = { args: { activity: meeting } };

export const DateOnly: Story = {
  args: { activity: { ...meeting, occurred_precision: "day" } },
  play: async ({ canvasElement }) => {
    const title = await within(canvasElement).findByText("Depot walkthrough");
    const row = title.closest("article");
    await expect(row).not.toBeNull();
    await expect(row?.textContent).not.toMatch(/\d{1,2}:\d{2}/);
  },
};

export const DateOnlyDark: Story = {
  ...DateOnly,
  globals: { theme: "dark" },
};
