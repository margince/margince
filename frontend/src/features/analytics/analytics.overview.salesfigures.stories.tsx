import type { Meta, StoryObj } from "@storybook/react-vite";
import { within } from "storybook/test";
import { StoryProviders } from "../../testing/story-utils";
import { AnalyticsScreen } from "./analytics";
import {
  isSetup,
  OVERVIEW_SCENARIOS,
  overviewAddress,
} from "./analytics.overview.scenarios.fixtures";
import { installOverviewServer } from "./analytics.overview.server.fixtures";

const meta: Meta = {
  title: "Records/Reports/Analytics/Sales figures",
  parameters: { layout: "padded" },
  beforeEach: () => {
    const previous = location.hash;
    location.hash = overviewAddress("sales");
    const pair = OVERVIEW_SCENARIOS.populated.views.sales;
    if (!isSetup(pair)) throw new Error("Populated sales fixture is missing");
    installOverviewServer(pair);
    return () => {
      location.hash = previous;
    };
  },
};
export default meta;
type Story = StoryObj;

export const Loaded: Story = {
  render: () => (
    <StoryProviders>
      <AnalyticsScreen />
    </StoryProviders>
  ),
  play: async ({ canvasElement }) => {
    await within(canvasElement).findByRole("region", {
      name: "Opening readings",
    });
  },
};
export const LoadedDark: Story = { ...Loaded, globals: { theme: "dark" } };
