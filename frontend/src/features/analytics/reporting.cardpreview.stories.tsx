import type { Meta, StoryObj } from "@storybook/react-vite";
import { installFetchStub, StoryProviders } from "../../testing/story-utils";
import { CardPreview } from "./reporting.cardpreview";
import {
  reportingStoryEvaluation,
  reportingStoryRoutes,
} from "./reporting.story-fixtures";

const meta: Meta = {
  title: "Records/Reports/Analytics/Card preview",
  beforeEach: () => installFetchStub(reportingStoryRoutes()),
};
export default meta;
type Story = StoryObj;
export const Default: Story = {
  render: () => (
    <StoryProviders>
      <CardPreview
        selection={reportingStoryEvaluation.selection}
        choice={{ kind: "bookings_trend", metric: "bookings_won" }}
      />
    </StoryProviders>
  ),
};
export const Dark: Story = { ...Default, globals: { theme: "dark" } };
