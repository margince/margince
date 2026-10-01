import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { ReportingEvidenceDrawer } from "./reporting.evidence";
import {
  reportingStoryEvaluation,
  reportingStoryRoutes,
} from "./reporting.story-fixtures";
import { installFetchStub, StoryProviders } from "./story-utils";

const meta: Meta = { title: "Records/Reports/Analytics/Evidence" };
export default meta;
type Story = StoryObj;
function Preview({
  earlierDefinition = false,
}: Readonly<{ earlierDefinition?: boolean }>) {
  const [open, setOpen] = useState(true);
  return open ? (
    <ReportingEvidenceDrawer
      evaluation={
        earlierDefinition
          ? {
              ...reportingStoryEvaluation,
              metrics: reportingStoryEvaluation.metrics.map((metric) => ({
                ...metric,
                version: "1",
              })),
            }
          : reportingStoryEvaluation
      }
      editionId={earlierDefinition ? "old" : undefined}
      reference={{ metric: "bookings_won", context_id: "interval" }}
      onClose={() => setOpen(false)}
    />
  ) : (
    <p>Dialog closed</p>
  );
}
export const Default: Story = {
  render: () => {
    installFetchStub(reportingStoryRoutes());
    return (
      <StoryProviders>
        <Preview />
      </StoryProviders>
    );
  },
};

export const EarlierDefinition: Story = {
  render: () => {
    const routes = reportingStoryRoutes();
    installFetchStub({
      ...routes,
      "GET /analytics/editions/old/evidence": routes["GET /analytics/evidence"],
    });
    return (
      <StoryProviders>
        <Preview earlierDefinition />
      </StoryProviders>
    );
  },
};
