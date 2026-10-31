import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../../testing/story-utils";
import { ANSWER } from "./analytics.questions.testkit";
import { ReportingCharts } from "./reporting.charts";
import {
  reportingCatalogClient,
  reportingStoryEvaluation,
} from "./reporting.story-fixtures";

const meta: Meta = {
  title: "Records/Reports/Analytics/Graph layout",
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj;
function Preview() {
  const [reference, select] = useState("");
  return (
    <StoryProviders client={reportingCatalogClient()}>
      <ReportingCharts
        evaluation={reportingStoryEvaluation}
        onEvidence={(reference) => select(JSON.stringify(reference))}
      />
      <p aria-live="polite">{reference}</p>
    </StoryProviders>
  );
}
export const FourQuestions: Story = { render: () => <Preview /> };

export const PartialHistory: Story = {
  render: () => (
    <StoryProviders client={reportingCatalogClient()}>
      <ReportingCharts
        evaluation={{
          ...reportingStoryEvaluation,
          charts: reportingStoryEvaluation.charts.map((chart) => ({
            ...chart,
            coverage: {
              ...chart.coverage,
              status: "partial",
              reason:
                "Older records have incomplete attribution. Only known contributions are included.",
            },
          })),
        }}
        onEvidence={() => {}}
      />
    </StoryProviders>
  ),
};

export const LongStageNames: Story = {
  render: () => (
    <StoryProviders client={reportingCatalogClient()}>
      <ReportingCharts
        evaluation={{
          ...reportingStoryEvaluation,
          charts: reportingStoryEvaluation.charts.map((chart) =>
            chart.kind === "stage_distribution"
              ? {
                  ...chart,
                  points: chart.points.map((point, index) => ({
                    ...point,
                    label:
                      index % 2 === 0
                        ? "Procurement, legal and security approval"
                        : "Technical pilot and stakeholder alignment",
                  })),
                }
              : chart,
          ),
        }}
        onEvidence={() => {}}
      />
    </StoryProviders>
  ),
};

const boundEvaluation = {
  ...reportingStoryEvaluation,
  selection: {
    ...reportingStoryEvaluation.selection,
    layout: [
      {
        cards: [
          {
            id: "bound",
            kind: "question" as const,
            width: "full" as const,
            question: {
              entity: "deals-by-stage",
              group_by: ["stage_id"],
              measures: [{ fn: "count" as const }],
              binding: "ref-1",
            },
          },
        ],
      },
    ],
  },
};

// A card bound to a saved question with filters, as a reader the filters
// cover sees it, and as one they do not.
function BoundCard({ restricted }: Readonly<{ restricted: boolean }>) {
  installFetchStub({
    "GET /analytics/reports/report/questions/bound/answer": () =>
      jsonResponse(
        restricted
          ? { status: "restricted", binding: { filtered: true } }
          : {
              status: "answered",
              answer: ANSWER,
              binding: {
                filtered: true,
                period_basis: {
                  timezone: "Europe/Berlin",
                  fiscal_year_start_month: 1,
                },
              },
            },
      ),
  });
  return (
    <StoryProviders client={reportingCatalogClient()}>
      <ReportingCharts
        evaluation={boundEvaluation}
        report={{ id: "report" }}
        onEvidence={() => {}}
      />
    </StoryProviders>
  );
}

export const BoundQuestion: Story = {
  render: () => <BoundCard restricted={false} />,
};

export const BoundQuestionRestricted: Story = {
  render: () => <BoundCard restricted />,
};
