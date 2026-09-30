import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { SaveReportingDialog } from "./reporting.save";
import {
  reportingStoryEvaluation,
  reportingStoryRoutes,
} from "./reporting.story-fixtures";
import { installFetchStub, StoryProviders } from "./story-utils";

const meta: Meta = { title: "Records/Reports/Analytics/Save report" };
export default meta;
type Story = StoryObj;
function Preview() {
  const [open, setOpen] = useState(true);
  return open ? (
    <SaveReportingDialog
      selection={reportingStoryEvaluation.selection}
      onClose={() => setOpen(false)}
      onSaved={() => setOpen(false)}
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
