import type { Meta, StoryObj } from "@storybook/react-vite";
import { within } from "storybook/test";
import { installFetchStub, StoryProviders } from "../../testing/story-utils";
import { InitiativeDrawer, ReportingEvidenceDrawer } from "./analytics.drawers";
import {
  reportingStoryEvaluation,
  reportingStoryRoutes,
} from "./reporting.story-fixtures";

const meta: Meta = {
  title: "Records/Reports/Analytics/Drawer loaders",
  decorators: [
    (Story) => (
      <StoryProviders>
        <Story />
      </StoryProviders>
    ),
  ],
  beforeEach: () => installFetchStub(reportingStoryRoutes()),
};
export default meta;
type Story = StoryObj;
const ready = async () => {
  await within(document.body).findByRole("dialog");
};
export const Evidence: Story = {
  render: () => (
    <ReportingEvidenceDrawer
      evaluation={reportingStoryEvaluation}
      reference={{ metric: "bookings_won", context_id: "interval" }}
      onClose={() => undefined}
    />
  ),
  play: ready,
};
export const EvidenceDark: Story = { ...Evidence, globals: { theme: "dark" } };
export const Initiative: Story = {
  render: () => <InitiativeDrawer onClose={() => undefined} />,
  play: ready,
};
export const InitiativeDark: Story = {
  ...Initiative,
  globals: { theme: "dark" },
};
