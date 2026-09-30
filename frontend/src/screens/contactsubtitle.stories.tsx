import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../api/schema";
import { ContactSubtitle } from "./contactsubtitle";
import { StoryProviders } from "./story-utils";
import "./contact360.css";

type Contact360 = components["schemas"]["Contact360"];

const view: Contact360 = {
  as_of: "2026-08-13T09:00:00Z",
  contact: {
    id: "p-1",
    full_name: "Dana Buyer",
    title: "Head of Revenue",
    source: "manual",
    captured_by: "human:u-1",
    created_at: "2026-06-01T08:00:00Z",
    updated_at: "2026-08-01T08:00:00Z",
  },
  sections_omitted: [],
  employments: {
    data: [
      {
        relationship_id: "rel-1",
        company_id: "o-1",
        company_name: "Brandt Automotive GmbH",
        employment_status: "current",
        is_current_primary: true,
      },
    ],
    page: { has_more: false },
  },
};

const boughtView: Contact360 = {
  ...view,
  contact: {
    ...view.contact,
    bought_fields: [
      {
        target: "title",
        provider: "surfe",
        applied_at: "2026-06-02T12:00:00Z",
      },
      {
        target: "employment:rel-1",
        provider: "surfe",
        applied_at: "2026-06-02T12:00:00Z",
      },
    ],
  },
};

const meta: Meta<typeof ContactSubtitle> = {
  title: "Records/Contact 360/Header subtitle",
  component: ContactSubtitle,
  decorators: [
    (Story) => (
      <StoryProviders>
        <Story />
      </StoryProviders>
    ),
  ],
};
export default meta;
type Story = StoryObj<typeof ContactSubtitle>;

export const Typed: Story = { args: { view } };

// A title and an employer a purchase filled: the title underlined, the
// employer's mark beside the button that opens the company.
export const Bought: Story = { args: { view: boughtView } };

export const BoughtDark: Story = {
  args: { view: boughtView },
  globals: { theme: "dark" },
};
