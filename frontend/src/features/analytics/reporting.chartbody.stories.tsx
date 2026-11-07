import type { Meta, StoryObj } from "@storybook/react-vite";
import { StoryProviders } from "../../testing/story-utils";
import { ChartBody } from "./reporting.chartbody";
import type { ReportingEvaluation } from "./reporting.model";
import {
  reportingCatalogClient,
  reportingLeadsEvaluation,
  reportingPartnersEvaluation,
} from "./reporting.story-fixtures";

const meta: Meta = { title: "Records/Reports/Analytics/Chart body" };
export default meta;
type Story = StoryObj;

function breakdownOf(
  evaluation: ReportingEvaluation,
  metric: ReportingEvaluation["charts"][number]["metric"],
) {
  const chart = evaluation.charts.find(
    (each) => each.kind === "breakdown" && each.metric === metric,
  );
  if (!chart) throw new Error(`the fixture has no ${metric} breakdown`);
  return (
    <StoryProviders client={reportingCatalogClient()}>
      <ChartBody chart={chart} evaluation={evaluation} onEvidence={() => {}} />
    </StoryProviders>
  );
}

// A count split by group: the bars carry the figures, so no record count.
export const LeadOrigins: Story = {
  render: () => breakdownOf(reportingLeadsEvaluation, "leads_created"),
};

// Money split by partner and kind, with how many deals each group holds.
export const PartnerPipeline: Story = {
  render: () => breakdownOf(reportingPartnersEvaluation, "partner_pipeline"),
};
export const PartnerPipelineDark: Story = {
  ...PartnerPipeline,
  globals: { theme: "dark" },
};
