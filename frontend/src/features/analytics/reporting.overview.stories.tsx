import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../../testing/story-utils";
import { ReportingOverview } from "./reporting.overview";
import { sdrEvaluation } from "./reporting.scenarios";
import {
  reportingLeadsEvaluation,
  reportingPartnersEvaluation,
  reportingSectionRoutes,
  reportingStoryEvaluation,
  reportingStoryFramework,
  reportingStoryRoutes,
  reportingStoryScope,
} from "./reporting.story-fixtures";

const meta: Meta = {
  title: "Records/Reports/Analytics/Performance",
  parameters: { layout: "padded" },
  beforeEach: () => {
    const previousHash = globalThis.location.hash;
    globalThis.location.hash = "#/analytics/performance";
    return () => {
      globalThis.location.hash = previousHash;
    };
  },
};
export default meta;
type Story = StoryObj;
export const TeamPerformance: Story = {
  render: () => {
    installFetchStub(reportingStoryRoutes());
    return (
      <StoryProviders>
        <ReportingOverview view="sales" scope={reportingStoryScope} />
      </StoryProviders>
    );
  },
};
export const ReadOnly: Story = {
  render: () => {
    installFetchStub(reportingStoryRoutes(reportingStoryEvaluation, true));
    return (
      <StoryProviders>
        <ReportingOverview view="sales" scope={reportingStoryScope} />
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
        <ReportingOverview view="sales" scope={reportingStoryScope} />
      </StoryProviders>
    );
  },
};
export const MissingTargets: Story = {
  render: () => {
    const evaluation = {
      ...reportingStoryEvaluation,
      metrics: reportingStoryEvaluation.metrics.map((metric) => ({
        ...metric,
        target: undefined,
        target_actual: undefined,
        attainment: undefined,
      })),
      charts: reportingStoryEvaluation.charts.map((chart) => ({
        ...chart,
        points: chart.points.map((point) => ({ ...point, target: undefined })),
      })),
    };
    installFetchStub(reportingStoryRoutes(evaluation));
    return (
      <StoryProviders>
        <ReportingOverview view="sales" scope={reportingStoryScope} />
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
        <ReportingOverview view="sales" scope={reportingStoryScope} />
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
        <ReportingOverview view="sales" scope={reportingStoryScope} />
      </StoryProviders>
    );
  },
};

function renderSdr(withTargets: boolean) {
  const evaluation = withTargets
    ? sdrEvaluation
    : {
        ...sdrEvaluation,
        metrics: sdrEvaluation.metrics.map((metric) => ({
          ...metric,
          target: undefined,
          attainment: undefined,
        })),
        charts: sdrEvaluation.charts.map((chart) => ({
          ...chart,
          points: chart.points.map((point) => ({
            ...point,
            target: undefined,
          })),
        })),
      };
  const routes = reportingStoryRoutes(evaluation);
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
      <ReportingOverview view="sdr" scope={reportingStoryScope} />
    </StoryProviders>
  );
}
export const SDROutcomes: Story = { render: () => renderSdr(true) };
export const SDRWithoutTargets: Story = { render: () => renderSdr(false) };
export const SDRWithoutTargetsDark: Story = {
  ...SDRWithoutTargets,
  globals: { theme: "dark" },
};
export const TeamPerformanceDark: Story = {
  ...TeamPerformance,
  globals: { theme: "dark" },
};

export const InvalidDateRange: Story = {
  render: () => {
    globalThis.location.hash =
      "#/analytics/performance?period=custom&from=2025-01-01&through=2026-10-31";
    installFetchStub({
      ...reportingStoryRoutes(),
      "GET /analytics/evaluate": () =>
        jsonResponse(
          {
            status: 400,
            code: "reporting_interval_invalid",
            detail: "choose an interval of no more than twelve months",
          },
          400,
        ),
    });
    return (
      <StoryProviders>
        <ReportingOverview view="sales" scope={reportingStoryScope} />
      </StoryProviders>
    );
  },
};

export const CustomRangeThroughFutureMonthEnd: Story = {
  render: () => {
    globalThis.location.hash =
      "#/analytics/performance?period=custom&from=2026-09-01&through=2026-10-31";
    const evaluation: typeof reportingStoryEvaluation = {
      ...reportingStoryEvaluation,
      selection: {
        ...reportingStoryEvaluation.selection,
        period: "custom",
        interval: {
          start_at: reportingStoryEvaluation.context.interval.start_at,
          end_at: "2026-10-31T23:00:00Z",
        },
      },
      context: { ...reportingStoryEvaluation.context, period_kind: "custom" },
      charts: reportingStoryEvaluation.charts.map((chart) => ({
        ...chart,
        points: chart.points.map((point) => ({ ...point, target: undefined })),
        allocated_target: undefined,
        allocation_difference: undefined,
      })),
    };
    installFetchStub(reportingStoryRoutes(evaluation));
    return (
      <StoryProviders>
        <ReportingOverview view="sales" scope={reportingStoryScope} />
      </StoryProviders>
    );
  },
};
export const CustomRangeThroughFutureMonthEndDark: Story = {
  ...CustomRangeThroughFutureMonthEnd,
  globals: { theme: "dark" },
};
export const InvalidDateRangeDark: Story = {
  ...InvalidDateRange,
  globals: { theme: "dark" },
};

function renderView(view: "leads" | "partners") {
  globalThis.location.hash = `#/analytics/${view}`;
  installFetchStub(
    reportingSectionRoutes(
      view === "leads" ? reportingLeadsEvaluation : reportingPartnersEvaluation,
    ),
  );
  return (
    <StoryProviders>
      <ReportingOverview view={view} scope={reportingStoryScope} />
    </StoryProviders>
  );
}

// Leads by origin and by status moved to, the qualified share against its
// target, and the first response time.
export const Leads: Story = { render: () => renderView("leads") };
// Partner money by partner and kind, one partner the reader cannot open.
export const Partners: Story = { render: () => renderView("partners") };
export const PartnersDark: Story = {
  ...Partners,
  globals: { theme: "dark" },
};
export const LeadsPhone: Story = {
  ...Leads,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};
