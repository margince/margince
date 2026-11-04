// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../../api/schema";
import { formatAnalyticsMoneyOrAbsent } from "../../format/format";
import { REPORTING_FIXTURE_ZONE } from "../../testing/reporting.fixtures";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../../testing/story-utils";
import { AccountCoverageView } from "./analytics.accounts";
import { CloseMonthTable } from "./analytics.closemonth";
import { ForecastAnswer } from "./analytics.forecast.answer";
import { ForecastDeals } from "./analytics.forecast.deals";
import { QualifiedAnswer } from "./analytics.forecast.qualified";
import {
  overviewCatalog,
  SELF,
  startingMovement,
} from "./analytics.overview.fixtures";
import { InitiativePanel } from "./analytics.overview.initiative";
import { upcomingMeetings } from "./analytics.overview.prospectingdata";
import { UpcomingMeetings } from "./analytics.overview.prospectingwork";
import { MetricReading } from "./analytics.overview.readings";
import {
  overviewRoutes,
  upcomingWindow,
} from "./analytics.overview.routes.fixtures";
import { quarterSales } from "./analytics.overview.scenariodata";
import { SinceLastReview } from "./analytics.overview.since";
import { StageTable } from "./analytics.pipeline";
import { CallCheckSheet } from "./inputcheck.call";
import { CheckFacts } from "./inputcheck.facts";
import { CohortTable } from "./reporting.cohort";
import { InitiativeDrawer } from "./reporting.initiativedrawer";
import {
  InitiativeReports,
  type SalesInitiative,
} from "./reporting.initiatives";
import { reportingStoryRoutes } from "./reporting.story-fixtures";
import { TargetComparison } from "./reporting.targetcomparison";

const at = "2026-10-31T12:00:00Z";
const scope = { kind: "workspace", label: "Whole company" } as const;
const evaluation = quarterSales();
const initiative: SalesInitiative = {
  id: "11111111-1111-4111-8111-111111111111",
  name: "Quarter opportunities",
  owner_id: SELF,
  objective_metric: "bookings_won",
  target_value: 80000000,
  deadline: "2026-12-31",
  status: "live",
  include_future: false,
  version: 1,
  created_at: at,
  updated_at: at,
};
const readings: components["schemas"]["ForecastReadings"] = {
  period_start: "2026-10-01",
  period_end: "2026-12-31",
  scope_kind: "workspace",
  qualification_mode: "off",
  won_minor: 42000000,
  evidence_minor: 34000000,
  best_case_minor: 60000000,
  open_minor: 136000000,
  weighted_minor: 68000000,
  eligible_count: 18,
  priced_count: 17,
  confirmed_date_count: 15,
  fx_missing_count: 0,
  as_of: at,
  timezone: REPORTING_FIXTURE_ZONE,
  base_currency: "EUR",
  contributions: [
    {
      deal_id: "22222222-2222-4222-8222-222222222222",
      deal_name: "Quarter subscription",
      base_minor: 743218965,
      weighted_minor: 445931379,
      in_open: true,
      in_won: false,
      in_evidence: true,
      in_best_case: true,
      applied_probability: 60,
      chance_in_use: 60,
      signals: [],
    },
  ],
  qualified: {
    evidence_minor: 34000000,
    best_case_minor: 60000000,
    weighted_minor: 68000000,
    excluded_minor: 1000000,
    unsupported_minor: 0,
    unknown_minor: 1000000,
    excluded_count: 1,
    unsupported_count: 0,
    unknown_count: 1,
  },
};
const check: components["schemas"]["InputCheck"] = {
  id: "check",
  type: "forecast_call_moved",
  subject_kind: "forecast_call",
  subject_id: "call",
  severity: "medium",
  status: "open",
  owner_id: SELF,
  first_seen_at: at,
  last_seen_at: at,
  inputs_changed: false,
  affected_minor: 743218965,
  currency: "EUR",
  claim: { scope_kind: "workspace", period_start: "2026-10-01" },
  observed: { comparison: "moved", changed_deals: 2, deal_ids: ["one", "two"] },
};
const coverage: components["schemas"]["AccountCoverage"] = {
  scope,
  as_of: at,
  thresholds: {
    max_engaged_seats: 1,
    untouched_days: 21,
    silent_days: 30,
    won_within_days: 180,
  },
  framework_revision: 0,
  counts: { single_threaded: 1, untouched: 0, silent: 0 },
  activity_readable: true,
  view: "single_threaded",
  withheld: 0,
  rows: [
    {
      deal: {
        id: "22222222-2222-4222-8222-222222222222",
        name: "Quarter subscription",
        amount_minor: 743218965,
        currency: "EUR",
        stage: "Proposal",
      },
      company: {
        id: "33333333-3333-4333-8333-333333333333",
        name: "Example company",
      },
      engaged_seats: 0,
      named_seats: 1,
    },
  ],
};
const summary: components["schemas"]["SdrSummary"] = {
  as_of: at,
  scope,
  workflow: "qualification",
  upcoming_meetings: 2,
  upcoming_window: upcomingWindow(at),
};
const meta: Meta = {
  title: "Records/Reports/Analytics/Quantity details",
  parameters: { layout: "padded" },
  decorators: [
    (Story) => (
      <StoryProviders>
        <Story />
      </StoryProviders>
    ),
  ],
  beforeEach: () => {
    installFetchStub({
      ...overviewRoutes(),
      ...reportingStoryRoutes(),
      "GET /analytics/metrics": () => jsonResponse(overviewCatalog),
      "GET /analytics/evaluate": () => jsonResponse(startingMovement()),
      "GET /forecast": () => jsonResponse(readings),
      "GET /analytics/account-coverage": () => jsonResponse(coverage),
      "GET /analytics/sdr-summary/meetings": () =>
        jsonResponse({
          as_of: at,
          window: summary.upcoming_window,
          rows: upcomingMeetings(at),
        }),
      "GET /analytics/initiatives": () => jsonResponse({ data: [initiative] }),
    });
  },
};
export default meta;
type Story = StoryObj;
export const Accounts: Story = {
  render: () => (
    <AccountCoverageView
      scope={scope}
      scopeControl={null}
      locale="en"
      timezone={REPORTING_FIXTURE_ZONE}
    />
  ),
};
export const CloseMonths: Story = {
  render: () => (
    <CloseMonthTable
      rows={[
        {
          close_month: "2026-12",
          raw_minor: 743218965,
          deal_count: 1,
          priced_count: 1,
        },
      ]}
      locale="en"
      baseCurrency="EUR"
    />
  ),
};
export const ForecastExplanation: Story = {
  render: () => (
    <ForecastAnswer readings={readings} locale="en" period="quarter" />
  ),
};
export const ForecastRecords: Story = {
  render: () => <ForecastDeals scope={scope} period="quarter" />,
};
export const QualifiedForecast: Story = {
  render: () => (
    <QualifiedAnswer
      readings={readings}
      money={(value) => formatAnalyticsMoneyOrAbsent(value, "EUR", "en")}
    />
  ),
};
export const InitiativeProgress: Story = {
  render: () => (
    <InitiativePanel
      selection={{ ...evaluation.selection, initiative_id: initiative.id }}
      onEvidence={() => {}}
    />
  ),
};
export const BookedMeetings: Story = {
  render: () => (
    <UpcomingMeetings
      self={SELF}
      summary={{ data: summary, isError: false }}
      title="Booked meetings"
    />
  ),
};
export const Reading: Story = {
  render: () => (
    <MetricReading
      evaluation={evaluation}
      id="bookings_won"
      onEvidence={() => {}}
    />
  ),
};
export const MovementStarting: Story = {
  render: () => (
    <SinceLastReview
      catalog={overviewCatalog}
      scope={scope}
      pipelineId={undefined}
      after="ready"
    />
  ),
};
export const StageValues: Story = {
  render: () => (
    <StageTable
      rows={[
        {
          stage_id: "proposal",
          raw_minor: 743218965,
          weighted_minor: 445931379,
          deal_count: 1,
          priced_count: 1,
        },
      ]}
      stages={[
        {
          id: "proposal",
          pipeline_id: "sales",
          name: "Proposal",
          position: 1,
          semantic: "open",
          win_probability: 60,
        },
      ]}
      locale="en"
      baseCurrency="EUR"
    />
  ),
};
export const ChangedManagerCall: Story = {
  render: () => (
    <CallCheckSheet check={check} open onClose={() => {}} closeLabel="Close" />
  ),
};
export const FindingFacts: Story = {
  render: () => <CheckFacts check={check} />,
};
export const Cohort: Story = {
  render: () => (
    <CohortTable
      label="Arrival cohort"
      chart={{
        kind: "lead_cohort",
        metric: "lead_cohort_outcomes",
        context_id: "interval",
        unit: "percent",
        coverage: { status: "ok", withheld: false },
        points: [
          {
            key: "arrivals",
            label: "Arrivals",
            value: 16.67,
            status: "ok",
            observations: 6,
            outcomes: [
              {
                key: "qualified",
                label: "Qualified",
                count: 1,
                median_days: 1.5,
              },
            ],
          },
        ],
      }}
      onEvidence={() => {}}
    />
  ),
};
export const InitiativeSetup: Story = {
  render: () => (
    <InitiativeDrawer
      initiative={{ ...initiative, status: "draft" }}
      onClose={() => {}}
    />
  ),
};
export const Initiatives: Story = { render: () => <InitiativeReports /> };
export const ForecastTarget: Story = {
  render: () => <TargetComparison evaluation={evaluation} />,
};
export const LargeReadingOnPhone: Story = { ...Reading, tags: ["uat-phone"] };
export const ForecastRecordsOnPhone: Story = {
  ...ForecastRecords,
  tags: ["uat-phone"],
};
export const ForecastRecordsDark: Story = {
  ...ForecastRecords,
  globals: { theme: "dark" },
};
