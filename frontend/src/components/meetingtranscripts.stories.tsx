// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import type { components } from "../api/schema";
import { en } from "../i18n/en";
import { meFixture } from "../testing/mefixture";
import {
  emptyPage,
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../testing/story-utils";
import { RecordMeetingOutcome } from "./meetingoutcome";

const meeting: components["schemas"]["Activity"] = {
  id: "01a05500-0000-7000-8000-0000000000b1",
  kind: "meeting",
  subject: "Renewal review",
  body: "Calendar agenda: renewal options",
  meeting_notes: "Buyer approved the renewal. Send the final proposal.",
  meeting_status: "held",
  calendar_managed: true,
  source_system: "gcal",
  source: "gcal",
  captured_by: "connector:gcal",
  writable: true,
  occurred_at: "2026-11-26T08:00:00Z",
  created_at: "2026-11-26T08:00:00Z",
  updated_at: "2026-11-26T08:00:00Z",
};
function routes(writable: boolean) {
  installFetchStub({
    "GET /me": () =>
      jsonResponse(
        meFixture({ allow: { activity: ["read", "create", "update"] } }),
      ),
    [`GET /activities/${meeting.id}`]: () =>
      jsonResponse({ ...meeting, writable }),
    "GET /activities": () => jsonResponse(emptyPage),
  });
}
const meta: Meta<typeof RecordMeetingOutcome> = {
  title: "Records/Meeting notes and transcripts",
  component: RecordMeetingOutcome,
};
export default meta;
type Story = StoryObj<typeof meta>;
export const HeldCalendarMeeting: Story = {
  render: () => {
    routes(true);
    return (
      <StoryProviders>
        <RecordMeetingOutcome activity={meeting} invalidateKeys={[]} />
      </StoryProviders>
    );
  },
  play: async ({ canvasElement }) => {
    await userEvent.click(
      within(canvasElement).getByRole("button", {
        name: en["meeting.transcripts"],
      }),
    );
    const dialog = within(document.body).getByRole("dialog");
    await expect(
      within(dialog).findByText(meeting.meeting_notes ?? ""),
    ).resolves.toBeVisible();
    await expect(
      within(dialog).getByRole("textbox", {
        name: en["meeting.transcriptText"],
      }),
    ).toBeVisible();
  },
};
export const ReadOnlyMeeting: Story = {
  render: () => {
    routes(false);
    return (
      <StoryProviders>
        <RecordMeetingOutcome
          activity={{ ...meeting, writable: false }}
          invalidateKeys={[]}
        />
      </StoryProviders>
    );
  },
  play: async ({ canvasElement }) => {
    await userEvent.click(
      within(canvasElement).getByRole("button", {
        name: en["meeting.transcripts"],
      }),
    );
    await expect(
      within(document.body).findByText(en["meeting.readOnly"]),
    ).resolves.toBeVisible();
  },
};
