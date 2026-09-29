import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { ReportingCharts } from "./reporting.charts";
import { reportingStoryEvaluation } from "./reporting.story-fixtures";
import { StoryProviders } from "./story-utils";

const meta: Meta = {
  title: "Records/Reports/Analytics/Graph layout",
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj;
function Preview() {
  const [reference, select] = useState("");
  return (
    <StoryProviders>
      <ReportingCharts
        evaluation={reportingStoryEvaluation}
        onEvidence={(reference) => select(JSON.stringify(reference))}
      />
      <p aria-live="polite">{reference}</p>
    </StoryProviders>
  );
}
export const FourQuestions: Story = { render: () => <Preview /> };
