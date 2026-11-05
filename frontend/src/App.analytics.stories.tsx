import type { Meta, StoryObj } from "@storybook/react-vite";
import { Suspense } from "react";
import { AnalyticsScreen } from "./App.analytics";
import { AnalyticsScreen as AnalyticsRoute } from "./App.screens";
import { reportingStoryRoutes } from "./features/analytics/reporting.story-fixtures";
import { installFetchStub, StoryProviders } from "./testing/story-utils";

const meta = {
  title: "Shell/Reports",
  component: AnalyticsScreen,
} satisfies Meta<typeof AnalyticsScreen>;
export default meta;
type Story = StoryObj;

function installReports() {
  globalThis.location.hash = "#/analytics/reports";
  installFetchStub(reportingStoryRoutes());
}

export const ComposedScreen: Story = {
  render: () => {
    installReports();
    return (
      <StoryProviders>
        <Suspense fallback={null}>
          <AnalyticsScreen />
        </Suspense>
      </StoryProviders>
    );
  },
};

export const LazyRoute: Story = {
  render: () => {
    installReports();
    return (
      <StoryProviders>
        <Suspense fallback={null}>
          <AnalyticsRoute />
        </Suspense>
      </StoryProviders>
    );
  },
};
