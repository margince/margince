import type { Meta, StoryObj } from "@storybook/react-vite";
import { REPORTING_FIXTURE_ZONE } from "./reporting.fixtures";
import {
  reportingStoryRoutes,
  reportingStoryScope,
} from "./reporting.story-fixtures";
import { ReportingTargetDialog } from "./reporting.targetdialog";
import { installFetchStub, StoryProviders } from "./story-utils";

const meta: Meta = { title: "Records/Reports/Analytics/Target editor" };
export default meta;
type Story = StoryObj;
export const Create: Story = {
  render: () => {
    installFetchStub(reportingStoryRoutes());
    return (
      <StoryProviders>
        <ReportingTargetDialog
          context={{
            as_of: "2026-09-22T12:00:00Z",
            timezone: REPORTING_FIXTURE_ZONE,
            base_currency: "EUR",
            default_scope: reportingStoryScope,
            allowed_scopes: [reportingStoryScope],
            capabilities: {
              view_manager_forecast: true,
              submit_manager_forecast: true,
            },
          }}
          onClose={() => {}}
        />
      </StoryProviders>
    );
  },
};
export const Dark: Story = { ...Create, globals: { theme: "dark" } };
