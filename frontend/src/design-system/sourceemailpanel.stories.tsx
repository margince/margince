import type { Meta, StoryObj } from "@storybook/react-vite";

import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../screens/story-utils";
import { SourceEmailPanel } from "./sourceemailpanel";

const ACTIVITY = "11111111-1111-4111-8111-111111111111";

const SUMMARY = {
  activity_id: ACTIVITY,
  occurred_at: "2026-09-01T09:00:00Z",
  version: 1,
  subject: "Invitation to Vietnam 360 — Vietnam's Next Manufacturing Edge",
  display_status: "team",
  move: "needs_reply",
  attachment_count: 0,
};

function presentation(overrides: Record<string, unknown>) {
  return {
    id: ACTIVITY,
    lifecycle: "delivered",
    occurred_at: "2026-09-01T09:00:00Z",
    version: 1,
    summary: SUMMARY,
    body: "",
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
    ...overrides,
  };
}

const meta = {
  title: "Design System/Source email panel",
  component: SourceEmailPanel,
} satisfies Meta<typeof SourceEmailPanel>;
export default meta;

// Each story mounts the component itself, because each has to install its own
// fetch stub first — so they carry a `render` and no `args`, and are typed as
// bare `StoryObj` rather than `StoryObj<typeof meta>`, which would require the
// args the render replaces. `emailentry.stories.tsx` types its own the same way.

/** The ordinary case: the message a task was read out of, in the task. */
export const Readable: StoryObj = {
  render: () => {
    installFetchStub({
      [`GET /activities/${ACTIVITY}/email-presentation`]: () =>
        jsonResponse(
          presentation({
            body: "Dear Lars,\n\nWe would like to invite you to Vietnam 360.\n\nBest regards\nNhật Minh Nguyễn",
          }),
        ),
    });
    return (
      <StoryProviders>
        <SourceEmailPanel
          activityId={ACTIVITY}
          formatWhen={() => "1 Sept 2026, 09:00"}
        />
      </StoryProviders>
    );
  },
};

/**
 * A message this reader is outside the audience for. The panel shares NOTHING
 * below the refusal — not the subject, not the sender, not the date.
 */
export const Withheld: StoryObj = {
  render: () => {
    installFetchStub({
      [`GET /activities/${ACTIVITY}/email-presentation`]: () =>
        jsonResponse(
          presentation({
            body: null,
            summary: { ...SUMMARY, display_status: "participants" },
            access: {
              content_state: "withheld",
              display_status: "participants",
              audience: "participants",
              can_change: false,
              change_mode: "none",
            },
          }),
        ),
    });
    return (
      <StoryProviders>
        <SourceEmailPanel
          activityId={ACTIVITY}
          formatWhen={() => "1 Sept 2026, 09:00"}
        />
      </StoryProviders>
    );
  },
};

/** The read failed and can be asked again. */
export const Failed: StoryObj = {
  render: () => {
    installFetchStub({
      [`GET /activities/${ACTIVITY}/email-presentation`]: () =>
        new Response("", { status: 500 }),
    });
    return (
      <StoryProviders>
        <SourceEmailPanel
          activityId={ACTIVITY}
          formatWhen={() => "1 Sept 2026, 09:00"}
        />
      </StoryProviders>
    );
  },
};
