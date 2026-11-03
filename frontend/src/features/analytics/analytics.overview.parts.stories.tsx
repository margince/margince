// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useQuery } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { expect, userEvent, within } from "storybook/test";
import { StatCard } from "../../design-system/atoms";
import { Panel, PanelBody } from "../../design-system/panel";
import { StoryProviders } from "../../testing/story-utils";
import { useAnalyticsContext } from "./analytics.context";
import { DecisionsPanel } from "./analytics.decisions";
import { StockCharts } from "./analytics.overview.board";
import { ForecastCoverage } from "./analytics.overview.decision";
import { DoNext } from "./analytics.overview.donext";
import { ADMIN_GRANTS, SELF } from "./analytics.overview.fixtures";
import { HeroAside } from "./analytics.overview.hero";
import { InterventionPanel } from "./analytics.overview.interventions";
import { SalesOverviewMetrics } from "./analytics.overview.metrics";
import { MonthlyTargets } from "./analytics.overview.monthly";
import { MyProspecting } from "./analytics.overview.prospecting";
import { prospectingWeeks } from "./analytics.overview.prospectingdata";
import {
  ProspectingReadings,
  ProspectingResults,
} from "./analytics.overview.prospectinghero";
import { OutcomeHistory } from "./analytics.overview.prospectingweeks";
import { SalesReview } from "./analytics.overview.sales";
import { OpenPipelineLine } from "./analytics.overview.salesreadings";
import {
  frames,
  prospectingOn,
  quarterSales,
} from "./analytics.overview.scenariodata";
import {
  installOverviewServer,
  isSetup,
  OVERVIEW_SCENARIOS,
  type OverviewView,
  overviewAddress,
} from "./analytics.overview.scenarios.fixtures";
import { EvidenceRecords } from "./reporting.evidencerecords";
import {
  useReportingCatalog,
  useReportingFramework,
} from "./reporting.queries";
import {
  reportingStoryEvaluation,
  reportingStoryRoutes,
} from "./reporting.story-fixtures";
import { WrittenSummary } from "./reporting.written";
import "./analytics.css";

const meta: Meta = {
  title: "Records/Reports/Analytics/Overview parts",
  parameters: { layout: "padded" },
  beforeEach: () => {
    const hash = globalThis.location.hash;
    return () => {
      globalThis.location.hash = hash;
    };
  },
};
export default meta;
type Story = StoryObj;
const sales = quarterSales();
const prospecting = prospectingOn(frames.weekAcrossMonths);

function story(view: OverviewView, children: ReactNode): Story {
  return {
    render: () => {
      const setup = OVERVIEW_SCENARIOS.populated.views[view];
      if (!isSetup(setup)) throw new Error(`No populated ${view} fixture`);
      const evidence = reportingStoryRoutes()["GET /analytics/evidence"];
      if (!evidence)
        throw new Error("The reporting fixture must serve evidence");
      globalThis.location.hash = overviewAddress(view);
      installOverviewServer({
        ...setup,
        routes: {
          ...setup.routes,
          evidence: () => evidence(null),
          grants: { ...ADMIN_GRANTS, activity: ["read", "create", "update"] },
        },
      });
      return <StoryProviders>{children}</StoryProviders>;
    },
  };
}

function RoleOverview({ view }: Readonly<{ view: "sales" | "prospecting" }>) {
  const catalog = useReportingCatalog();
  const context = useAnalyticsContext();
  const workflow = useReportingFramework();
  if (!catalog.data || !context.data) return null;
  return view === "sales" ? (
    <SalesReview
      catalog={catalog.data}
      scope={context.data.default_scope}
      switcher={null}
      scopeControl={null}
      emptyNotice={() => null}
    />
  ) : (
    <MyProspecting
      catalog={catalog.data}
      context={context.data}
      workflow={workflow}
      switcher={null}
    />
  );
}

function SalesReadings({
  evaluation = sales,
}: Readonly<{ evaluation?: typeof sales }>) {
  return (
    <SalesOverviewMetrics
      evaluation={evaluation}
      forecastHref="#/analytics/forecast"
      onEvidence={() => {}}
      aside={
        <HeroAside
          evaluation={evaluation}
          charts={evaluation}
          onApply={() => {}}
          targetSetup={null}
        />
      }
    />
  );
}

function Charts() {
  const charts = useQuery({
    queryKey: ["story-sales-charts"],
    queryFn: async () => sales,
  });
  return (
    <>
      <OpenPipelineLine evaluation={sales} onEvidence={() => {}} />
      <StockCharts charts={charts} onEvidence={() => {}} />
    </>
  );
}

function ProspectingFigures() {
  const weeks = useQuery({
    queryKey: ["story-prospecting-weeks"],
    queryFn: async () =>
      prospectingWeeks(
        frames.weekAcrossMonths,
        "leads_qualified",
        [2, 3, 1, 4, 2, 5],
      ),
  });
  return (
    <>
      <ProspectingResults
        evaluation={prospecting}
        outcome="leads_qualified"
        workflowState="known"
        weeks={weeks}
        onEvidence={() => {}}
        onRetryWorkflow={() => {}}
        aside={
          <MonthlyTargets evaluation={prospecting} ownScope={`owner:${SELF}`} />
        }
      />
      <ProspectingReadings
        evaluation={prospecting}
        summary={{ isError: false }}
        onEvidence={() => {}}
      />
      <OutcomeHistory weeks={weeks} onEvidence={() => {}} />
    </>
  );
}

const gaps = {
  ...sales,
  metrics: sales.metrics.map((metric) =>
    metric.id === "forecast_landing"
      ? {
          ...metric,
          forecast_basis: metric.forecast_basis
            ? {
                ...metric.forecast_basis,
                eligible_count: 18,
                priced_count: 16,
                fx_missing_count: 1,
                included_count: 15,
              }
            : undefined,
        }
      : metric,
  ),
};

const managerCall = {
  ...sales,
  metrics: sales.metrics.map((metric): typeof metric =>
    metric.id === "forecast_landing" && metric.forecast_basis
      ? {
          ...metric,
          value: 79_000_000,
          forecast_basis: {
            ...metric.forecast_basis,
            call_minor: 79_000_000,
            call_at: "2026-10-30T09:00:00Z",
            landing: {
              ...metric.forecast_basis.landing,
              measure: "manager_call",
              amount_minor: 79_000_000,
              remaining_minor: 37_000_000,
              system_minor: 76_000_000,
            },
          },
          target_comparison: metric.target_comparison
            ? {
                ...metric.target_comparison,
                basis: "manager_call",
                landing: 79_000_000,
                gap: 1_000_000, // ds:ignore forecast shortfall in minor units
              }
            : undefined,
        }
      : metric,
  ),
};
export const ManagerCallAndDate = story(
  "sales",
  <SalesReadings evaluation={managerCall} />,
);

export const SalesReadingsAndTarget = story("sales", <SalesReadings />);
export const SalesReadingsOnPhone = {
  ...SalesReadingsAndTarget,
  tags: ["uat-phone"],
  globals: { viewport: { value: "phone" } },
};
export const ForecastPopulationGaps = story(
  "sales",
  <StatCard
    label="Pipeline due this quarter"
    value="€1.36m"
    detail={<ForecastCoverage evaluation={gaps} />}
  />,
);
export const PipelineAndCharts = story("sales", <Charts />);
export const ProspectingFiguresAndHistory = story(
  "prospecting",
  <ProspectingFigures />,
);
export const NextWork = story("prospecting", <DoNext />);
export const FirstReviewDecision = story("sales", <DecisionsPanel />);
export const CheckReadFailure = story(
  "sales",
  <InterventionPanel
    interventions={{ state: "failed", list: undefined, unreadSources: 0 }}
    dials={new Map()}
    emptyLabel="No findings"
  />,
);
export const EvidenceRows = story(
  "sales",
  <Panel title="Supporting records">
    <PanelBody>
      <EvidenceRecords
        evaluation={reportingStoryEvaluation}
        reference={{ metric: "bookings_won", context_id: "interval" }}
        zero={false}
        unit="money"
        selectedValue={reportingStoryEvaluation.metrics[0]?.value}
        onClose={() => {}}
      />
    </PanelBody>
  </Panel>,
);
export const DetailReportSummary = story(
  "sales",
  <Panel title="Performance">
    <PanelBody>
      <WrittenSummary evaluation={reportingStoryEvaluation} />
    </PanelBody>
  </Panel>,
);
export const SalesReviewPage = story("sales", <RoleOverview view="sales" />);
export const SalesReviewPageDark = {
  ...SalesReviewPage,
  globals: { theme: "dark" },
};
export const MyProspectingPage = story(
  "prospecting",
  <RoleOverview view="prospecting" />,
);
export const MyProspectingPageOnPhone = {
  ...MyProspectingPage,
  tags: ["uat-phone"],
  globals: { viewport: { value: "phone" } },
};

export const IncompletePipelineReadings: Story = {
  ...story("sales", <SalesReadings evaluation={gaps} />),
  play: async ({ canvasElement }) => {
    await userEvent.click(
      within(canvasElement).getByRole("button", { name: "Evidence" }),
    );
    const body = within(document.body);
    await expect(body.getByText(/2 deals with no amount/)).toBeVisible();
    await expect(body.getByText(/1 deal with no exchange rate/)).toBeVisible();
  },
};
