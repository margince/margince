import type { Meta, StoryObj } from "@storybook/react-vite";
import { StoryProviders } from "../testing/story-utils";
import { bookingProfile } from "./book.testkit";
import { DefaultFields, SetupChecklist } from "./meeting-settings-sections";

import "./book.css";

const meta: Meta<typeof SetupChecklist> = {
  title: "Settings/You/Meetings/Meeting settings sections",
  component: SetupChecklist,
};
export default meta;
type Story = StoryObj<typeof SetupChecklist>;

export const ChecklistOneLeft: Story = {
  render: () => (
    <StoryProviders>
      <SetupChecklist calendar hours link={false} />
    </StoryProviders>
  ),
};
export const ChecklistOneLeftDark: Story = {
  ...ChecklistOneLeft,
  globals: { theme: "dark" },
};
export const DefaultsOnOutlook: Story = {
  render: () => (
    <StoryProviders>
      <DefaultFields
        form={{ ...bookingProfile, provider: "graphcal" }}
        provider="graphcal"
        onChange={() => undefined}
      />
    </StoryProviders>
  ),
};
export const DefaultsOnOutlookDark: Story = {
  ...DefaultsOnOutlook,
  globals: { theme: "dark" },
};
