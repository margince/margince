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
function withRoutes(routes: RouteMap): Decorator {
  return (Story) => {
    installFetchStub({
      "GET /me": meRoute({ activity: ["create"] }, { seat: "full" }),
      ...routes,
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

// The waiting list could not be read: the section stays, says so, and offers
// the read again rather than vanishing as if nothing were waiting.
function WaitingUnreadable() {
  const proposals = useMeetingProposals(view.contact.id);
  return <WaitingSection contact={view.contact} proposals={proposals} />;
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
