// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import { meFixture } from "../testing/mefixture";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../testing/story-utils";
import { MeetingOutcome } from "./meetingoutcome";

// Canceling writes at once; saying how a meeting went opens the composer bound to it.

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
  title: "Records/Meeting outcome",
  component: MeetingOutcome,
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof MeetingOutcome>;

// Opening the composer is the same act in both stories below, so it is one
// function rather than two copies that can drift apart.
const openComposer: Story["play"] = async ({ canvasElement }) => {
  const canvas = within(canvasElement);
  await userEvent.click(
    await canvas.findByRole("button", {
      name: "Update Discovery call with Turbinenbau",
    }),
  );
};

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
        <div>
          <MeetingOutcome
            id={MEETING_ID}
            version={3}
            title="Discovery call with Turbinenbau"
            invalidateKeys={[]}
            cancellable
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
        <div>
          <MeetingOutcome
            id={MEETING_ID}
            version={3}
            title="Discovery call with Turbinenbau"
            invalidateKeys={[]}
            cancellable
          />
        </div>
      </StoryProviders>
    );
  },
  play: openComposer,
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
        <div>
          <MeetingOutcome
            id={MEETING_ID}
            version={3}
            title="Discovery call with Turbinenbau"
            invalidateKeys={[]}
            cancellable
          />
        </div>
      </StoryProviders>
    );
  },
  play: openComposer,
};

/**
 * A meeting with a live calendar invitation: the invitation owns its subject,
 * notes and date, and is canceled through itself. The card offers no cancel,
 * and the composer asks only how it went.
 */
export const CalendarManaged: Story = {
  render: () => {
    stubRoutes({
      ...CAPTURED_MEETING,
      invitation_status: "confirmed",
      calendar_managed: true,
    });
    return (
      <StoryProviders>
        <div>
          <MeetingOutcome
            id={MEETING_ID}
            version={3}
            title="Discovery call with Turbinenbau"
            invalidateKeys={[]}
            cancellable={false}
          />
        </div>
      </StoryProviders>
    );
  },
  play: async (context) => {
    const canvas = within(context.canvasElement);
    await canvas.findByRole("button", {
      name: "Update Discovery call with Turbinenbau",
    });
    await expect(
      canvas.queryByRole("button", { name: /canceled/ }),
    ).not.toBeInTheDocument();
    await openComposer?.(context);
  },
};
