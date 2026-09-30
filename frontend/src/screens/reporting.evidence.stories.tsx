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
function Preview() {
  const [open, setOpen] = useState(true);
  return open ? (
    <ReportingEvidenceDrawer
      evaluation={reportingStoryEvaluation}
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
