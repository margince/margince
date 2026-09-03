// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
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

// The Analytics screen for the fe-uat render gate. The tabs choose a SECTION
// and a section draws every report it holds, each into the same surface — a
// titled card whose trailing action row carries "Explain this number". So a
// story here is one section's worth of screen, which is exactly the drift these
// stories exist to catch: the screen body had no render coverage while the
// reports grew three different looks.
//
// Every read is stubbed off the same shapes reports.test.tsx exercises. The
// picker lives on the screen, so a story presses it rather than being handed a
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

// The cell the result's own derivation handle is bound to: a report groups by
// its own dimension, and the handle names one group of it. The screen forwards
// every predicate on the handle, so a handle carrying another report's dimension
// would ask for a slice this result never held.
const DERIVATION_CELL: Record<string, [dimension: string, group: string]> = {
  "deals-by-stage": ["stage_id", "pl-s1"],
  forecast: ["forecast_category", "commit"],
  "open-deals-per-company": ["organization_id", "BÄR Pharma GmbH"],
};

function run(report: string, rows: Record<string, unknown>[]) {
  const [dimension, group] = DERIVATION_CELL[report];
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
    derivation_url: `/v1/reports/${report}/derivation?by=${dimension}&agg=sum:amount_minor:raw_minor&${dimension}=${encodeURIComponent(group)}`,
  });
}

const stageRows = [
  {
    stage_id: "pl-s1",
    raw_minor: 24686,
    weighted_minor: 4938,
    deal_count: 2,
    currency: "EUR",
  },
  {
    stage_id: "pl-s2",
    raw_minor: 1850000,
    weighted_minor: 1110000,
    deal_count: 5,
    currency: "EUR",
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

// The source rows behind the commit tile of the forecast, which is the cell the
// handle above is bound to. They add up to that tile's €28,000.00 exactly,
// because a drill-through that does not reconcile to the figure it explains is
// the one thing this card must never show.
const derivation = {
  report: "forecast",
  definition: "Sum of open-deal amounts in the commit category, in EUR",
  plan: {},
  columns: ["deal", "amount"],
  rows: [
    { deal: "BÄR Pharma — Packaging QA", amount: "€7,000.00" },
    { deal: "Brandt — Line QA Retrofit", amount: "€7,000.00" },
    { deal: "Halbach Werke — Filler upgrade", amount: "€7,000.00" },
    { deal: "Ostmann Logistik — Label printer", amount: "€7,000.00" },
  ],
};

// The source rows behind the first stage of the pipeline table — the cell that
// section's own handle is bound to. Same obligation as the forecast one: the
// rows reconcile to the figure the card was opened from.
const stageDerivation = {
  report: "deals-by-stage",
  definition: "Sum of open-deal amounts in the Qualified stage, in EUR",
  plan: {},
  columns: ["deal", "amount"],
  rows: [
    { deal: "BÄR Pharma — Packaging QA", amount: "€146.86" },
    { deal: "Brandt — Line QA Retrofit", amount: "€100.00" },
  ],
};

const routes: RouteMap = {
  "GET /me": meRoute({}),
  "GET /pipelines": () => jsonResponse(pipelines),
  "POST /reports/deals-by-stage": () => run("deals-by-stage", stageRows),
  "POST /reports/forecast": () => run("forecast", forecastRows),
  "POST /reports/open-deals-per-company": () =>
    run("open-deals-per-company", companyRows),
  // The derivation the FORECAST result's handle points at. The unrouted
  // fallback is an empty list page, which carries neither `columns` nor `rows`
  // — both required of a ReportDerivation — so a missing route here reaches the
  // explain card as a 200 the screen has every right to trust.
  "GET /reports/forecast/derivation": () => jsonResponse(derivation),
  // EVERY report whose card a story opens needs its own derivation routed, for
  // the same reason. The pipeline section carries three explain verbs and none
  // of them was routed, so opening one reached the card with a body that had no
  // `rows` at all and the card threw reading their length.
  "GET /reports/deals-by-stage/derivation": () => jsonResponse(stageDerivation),
  "GET /reports/open-deals-per-company/derivation": () =>
    jsonResponse({ ...stageDerivation, report: "open-deals-per-company" }),
};

function screenStory() {
  installFetchStub(routes);
  return (
    <StoryProviders>
      <AnalyticsScreen />
    </StoryProviders>
  );
}

const clickButton =
  (name: string) =>
  async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    await userEvent.click(
      await within(canvasElement).findByRole("button", { name }),
    );
  };

const meta: Meta = { title: "Records/Reports" };
export default meta;

type Story = StoryObj;

// Five money figures read across as one comparison — the strip, under the
// callout that says how to read the second figure in each slot.
export const Forecast: Story = {
  render: screenStory,
  play: clickButton("Forecast"),
};

// The pipeline section, which is BOTH of its reports: the stage table with its
// unweighted and weighted columns, then open deals per company. One story
// because it is one picture — the two cards fit the viewport together, and a
// second story per report would be the same capture under a second name. What
// to check is that the two cards read as one column: the same titled surface,
// each with its own frame caption and its own explain verb.
export const Pipeline: Story = {
  render: screenStory,
  play: clickButton("Pipeline"),
};

// "Explain this number" open, with the report card above it and the derivation
// card below — both the same titled-card surface.
//
// On the PIPELINE section, and reached by opening it first. The Forecast
// section the screen opens on has no explain verb of its own any more: it is
// the period's current call and the readings behind it, which state their own
// basis rather than deriving a figure. Clicking blind on whatever section
// happened to be open is what left this story pressing a button that was no
// longer there.
//
// The FIRST of the section's three verbs. Each report card carries one, so a
// lookup by name alone matches three buttons and resolves to none.
export const Explain: Story = {
  render: screenStory,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", { name: "Pipeline" }),
    );
    const verbs = await canvas.findAllByRole("button", {
      name: "Explain this number",
    });
    await userEvent.click(verbs[0]);
  },
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
      <ShareViewButton target="forecast" snapshotId="snap-1" />
    </StoryProviders>
  ),
  beforeEach: () => installFetchStub(shareRoutes),
  play: clickButton("Share view"),
};

export const ShareDialogLinkShownOnce: Story = {
  render: () => (
    <StoryProviders>
      <ShareViewButton target="forecast" snapshotId="snap-1" />
    </StoryProviders>
  ),
  beforeEach: () => installFetchStub(shareRoutes),
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    await userEvent.click(
      await within(canvasElement).findByRole("button", { name: "Share view" }),
    );
    // The DIALOG is portalled to the body, so it is not under the canvas the
    // trigger is in. Looked for there, the confirm verb simply was not found
    // and the story captured a closed dialog.
    await userEvent.click(
      await within(document.body).findByRole("button", { name: "Create link" }),
    );
  },
};
