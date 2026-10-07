import type { Meta, StoryObj } from "@storybook/react-vite";
import { ReportingForecastGraphs } from "./reporting.forecast";
import { forecastEvaluation } from "./reporting.scenarios";
import {
  reportingStoryRoutes,
  reportingStoryScope,
} from "./reporting.story-fixtures";
import { installFetchStub, meRoute, StoryProviders } from "./story-utils";

const meta: Meta = { title: "Records/Reports/Analytics/Forecast graphs" };
export default meta;
type Story = StoryObj;
export const Default: Story = {
  render: () => {
    installFetchStub(
      reportingStoryRoutes({
        ...forecastEvaluation,
        metrics: [],
        charts: forecastEvaluation.charts.filter(
          (chart) => chart.kind === "pipeline_movement",
        ),
      }),
    );
    return (
      <StoryProviders>
        <ReportingForecastGraphs scope={reportingStoryScope} />
      </StoryProviders>
    );
  },
};

export const CollectingHistory: Story = {
  render: () => {
    const evaluation = {
      ...forecastEvaluation,
      metrics: [],
      charts: forecastEvaluation.charts
        .filter((chart) => chart.kind === "pipeline_movement")
        .map((chart) =>
          chart.kind === "pipeline_movement"
            ? {
                ...chart,
                opening: undefined,
                closing: undefined,
                points: [],
                coverage: {
                  status: "unavailable",
                  withheld: false,
                  reason:
                    "Collecting movement history. The next successful daily capture enables comparison.",
                },
              }
            : chart,
        ),
    } satisfies typeof forecastEvaluation;
    installFetchStub(reportingStoryRoutes(evaluation));
    return (
      <StoryProviders>
        <ReportingForecastGraphs scope={reportingStoryScope} />
      </StoryProviders>
    );
  },
};

export const LiveBeforeFirstCapture: Story = {
  render: () => {
    installFetchStub({
      ...reportingStoryRoutes({ ...forecastEvaluation, charts: [] }),
      "GET /me": meRoute({
        forecast: ["read", "create"],
        report_definition: ["read"],
      }),
    });
    return (
      <StoryProviders>
        <ReportingForecastGraphs scope={reportingStoryScope} />
      </StoryProviders>
    );
  },
};
