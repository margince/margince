import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../api/schema";
import { ContactReadings } from "./contactreadings";
import { StoryProviders } from "./story-utils";

const view: components["schemas"]["Contact360"] = {
  as_of: "2026-09-13T10:00:00Z",
  contact: {
    id: "contact-1",
    full_name: "Dana Buyer",
    source: "manual",
    captured_by: "human:user-1",
    created_at: "2026-09-01T09:00:00Z",
    updated_at: "2026-09-01T09:00:00Z",
  },
  sections_omitted: [],
  last_inbound_at: "2026-09-10T09:00:00Z",
  last_outbound_at: "2026-09-11T09:00:00Z",
};
const meta: Meta<typeof ContactReadings> = {
  title: "Records/Contact record/Readings",
  component: ContactReadings,
  decorators: [
    (Story) => (
      <StoryProviders>
        <Story />
      </StoryProviders>
    ),
  ],
};
export default meta;
type Story = StoryObj<typeof ContactReadings>;
export const LastMessageFromUs: Story = { args: { view } };
export const LastMessageFromThem: Story = {
  args: { view: { ...view, last_inbound_at: "2026-09-12T09:00:00Z" } },
};
export const Withheld: Story = {
  args: {
    view: {
      ...view,
      sections_omitted: ["last_touch", "claims", "commercial", "next_meeting"],
    },
  },
};
