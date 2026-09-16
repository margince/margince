// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { meFixture } from "../app/mefixture";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";
import { MeetingOutcome } from "./worklist.meetingoutcome";
// The line the verbs stand on belongs to the ROW rather than to the screen, so
// a story mounting them without it draws a bare inline flow with no gap.
import "./worklist.css";
import "./worklist.row.css";

// What a rep does with a meeting that already happened.
//
// The card used to offer three statuses — held, no-show, cancelled — and each
// wrote one word and closed. The two verbs here are not two of those three:
// they differ in KIND. Cancelling is a fact with nothing to add, so the card
// writes it; saying what came of a meeting is the thing a rep opened the queue
// to record, so it opens the composer bound to that meeting.

const MEETING_ID = "01a05500-0000-7000-8000-0000000000b1";

// The meeting as its own endpoint answers it. The body is the calendar's own
// excerpt — the shape capture/meetingmap composes — because amending that text
// without losing the attendee record is what the dialog is for.
const CAPTURED_MEETING = {
  id: MEETING_ID,
  kind: "meeting",
  subject: "Discovery call with Turbinenbau",
  body: "Organizer: greta@turbinenbau.example\nAttendees: lars@gradion.com, greta@turbinenbau.example\n\nFirst look at the retrofit line.",
  occurred_at: "2026-08-31T08:00:00Z",
  meeting_status: "booked",
  version: 3,
  source: "gcal",
  captured_by: "connector:gcal",
  created_at: "2026-08-31T08:00:00Z",
  updated_at: "2026-08-31T08:00:00Z",
};

// Every story routes the session probe: unrouted, `useMe` reads the list-shaped
// fallback as a malformed session, every grant fails closed, and the card draws
// a branch the story is not named for — with the render gate none the wiser.
function stubRoutes(meeting: unknown = CAPTURED_MEETING): void {
  installFetchStub({
    "GET /me": () =>
      jsonResponse(meFixture({ allow: { activity: ["read", "update"] } })),
    [`GET /activities/${MEETING_ID}`]: () => jsonResponse(meeting),
    [`PATCH /activities/${MEETING_ID}`]: () =>
      jsonResponse({ ...CAPTURED_MEETING, version: 4 }),
  });
}

const meta: Meta<typeof MeetingOutcome> = {
  title: "Records/Worklist/Meeting outcome",
  component: MeetingOutcome,
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof MeetingOutcome>;

/**
 * THE CARD's two verbs, closed.
 *
 * Neither is filled: a meeting the product knows nothing about has no expected
 * answer, and leading the line with either would claim one.
 */
export const TwoVerbs: Story = {
  render: () => {
    stubRoutes();
    return (
      <StoryProviders>
        <div className="worklist-row-acts">
          <MeetingOutcome
            id={MEETING_ID}
            version={3}
            title="Discovery call with Turbinenbau"
          />
        </div>
      </StoryProviders>
    );
  },
};

/**
 * THE COMPOSER, open on the meeting, seeded from what was captured.
 *
 * The subject and the calendar's own notes are already in the boxes: the reader
 * is amending what arrived, not writing it again from nothing. A form that
 * opened blank would invite them to save an empty body over the attendee list.
 */
export const ComposerOpen: Story = {
  render: () => {
    stubRoutes();
    return (
      <StoryProviders>
        <div className="worklist-row-acts">
          <MeetingOutcome
            id={MEETING_ID}
            version={3}
            title="Discovery call with Turbinenbau"
          />
        </div>
      </StoryProviders>
    );
  },
  play: async ({ canvasElement }) => {
    const { within, userEvent } = await import("storybook/test");
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", { name: "Update" }),
    );
  },
};

/**
 * A MEETING THAT CARRIES NO NOTES, which is the ordinary case for one booked
 * from a bare calendar invitation: the body box stands empty and the reader
 * types the outcome into it.
 */
export const NothingCapturedYet: Story = {
  render: () => {
    stubRoutes({ ...CAPTURED_MEETING, body: null });
    return (
      <StoryProviders>
        <div className="worklist-row-acts">
          <MeetingOutcome
            id={MEETING_ID}
            version={3}
            title="Discovery call with Turbinenbau"
          />
        </div>
      </StoryProviders>
    );
  },
  play: async ({ canvasElement }) => {
    const { within, userEvent } = await import("storybook/test");
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", { name: "Update" }),
    );
  },
};
