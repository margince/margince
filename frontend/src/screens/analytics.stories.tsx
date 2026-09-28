// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { screen, userEvent, within } from "storybook/test";
import { StatStrip } from "../design-system/statstrip";
import { AnalyticsScreen, ForecastTile } from "./analytics";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  type RouteMap,
  StoryProviders,
} from "./story-utils";

// The Reports screen for the fe-uat render gate. All three report segments draw
// into ONE surface — a titled card whose trailing action row carries "Explain
// this number" — so each story below is that same card holding a different
// report, which is exactly the drift these stories exist to catch: the screen
// body had no render coverage while three segments grew three different looks.
//
// Every read is stubbed off the same shapes reports.test.tsx exercises; the
// segment stories click the picker in play() because the screen owns the
// selection, and fe-uat waits for the interaction to settle before capturing.

const pipelines = {
  data: [
    {
      id: "pl",
      name: "Sales",
      is_default: true,
      position: 0,
      stages: [
        {
          id: "pl-s1",
          pipeline_id: "pl",
          name: "Qualify",
          position: 1,
          semantic: "open",
          win_probability: 20,
        },
        {
          id: "pl-s2",
          pipeline_id: "pl",
          name: "Proposal sent",
          position: 2,
          semantic: "open",
          win_probability: 60,
        },
      ],
    },
  ],
  page: { next_cursor: null, has_more: false },
};

function run(report: string, rows: Record<string, unknown>[]) {
  return jsonResponse({
    report,
    plan: {},
    columns: [],
    rows,
    // The frame every result carries, so the story shows the caption a reader
    // actually meets under each report.
    as_of: "2026-03-04T09:00:00Z",
    timezone: "Europe/Berlin",
    base_currency: "EUR",
    fiscal_year_start_month: 1,
    derivation_url: `/v1/reports/${report}/derivation?by=stage_id&agg=sum:amount_minor:raw_minor&stage_id=pl-s1`,
  });
}

// The converted report returns one row per stage and no currency column: each
// deal was priced into the base currency before anything was summed. Each row
// carries its own handle, so each stage draws its own explain trigger.
const stageHandle = (stageId: string) =>
  `/v1/reports/pipeline-current/derivation?by=stage_id&agg=sum:amount_base_minor:raw_minor&stage_id=${stageId}`;

const stageRows = [
  {
    stage_id: "pl-s1",
    raw_minor: 24686,
    weighted_minor: 4938,
    deal_count: 2,
    derivation_url: stageHandle("pl-s1"),
  },
  {
    stage_id: "pl-s2",
    raw_minor: 1850000,
    weighted_minor: 1110000,
    deal_count: 5,
    derivation_url: stageHandle("pl-s2"),
  },
];

const forecastRows = [
  {
    forecast_category: "commit",
    raw_minor: 2800000,
    weighted_minor: 2380000,
    deal_count: 4,
    currency: "EUR",
  },
  {
    forecast_category: "best_case",
    raw_minor: 1250000,
    weighted_minor: 500000,
    deal_count: 3,
    currency: "EUR",
  },
  {
    forecast_category: "pipeline",
    raw_minor: 940000,
    weighted_minor: 188000,
    deal_count: 6,
    currency: "EUR",
  },
  {
    forecast_category: "slipped",
    raw_minor: 90000,
    weighted_minor: 45000,
    deal_count: 1,
    currency: "EUR",
  },
];

const companyRows = [
  {
    company_id: "BÄR Pharma GmbH",
    raw_minor: 2500000,
    deal_count: 4,
    currency: "EUR",
  },
  {
    company_id: "Brandt Systemtechnik",
    raw_minor: 870000,
    deal_count: 2,
    currency: "EUR",
  },
];

const derivation = {
  report: "pipeline-current",
  definition:
    "Sum of open-deal amounts in the base currency, grouped by stage, in Qualify",
  plan: {},
  columns: ["deal", "amount"],
  rows: [
    { deal: "BÄR Pharma — Packaging QA", amount: "€123.43" },
    { deal: "Brandt — Line QA Retrofit", amount: "€123.43" },
  ],
};

const routes: RouteMap = {
  "GET /me": meRoute({ forecast: ["create"] }),
  "GET /analytics/context": () =>
    jsonResponse({
      default_scope: { kind: "workspace", label: "Whole company" },
      allowed_scopes: [{ kind: "workspace", label: "Whole company" }],
      capabilities: {
        view_manager_forecast: true,
        submit_manager_forecast: true,
      },
      as_of: "2026-09-04T00:00:00Z",
      timezone: "Europe/Berlin",
      base_currency: "EUR",
    }),
  "GET /pipelines": () => jsonResponse(pipelines),
  "POST /reports/pipeline-current": () => run("pipeline-current", stageRows),
  "POST /reports/forecast": () => run("forecast", forecastRows),
  "POST /reports/win-loss": () =>
    run("win-loss", [
      {
        status: "won",
        deal_count: 8,
        raw_minor: 500000,
        median_days: 21,
        p75_days: 40,
      },
      {
        status: "lost",
        deal_count: 4,
        raw_minor: 200000,
        median_days: 55,
        p75_days: null,
      },
    ]),
  "POST /reports/stage-age": () =>
    run("stage-age", [
      { stage_id: "pl-s1", deal_count: 6, median_days: 12, p75_days: 30 },
      { stage_id: "pl-s2", deal_count: 3, median_days: null, p75_days: null },
    ]),
  "POST /reports/open-deals-per-company": () =>
    run("open-deals-per-company", companyRows),
  "GET /reports/pipeline-current/derivation": () => jsonResponse(derivation),
};

function screenStory() {
  installFetchStub(routes);
  return (
    <StoryProviders>
      <AnalyticsScreen />
    </StoryProviders>
  );
}

// Press each named button in turn, waiting for it to exist first.
//
// VARIADIC rather than one name, because the screen's two sections are a tab
// apart: the report cards live under Pipeline and the Forecast tab is the
// default, so anything inside a report card takes two presses to reach.
//
// The NAME stays `clickButton` deliberately. Widening it to take several was a
// rename first, and the rename cost a red pipeline: this file gains stories on
// main while a branch is open, a new one arrived calling the old name, and the
// merge compiled a call to a symbol neither side had broken by itself. A
// signature can widen without every existing caller having to move.
const clickButton =
  (...names: readonly string[]) =>
  async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    for (const name of names) {
      await userEvent.click(
        await within(canvasElement).findByRole("button", { name }),
      );
    }
  };

const meta: Meta = { title: "Records/Reports" };
export default meta;

type Story = StoryObj;

// The default segment: the stage table inside the report card, the explain verb
// in the card's own action row.
export const DealsByStage: Story = { render: screenStory };

// Five money figures read across as one comparison — the strip, under the
// callout that says how to read the second figure in each slot.
export const Forecast: Story = {
  render: screenStory,
  play: clickButton("Forecast"),
};

// The company table, which is a CARD in the Pipeline section rather than a
// segment of its own — there is no per-report picker, so the tab is the whole
// selection. This story asked for a button by the card's title and found none.
export const OpenDealsPerCompany: Story = {
  render: screenStory,
  play: clickButton("Deals"),
};

// The performance section: closed outcomes beside stage velocity, every
// duration the server's own, and a withheld percentile rendered as words
// rather than a zero.
export const Performance: Story = {
  render: screenStory,
  play: clickButton("Performance"),
};

// The seat's own outcomes: pipeline and meetings under an owner lens.
const ownLensRoutes: RouteMap = {
  ...routes,
  "GET /analytics/context": () =>
    jsonResponse({
      default_scope: { kind: "owner", id: "u-rep-1", label: "Riley Rep" },
      allowed_scopes: [{ kind: "owner", id: "u-rep-1", label: "Riley Rep" }],
      capabilities: {
        view_manager_forecast: false,
        submit_manager_forecast: false,
      },
      as_of: "2026-09-04T00:00:00Z",
      timezone: "Europe/Berlin",
      base_currency: "EUR",
    }),
  "POST /reports/activities-by-kind": () =>
    run("activities-by-kind", [
      { meeting_status: "booked", meetings: 2 },
      { meeting_status: "held", meetings: 3 },
      { meeting_status: "no_show", meetings: 1 },
    ]),
};

// Source health under the ops grant: one read source with its instant, and
// every unread state in its own words.
const coverageRoutes: RouteMap = {
  ...routes,
  "GET /analytics/coverage": () =>
    jsonResponse({
      run_id: "r1",
      as_of: "2026-09-05T02:00:00Z",
      sources: [
        {
          source: "mail",
          state: "checked",
          checked_through: "2026-09-05T01:30:00Z",
        },
        {
          source: "offers",
          state: "checked",
          checked_through: "2026-09-05T02:00:00Z",
        },
        { source: "calendar", state: "stale" },
        { source: "documents", state: "not_connected" },
      ],
    }),
};

const deliveryRoutes: RouteMap = {
  ...routes,
  "POST /reports/projects-by-phase": () =>
    run("projects-by-phase", [
      {
        phase: "delivering",
        projects: 3,
        open_deal_value_minor: 400000,
        won_deal_value_minor: 900000,
      },
    ]),
  "POST /reports/project-commitments": () =>
    run("project-commitments", [
      {
        project_id: "p1",
        name: "Rollout Nord",
        phase: "delivering",
        open_commitments: 5,
        overdue_commitments: 2,
      },
    ]),
  "POST /reports/projects-gone-quiet": () => run("projects-gone-quiet", []),
};

export const Delivery: Story = {
  render: () => {
    installFetchStub(deliveryRoutes);
    return (
      <StoryProviders>
        <AnalyticsScreen />
      </StoryProviders>
    );
  },
  play: clickButton("Delivery"),
};

export const DataCoverage: Story = {
  render: () => {
    installFetchStub({
      ...coverageRoutes,
      "GET /me": meRoute({ data_coverage: ["read"] }),
    });
    return (
      <StoryProviders>
        <AnalyticsScreen />
      </StoryProviders>
    );
  },
  play: clickButton("Data coverage"),
};

export const MyOutcomes: Story = {
  render: () => {
    installFetchStub(ownLensRoutes);
    return (
      <StoryProviders>
        <AnalyticsScreen />
      </StoryProviders>
    );
  },
  play: clickButton("My outcomes"),
};

// The seat's readings with NOTHING behind them. The lens answered and the seat
// holds no open deal, which is a reading — and a different one from a read that
// failed or one still in flight, all three of which used to say "Nothing to
// read yet".
export const MyOutcomesEmpty: Story = {
  render: () => {
    installFetchStub({
      ...ownLensRoutes,
      "POST /reports/pipeline-current": () => run("pipeline-current", []),
    });
    return (
      <StoryProviders>
        <AnalyticsScreen />
      </StoryProviders>
    );
  },
  play: clickButton("My outcomes"),
};

// "Explain this number" open: the report card above, the derivation card below
// it, both the same titled-card surface.
export const Explain: Story = {
  render: screenStory,
  // Pipeline first: the explain verb belongs to a report card's action row, and
  // the Forecast section the screen opens on draws no report cards at all.
  play: clickButton("Deals", "Explain this number"),
};

// One stage's figure explained in a drawer, over the table it came from.
export const ExplainRow: Story = {
  render: screenStory,
  play: async (context) => {
    await clickButton("Deals", "Explain Qualify")(context);
    await screen.findByRole("dialog");
  },
};

// The derivation card while its read is still in flight: the definition line,
// then two skeleton lines where the breakdown will land.
export const ExplainLoading: Story = {
  render: () => {
    installFetchStub({
      ...routes,
      "GET /reports/pipeline-current/derivation": () =>
        new Promise<Response>(() => {}),
    });
    return (
      <StoryProviders>
        <AnalyticsScreen />
      </StoryProviders>
    );
  },
  play: async (context) => {
    await clickButton("Deals", "Explain this number")(context);
    await within(context.canvasElement).findByText("How this number is built");
  },
};

// The four absences a slot has to tell apart, side by side, because they are
// four different facts and one of them used to be drawn as €0.00. A category
// the report returned no row for was measured in no currency at all; a band of
// deals nobody priced has a currency but no figure; a band whose deals ARE
// counted answers with the count and says the amount is what is missing; a
// stored zero IS a figure.
export const ForecastAbsences: Story = {
  render: () => (
    <StoryProviders>
      <StatStrip>
        <ForecastTile
          label="No deals"
          amountMinor={null}
          weightedMinor={null}
          currency="EUR"
          locale="en"
        />
        <ForecastTile
          label="Unpriced"
          amountMinor={null}
          weightedMinor={null}
          currency={null}
          locale="en"
        />
        <ForecastTile
          label="Counted, unpriced"
          amountMinor={null}
          weightedMinor={null}
          dealCount={7}
          currency="EUR"
          locale="en"
        />
        <ForecastTile
          label="Stored zero"
          amountMinor={0}
          weightedMinor={0}
          currency="EUR"
          locale="en"
        />
      </StatStrip>
    </StoryProviders>
  ),
};

// A currency whose scale is not the euro's, so a minor-unit slip shows up as
// three orders of magnitude rather than as a rounding difference.
export const ForecastZeroDecimalCurrency: Story = {
  render: () => (
    <StoryProviders>
      <StatStrip>
        <ForecastTile
          label="Commit"
          amountMinor={4500000000}
          weightedMinor={1800000000}
          currency="VND"
          locale="en"
        />
      </StatStrip>
    </StoryProviders>
  ),
};

// One forecast slot on its own, in the plate it actually renders in: the raw
// total is the reading, the weighted total the basis it was drawn from.
export const ForecastSlots: Story = {
  render: () => (
    <StoryProviders>
      <StatStrip>
        <ForecastTile
          label="Commit"
          amountMinor={2800000}
          weightedMinor={2380000}
          currency="EUR"
          locale="en"
        />
        <ForecastTile
          label="Best case"
          amountMinor={1250000}
          weightedMinor={500000}
          currency="EUR"
          locale="en"
        />
        <ForecastTile
          label="Omitted"
          amountMinor={0}
          currency="EUR"
          locale="en"
        />
        {/* The money covers only part of what the category holds, so the
            second fragment states the GAP instead of the plain count. */}
        <ForecastTile
          label="Pipeline"
          amountMinor={4100000}
          weightedMinor={1600000}
          dealCount={9}
          pricedDeals={6}
          currency="EUR"
          locale="en"
        />
      </StatStrip>
    </StoryProviders>
  ),
};

// At 390px. The strip folds to full-width ROWS — every slot declares
// `narrow="row"` — because two slots abreast on a phone clip the label AND
// ellipsize the figure, and a clipped number is a different number. The
// hairline between rows is the plate's; the tiles lose their boxes.
export const ForecastSlotsPhone: Story = {
  ...ForecastSlots,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};

// The seat's own two readings at 390px, where the door in each row's foot has
// to stay a thumb target of its own.
export const MyOutcomesPhone: Story = {
  ...MyOutcomes,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};
