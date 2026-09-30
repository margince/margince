import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { ReportingComparison } from "./reporting.comparison";
import { reportingEditions } from "./reporting.scenarios";
import { reportingStoryRoutes } from "./reporting.story-fixtures";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

const meta: Meta = { title: "Records/Reports/Analytics/Compare editions" };
export default meta;
type Story = StoryObj;
function Preview() {
  const [open, setOpen] = useState(true);
  return open ? (
    <ReportingComparison
      editions={reportingEditions}
      onClose={() => setOpen(false)}
    />
  ) : (
    <p>Dialog closed</p>
  );
}
export const Default: Story = {
  render: () => {
    installFetchStub({
      ...reportingStoryRoutes(),
      "GET /analytics/editions/compare": () =>
        jsonResponse({
          left: reportingEditions[1],
          right: reportingEditions[0],
          compatible: true,
          deltas: [
            { metric: "bookings_won", absolute: 4320000, percentage: 25 },
          ],
        }),
    });
    return (
      <StoryProviders>
        <Preview />
      </StoryProviders>
    );
  },
};

export const MembershipChanged: Story = {
  render: () => {
    installFetchStub({
      ...reportingStoryRoutes(),
      "GET /analytics/editions/compare": () =>
        jsonResponse({
          left: reportingEditions[1],
          right: reportingEditions[0],
          compatible: false,
          reason:
            "Team membership changed. These captures have different populations.",
          deltas: [],
        }),
    });
    return (
      <StoryProviders>
        <Preview />
      </StoryProviders>
    );
  },
};
