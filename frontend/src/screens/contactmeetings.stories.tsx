import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../api/schema";
import { ContactMeetingsTab } from "./contactmeetings";
import { installFetchStub, meRoute, StoryProviders } from "./story-utils";

const view: components["schemas"]["Contact360"] = {
  as_of: "2026-10-05T09:00:00Z",
  sections_omitted: [],
  contact: {
    id: "contact-1",
    full_name: "Nina Weber",
    source: "manual",
    captured_by: "human:host",
    created_at: "2026-10-01T09:00:00Z",
    updated_at: "2026-10-01T09:00:00Z",
  },
};

const meta: Meta<typeof ContactMeetingsTab> = {
  title: "Records/Contact record/Meetings",
  component: ContactMeetingsTab,
  decorators: [
    (Story) => {
      installFetchStub({
        "GET /me": meRoute({ activity: ["create"] }, { seat: "full" }),
      });
      return (
        <StoryProviders>
          <Story />
        </StoryProviders>
      );
    },
  ],
};
export default meta;
type Story = StoryObj<typeof ContactMeetingsTab>;
export const Empty: Story = {
  args: {
    view: { ...view, activities: { data: [], page: { has_more: false } } },
  },
};
export const EmptyDark: Story = { ...Empty, globals: { theme: "dark" } };
export const Phone: Story = { ...Empty, tags: ["uat-phone"] };
