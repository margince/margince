// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { screen, userEvent, within } from "storybook/test";
import { StatStrip } from "../design-system/statstrip";
import { AnalyticsScreen, ForecastTile } from "./analytics";
import { ShareViewButton } from "./analytics.share";
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
// deal was priced into the base currency before anything was summed.
const stageRows = [
  { stage_id: "pl-s1", raw_minor: 24686, weighted_minor: 4938, deal_count: 2 },
  {
    stage_id: "pl-s2",
    raw_minor: 1850000,
    weighted_minor: 1110000,
    deal_count: 5,
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
    organization_id: "BÄR Pharma GmbH",
    raw_minor: 2500000,
    deal_count: 4,
    currency: "EUR",
  },
  {
    organization_id: "Brandt Systemtechnik",
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
  "GET /me": meRoute({}),
  "GET /pipelines": () => jsonResponse(pipelines),
  "POST /reports/pipeline-current": () => run("pipeline-current", stageRows),
  "POST /reports/forecast": () => run("forecast", forecastRows),
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
  play: clickButton("Pipeline"),
};

// "Explain this number" open: the report card above, the derivation card below
// it, both the same titled-card surface.
export const Explain: Story = {
  render: screenStory,
  // Pipeline first: the explain verb belongs to a report card's action row, and
  // the Forecast section the screen opens on draws no report cards at all.
  play: clickButton("Pipeline", "Explain this number"),
};

// The three absences a slot has to tell apart, side by side, because they are
// three different facts and one of them used to be drawn as €0.00. A category
// the report returned no row for was measured in no currency at all; a band of
// deals nobody priced has a currency but no figure; a stored zero IS a figure.
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
      </StatStrip>
    </StoryProviders>
  ),
};

// The share dialog, in both states a reader meets it in. The kind picker is
// the first: two promises, told apart in words rather than by a label. The
// link reveal is the second, and it is the one worth capturing — it is shown
// once, so a regression that hid the caution would be invisible until somebody
// closed the dialog and lost their link.
const shareRoutes: RouteMap = {
  ...meRoute,
  "POST /v1/forecast/shares": () =>
    jsonResponse({
      id: "share-1",
      kind: "live",
      target: "forecast",
      expires_at: "2026-10-03T00:00:00Z",
      token: "shr_9f2c4a1e",
      created_at: "2026-09-03T00:00:00Z",
    }),
};

export const ShareDialogKinds: Story = {
  render: () => (
    <StoryProviders>
      <ShareViewButton
        target="forecast"
        scope={{ kind: "workspace", label: "Whole workspace" }}
        snapshotId="snap-1"
      />
    </StoryProviders>
  ),
  beforeEach: () => installFetchStub(shareRoutes),
  play: clickButton("Share view"),
};

export const ShareDialogLinkShownOnce: Story = {
  render: () => (
    <StoryProviders>
      <ShareViewButton
        target="forecast"
        scope={{ kind: "workspace", label: "Whole workspace" }}
        snapshotId="snap-1"
      />
    </StoryProviders>
  ),
  beforeEach: () => installFetchStub(shareRoutes),
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", { name: "Share view" }),
    );
    // `screen`, not `canvas`, for anything inside the dialog: Modal portals to
    // document.body, so the dialog is a sibling of the canvas rather than a
    // descendant of it, and a canvas-scoped query for its confirm waits out its
    // full budget for a button that is on screen the whole time. The TRIGGER
    // stays canvas-scoped, because that one really is in the story's own tree.
    await userEvent.click(
      await screen.findByRole("button", { name: "Create link" }),
    );
  },
};
