import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../api/schema";
import { FieldGrid } from "../design-system/fieldgrid";
import { BoughtAttributes } from "./contactboughtattributes";
import { StoryProviders } from "./story-utils";

type Profile = components["schemas"]["ContactProviderProfile"];

const profile: Profile = {
  provider: "surfe",
  state: "completed",
  categories_not_requested: [],
  emails: [],
  mobile_phones: [],
  job_history: [],
  departments: ["Sales", "Finance"],
  seniorities: ["Director"],
  location: "Munich, Germany",
  attributes: [
    {
      kind: "location",
      value: "Munich, Germany",
      retrieved_at: "2026-06-02T12:00:00Z",
    },
    {
      kind: "department",
      value: "Sales",
      retrieved_at: "2026-06-02T12:00:00Z",
    },
    {
      kind: "department",
      value: "Finance",
      retrieved_at: "2026-03-02T12:00:00Z",
    },
    {
      kind: "seniority",
      value: "Director",
      retrieved_at: "2026-03-02T12:00:00Z",
    },
  ],
};

const meta: Meta<typeof BoughtAttributes> = {
  title: "Records/Contact record/Bought attributes",
  component: BoughtAttributes,
  decorators: [
    (Story) => (
      <StoryProviders>
        <div style={{ maxWidth: 420 }}>
          <FieldGrid>
            <Story />
          </FieldGrid>
        </div>
      </StoryProviders>
    ),
  ],
};
export default meta;
type Story = StoryObj<typeof BoughtAttributes>;

export const FromOneProvider: Story = { args: { profiles: [profile] } };

export const FromOneProviderDark: Story = {
  args: { profiles: [profile] },
  globals: { theme: "dark" },
};

// A provider that sold none of the three draws no row at all.
export const NothingBought: Story = {
  args: { profiles: [{ ...profile, attributes: [] }] },
};
