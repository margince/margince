import type { Meta, StoryObj } from "@storybook/react-vite";
import { EmploymentEdit } from "./employmentedit";
import { StoryProviders } from "./story-utils";

const meta: Meta<typeof EmploymentEdit> = {
  title: "Records/Contact record/Employment correction",
  component: EmploymentEdit,
  decorators: [
    (Story) => (
      <StoryProviders>
        <Story />
      </StoryProviders>
    ),
  ],
  args: {
    contactId: "sample",
    employment: {
      relationship_id: "role",
      company_id: "company",
      is_current_primary: false,
      role: "Advisor",
      employment_status: "unknown",
      started_at: "2020-02-01",
      started_precision: "month",
      version: 1,
    },
    onClose: () => {},
    onSaved: async () => {},
  },
};
export default meta;
type Story = StoryObj<typeof meta>;
export const UnknownDates: Story = {};
export const Former: Story = {
  args: {
    employment: {
      relationship_id: "role",
      company_id: "company",
      is_current_primary: false,
      role: "Engineer",
      employment_status: "former",
      started_at: "2020-02-01",
      started_precision: "month",
      ended_at: "2024-05-17",
      ended_precision: "day",
      version: 1,
    },
  },
};
