import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  installFetchStub,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { ReportingForecastGraphs } from "./reporting.forecast";
import { forecastEvaluation } from "./reporting.scenarios";
import {
  reportingStoryRoutes,
  reportingStoryScope,
} from "./reporting.story-fixtures";

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

// On a week the cards still read the quarter and say so; Share view says it
// waits for the quarter, and Shared links stays.
export const OnAWeek: Story = {
  render: () => {
    installFetchStub({
      ...reportingStoryRoutes(forecastEvaluation),
      "GET /me": meRoute({
        report_definition: ["read"],
        forecast: ["read", "create"],
      }),
    });
    return (
      <StoryProviders>
        <ReportingForecastGraphs scope={reportingStoryScope} period="week" />
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
