import type { Meta, StoryObj } from "@storybook/react-vite";
import { bookingProfile } from "./book.testkit";
import {
  DefaultFields,
  QuickLink,
  SetupChecklist,
} from "./meeting-settings-sections";
import { StoryProviders } from "./story-utils";

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
export const QuickCopyPaused: Story = {
  render: () => (
    <StoryProviders>
      <QuickLink profile={{ ...bookingProfile, enabled: false }} />
    </StoryProviders>
  ),
};
export const QuickCopyPausedDark: Story = {
  ...QuickCopyPaused,
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
