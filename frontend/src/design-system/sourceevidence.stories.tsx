// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";

import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../testing/story-utils";
import { SourceEvidence } from "./sourceevidence";

const ACTIVITY = "22222222-2222-4222-8222-222222222222";

function activity(kind: "email" | "meeting") {
  return {
    id: ACTIVITY,
    kind,
    subject: kind === "email" ? "Fleet renewal" : "Quarterly review",
    body: "",
    occurred_at: "2026-09-01T09:00:00Z",
    occurred_precision: "instant",
    is_done: true,
    source: "capture",
    captured_by: "connector:gmail",
    created_at: "2026-09-01T09:00:00Z",
    updated_at: "2026-09-01T09:00:00Z",
    links: [],
  };
}

const PRESENTATION = {
  id: ACTIVITY,
  lifecycle: "delivered",
  occurred_at: "2026-09-01T09:00:00Z",
  version: 1,
  summary: {
    activity_id: ACTIVITY,
    occurred_at: "2026-09-01T09:00:00Z",
    version: 1,
    subject: "Fleet renewal",
    display_status: "team",
    move: "needs_reply",
    attachment_count: 0,
  },
  body: "Can you hold the price until Friday?\n\nBest regards\nDana Buyer",
  from: [],
  to: [],
  cc: [],
  bcc: [],
  bcc_withheld: false,
  attachments: [],
  links: [],
  thread: { members: [], next_cursor: null },
  can_reply: false,
  can_relink: false,
  access: {
    content_state: "available",
    display_status: "team",
    audience: "workspace",
    can_change: false,
    change_mode: "none",
  },
};

const meta = {
  title: "Components/AI and provenance/Source evidence",
  component: SourceEvidence,
} satisfies Meta<typeof SourceEvidence>;
export default meta;

// Each story installs its own fetch stub before it mounts, so each carries a
// `render` and no `args`.

/** An email is drawn in place, inside the task it was read out of. */
export const Email: StoryObj = {
  render: () => {
    installFetchStub({
      [`GET /activities/${ACTIVITY}`]: () => jsonResponse(activity("email")),
      [`GET /activities/${ACTIVITY}/email-presentation`]: () =>
        jsonResponse(PRESENTATION),
    });
    return (
      <StoryProviders>
        <SourceEvidence activityId={ACTIVITY} openerFor={() => () => {}} />
      </StoryProviders>
    );
  },
};

/** A transcript is a record of its own, so it keeps a button. */
export const Transcript: StoryObj = {
  render: () => {
    installFetchStub({
      [`GET /activities/${ACTIVITY}`]: () => jsonResponse(activity("meeting")),
    });
    return (
      <StoryProviders>
        <SourceEvidence activityId={ACTIVITY} openerFor={() => () => {}} />
      </StoryProviders>
    );
  },
};

/** A meeting logged with its date alone: the caption names the date, no time. */
export const DateOnlyTranscript: StoryObj = {
  render: () => {
    installFetchStub({
      [`GET /activities/${ACTIVITY}`]: () =>
        jsonResponse({ ...activity("meeting"), occurred_precision: "day" }),
    });
    return (
      <StoryProviders>
        <SourceEvidence activityId={ACTIVITY} openerFor={() => () => {}} />
      </StoryProviders>
    );
  },
  play: async ({ canvasElement }) => {
    const caption = await within(canvasElement).findByText(/Quarterly review/);
    await expect(caption.textContent).toMatch(/\d{2}\/\d{2}\/\d{4}/);
    await expect(caption.textContent).not.toMatch(/\d{1,2}:\d{2}/);
  },
};

/** The lookup failed, and says so with a way to ask again. */
export const Failed: StoryObj = {
  render: () => {
    installFetchStub({
      [`GET /activities/${ACTIVITY}`]: () => new Response("", { status: 500 }),
    });
    return (
      <StoryProviders>
        <SourceEvidence activityId={ACTIVITY} openerFor={() => () => {}} />
      </StoryProviders>
    );
  },
};

export const Withheld: StoryObj = {
  render: () => {
    installFetchStub({
      [`GET /activities/${ACTIVITY}`]: () =>
        jsonResponse({
          ...activity("meeting"),
          content_state: "withheld",
        }),
    });
    return (
      <StoryProviders>
        <SourceEvidence activityId={ACTIVITY} openerFor={() => () => {}} />
      </StoryProviders>
    );
  },
};
