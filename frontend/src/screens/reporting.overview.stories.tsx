import type { Meta, StoryObj } from "@storybook/react-vite";
import { ReportingOverview } from "./reporting.overview";
import { sdrEvaluation } from "./reporting.scenarios";
import {
  reportingStoryEvaluation,
  reportingStoryFramework,
  reportingStoryRoutes,
  reportingStoryScope,
} from "./reporting.story-fixtures";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

const meta: Meta = {
  title: "Records/Reports/Analytics/Performance",
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj;
export const TeamPerformance: Story = {
  render: () => {
    installFetchStub(reportingStoryRoutes());
    return (
      <StoryProviders>
        <ReportingOverview scope={reportingStoryScope} />
      </StoryProviders>
    );
  },
};
export const ReadOnly: Story = {
  render: () => {
    installFetchStub(reportingStoryRoutes(reportingStoryEvaluation, true));
    return (
      <StoryProviders>
        <ReportingOverview scope={reportingStoryScope} />
      </StoryProviders>
    );
  },
};
export const NoObservations: Story = {
  render: () => {
    const evaluation = {
      ...reportingStoryEvaluation,
      charts: reportingStoryEvaluation.charts.map((chart) => ({
        ...chart,
        points: [],
        coverage: { ...chart.coverage, reason: "No matching observations" },
      })),
    };
    installFetchStub(reportingStoryRoutes(evaluation));
    return (
      <StoryProviders>
        <ReportingOverview scope={reportingStoryScope} />
      </StoryProviders>
    );
  },
};
export const MissingTargets: Story = {
  render: () => {
    const evaluation = {
      ...reportingStoryEvaluation,
      charts: reportingStoryEvaluation.charts.map((chart) => ({
        ...chart,
        points: chart.points.map((point) => ({ ...point, target: undefined })),
      })),
    };
    installFetchStub(reportingStoryRoutes(evaluation));
    return (
      <StoryProviders>
        <ReportingOverview scope={reportingStoryScope} />
      </StoryProviders>
    );
  },
};
export const QueryError: Story = {
  render: () => {
    installFetchStub({
      ...reportingStoryRoutes(),
      "GET /analytics/evaluate": () =>
        jsonResponse(
          {
            title: "Reporting unavailable",
            detail: "Try again after the service reconnects.",
            status: 503,
          },
          503,
        ),
    });
    return (
      <StoryProviders>
        <ReportingOverview scope={reportingStoryScope} />
      </StoryProviders>
    );
  },
};
export const Loading: Story = {
  render: () => {
    installFetchStub({
      ...reportingStoryRoutes(),
      "GET /analytics/evaluate": () => new Promise<Response>(() => {}),
    });
    return (
      <StoryProviders>
        <ReportingOverview scope={reportingStoryScope} />
      </StoryProviders>
    );
  },
};

export const SDROutcomes: Story = {
  render: () => {
    const routes = reportingStoryRoutes(sdrEvaluation);
    routes["GET /analytics/framework"] = () =>
      jsonResponse({
        ...reportingStoryFramework,
        definition: { ...reportingStoryFramework.definition, template: "sdr" },
      });
    routes["GET /analytics/metrics"] = () =>
      jsonResponse({
        metrics: sdrEvaluation.metrics.map((metric) => ({
          id: metric.id,
          version: "1",
          unit: "count",
          supports_target: true,
          temporal_basis: "event_period",
          definition:
            "Independent confirmed outcomes credited to their original SDR.",
          blocks: ["sdr_outcomes", "target_progress"],
        })),
      });
    installFetchStub(routes);
    return (
      <StoryProviders>
        <ReportingOverview scope={reportingStoryScope} />
      </StoryProviders>
    );
  },
};
