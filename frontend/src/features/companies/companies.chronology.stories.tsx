// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import { GroupedTimelineList } from "../../design-system/timeline";
import { useViewerZone } from "../../lib/viewerzone";
import { company, company360 } from "../../testing/company.fixtures";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { useChronologySlots } from "./companies.chronology";

function History() {
  const zone = useViewerZone();
  const { slots } = useChronologySlots({
    company,
    view: company360,
    loading: false,
    failed: false,
    active: true,
  });
  return (
    <div style={{ maxWidth: 720 }}>
      <div className="timeline-header">{slots.timelineHeader}</div>
      {slots.timelineNotice ?? (
        <GroupedTimelineList groups={slots.timelineGroups ?? []} zone={zone} />
      )}
    </div>
  );
}

const mail = (id: string, direction: "inbound" | "outbound", day: string) => ({
  id,
  kind: "email",
  subject: "Re: Fleet renewal",
  body: "The revised offer is attached. Delivery starts in the third quarter.",
  direction,
  thread_key: "thread-1",
  occurred_at: `2026-06-${day}T09:00:00Z`,
  occurred_precision: "instant",
  content_state: "available",
  links: [{ entity_type: "company", entity_id: company.id }],
  source: "manual",
  captured_by: "human:u-1",
  created_at: "2026-06-01T08:00:00Z",
  updated_at: "2026-06-01T08:00:00Z",
  offers_email: false,
  is_done: false,
});

function render() {
  installFetchStub({
    "GET /me": meRoute({ activity: ["read"] }),
    "GET /activities": () =>
      jsonResponse({
        data: [mail("m-2", "outbound", "15"), mail("m-1", "inbound", "14")],
        page: { has_more: false, next_cursor: null },
      }),
  });
  return (
    <StoryProviders>
      <History />
    </StoryProviders>
  );
}

export default {
  title: "Records/Company 360/History threads",
  component: History,
} satisfies Meta<typeof History>;

type Story = StoryObj;

/** The Threads cut asks the server for mail and messages, so a thread older
 *  than the newest page still shows. */
export const ThreadsCut: Story = {
  render,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", { name: "Threads" }),
    );
    await expect(
      await canvas.findByText("Re: Fleet renewal"),
    ).toBeInTheDocument();
    await expect(canvas.queryByText("No threads yet.")).toBeNull();
  },
};

/** On a phone the date folds above the thread and each message's time drops
 *  under its sender, so the words keep the row's width. */
export const ThreadsCutOnPhone: Story = {
  ...ThreadsCut,
  tags: ["uat-phone"],
};
