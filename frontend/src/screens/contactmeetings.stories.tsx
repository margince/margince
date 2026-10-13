import type { Decorator, Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../api/schema";
import { bookingProfile, bookingProposals } from "./book.testkit";
import { ContactMeetingsTab } from "./contactmeetings";
import { useMeetingProposals, WaitingSection } from "./contactmeetings.waiting";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  type RouteMap,
  StoryProviders,
} from "./story-utils";

const view: components["schemas"]["Contact360"] = {
  as_of: "2026-10-05T09:00:00Z",
  sections_omitted: [],
  contact: {
    id: "contact-1",
    full_name: "Nina Weber",
    first_name: "Nina",
    primary_email: "nina@brandt.example",
    source: "manual",
    captured_by: "human:host",
    created_at: "2026-10-01T09:00:00Z",
    updated_at: "2026-10-01T09:00:00Z",
  },
};

// The tab's reads, with the proposals and the host's profile the story names.
// A factory where a route answers differently on its second call, so every
// render of the story starts again from the first page.
function withRoutes(routes: RouteMap | (() => RouteMap)): Decorator {
  return (Story) => {
    installFetchStub({
      "GET /me": meRoute({ activity: ["create"] }, { seat: "full" }),
      ...(typeof routes === "function" ? routes() : routes),
    });
    return (
      <StoryProviders>
        <Story />
      </StoryProviders>
    );
  };
}

const meta: Meta<typeof ContactMeetingsTab> = {
  title: "Records/Contact 360/Meetings",
  component: ContactMeetingsTab,
};
export default meta;
type Story = StoryObj<typeof ContactMeetingsTab>;
export const Empty: Story = {
  decorators: [withRoutes({})],
  args: {
    view: { ...view, activities: { data: [], page: { has_more: false } } },
  },
};
export const EmptyDark: Story = { ...Empty, globals: { theme: "dark" } };
export const Phone: Story = { ...Empty, tags: ["uat-phone"] };

// Two invitations still unanswered, one offering times and one a personal
// link, with the host's own booking link offered beside "Book a meeting".
export const WaitingOnReply: Story = {
  ...Empty,
  decorators: [
    withRoutes({
      "GET /scheduling/proposals": () =>
        jsonResponse({ data: bookingProposals }),
      "GET /scheduling/profile": () => jsonResponse(bookingProfile),
    }),
  ],
};
export const WaitingOnReplyDark: Story = {
  ...WaitingOnReply,
  globals: { theme: "dark" },
};
export const WaitingOnReplyPhone: Story = {
  ...WaitingOnReply,
  tags: ["uat-phone"],
};

// A contact with a week in motion: the next meeting booked through Margince
// with its Meet link and a prep moment, a second one after it, two held and
// one the contact missed, and two invitations still unanswered.
type Activity = components["schemas"]["Activity"];
function meeting(
  id: string,
  occurredAt: string,
  subject: string,
  status: NonNullable<Activity["meeting_status"]>,
  more: Partial<Activity> = {},
): Activity {
  return {
    id,
    kind: "meeting",
    subject,
    occurred_at: occurredAt,
    duration_seconds: 1800,
    meeting_status: status,
    is_done: false,
    source: "manual",
    captured_by: "human:host",
    content_state: "available",
    created_at: "2026-09-01T09:00:00Z",
    updated_at: "2026-09-01T09:00:00Z",
    ...more,
  };
}
const nextMeetingId = "0198f011-bbbb-7000-8000-000000000001";
const meetings: Activity[] = [
  meeting(
    "0198f011-bbbb-7000-8000-000000000002",
    "2026-10-09T09:00:00Z",
    "Weekly call: rollout planning",
    "booked",
    { invitation_status: "confirmed" },
  ),
  meeting(
    nextMeetingId,
    "2026-10-02T09:00:00Z",
    "Weekly call: rollout planning",
    "booked",
    { invitation_status: "confirmed" },
  ),
  meeting(
    "0198f011-bbbb-7000-8000-000000000003",
    "2026-09-24T08:00:00Z",
    "Weekly call: rollout planning",
    "held",
    { duration_seconds: 3600, invitation_status: "confirmed" },
  ),
  meeting(
    "0198f011-bbbb-7000-8000-000000000004",
    "2026-09-17T07:00:00Z",
    "Discovery call: scheduling tool",
    "held",
  ),
  meeting(
    "0198f011-bbbb-7000-8000-000000000005",
    "2026-09-10T08:00:00Z",
    "Intro call",
    "no_show",
  ),
];
const bookedView: components["schemas"]["Contact360"] = {
  ...view,
  as_of: "2026-10-01T09:00:00Z",
  next_meeting: {
    activity_id: nextMeetingId,
    starts_at: "2026-10-02T09:00:00Z",
    subject: "Weekly call: rollout planning",
    participants: [
      { contact_id: "contact-2", full_name: "Sabine Mayer" },
      { contact_id: "contact-1", full_name: "Nina Weber" },
    ],
  },
  moment: {
    claim_key: `meeting_prep:contact-1:${nextMeetingId}`,
    evidence_fingerprint: "fp-meeting-1",
    rule: "meeting_prep",
    headline: "Nina's weekly call is tomorrow.",
    why_now: "Prepare before the meeting, not after.",
    confidence: "observed_fact",
    evidence: [
      {
        type: "activity",
        id: nextMeetingId,
        label: "Weekly call: rollout planning, 2 Oct",
        observed_at: "2026-10-02T09:00:00Z",
      },
    ],
    recommended_action: {
      kind: "open_meeting_brief",
      label: "Open meeting brief",
      destination: { surface: "meeting_brief" },
      state: "available",
    },
    secondary_actions: [
      { kind: "draft_reply", label: "Draft agenda", state: "available" },
    ],
  },
};
const nextInvitation: components["schemas"]["MeetingInvitation"] = {
  id: nextMeetingId,
  status: "confirmed",
  start: "2026-10-02T09:00:00Z",
  end: "2026-10-02T09:30:00Z",
  subject: "Weekly call: rollout planning",
  location: "",
  version: 2,
  provider: "gcal",
  video_call: true,
  video_url: "https://meet.google.com/abc-defg-hij",
};
// What Load more reads after the first page.
const olderMeetings: Activity[] = [
  meeting(
    "0198f011-bbbb-7000-8000-000000000006",
    "2026-08-27T08:00:00Z",
    "Kick-off",
    "held",
  ),
];
export const Booked: Story = {
  decorators: [
    withRoutes(() => {
      let read = 0;
      return {
        "GET /scheduling/proposals": () =>
          jsonResponse({ data: bookingProposals }),
        "GET /scheduling/profile": () => jsonResponse(bookingProfile),
        "GET /activities": () =>
          read++ === 0
            ? jsonResponse({
                data: meetings,
                page: { has_more: true, next_cursor: "c2" },
              })
            : jsonResponse({
                data: olderMeetings,
                page: { has_more: false },
              }),
        [`GET /scheduling/invitations/${nextMeetingId}`]: () =>
          jsonResponse(nextInvitation),
      };
    }),
  ],
  args: {
    view: bookedView,
    onBriefMeeting: () => undefined,
    onAction: () => undefined,
  },
};
export const BookedDark: Story = { ...Booked, globals: { theme: "dark" } };
export const BookedPhone: Story = { ...Booked, tags: ["uat-phone"] };

// The waiting list could not be read: the section stays, says so, and offers
// the read again rather than vanishing as if nothing were waiting.
function WaitingUnreadable() {
  const proposals = useMeetingProposals(view.contact.id);
  return (
    <WaitingSection
      contact={view.contact}
      proposals={proposals}
      copied={{ url: null, onCopied: () => undefined }}
      afterWithdraw={() => null}
    />
  );
}
export const WaitingFailed: StoryObj = {
  decorators: [
    withRoutes({
      "GET /scheduling/proposals": () =>
        jsonResponse({ title: "Unavailable", status: 503 }, 503),
    }),
  ],
  render: () => <WaitingUnreadable />,
};
