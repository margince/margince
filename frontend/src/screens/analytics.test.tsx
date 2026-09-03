/** @vitest-environment jsdom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render as rtlRender,
  screen,
  waitFor,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { formatMoney, MONEY_ABSENT } from "../format/format";
import { LocaleProvider } from "../i18n";

type Stage = components["schemas"]["Stage"];

import {
  AnalyticsScreen,
  buildStageAggregates,
  parseDerivationQuery,
  sectionFromAddress,
} from "./analytics";

// D2 acceptance: a report picker over deals-by-stage (unchanged), forecast
// (unweighted category tiles + a weighted-vs-unweighted banner), and
// open-deals-per-company (a DataTable) — all driven by the same typed
// `runReport` POST, keyed on the report.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

const render = (ui: ReactNode) => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">{ui}</LocaleProvider>
    </QueryClientProvider>,
  );
};

type ReportsStubOpts = {
  onRun?: (key: string, body: Record<string, unknown>) => void;
  // Model a server that sends only PART of the frame — an installation
  // mid-upgrade, which is the one place a partial result actually arrives.
  // Dropping the whole frame would be a weaker fixture: a guard that checks
  // only one of the three fields passes against it, and the caption then
  // renders with an undefined zone.
  partialFrame?: boolean;
  stageRows?: Record<string, unknown>[];
  forecastRows?: Record<string, unknown>[];
  companyRows?: Record<string, unknown>[];
  derivation?: Record<string, unknown>;
  onDerivation?: (url: string) => void;
};

function reportsStub(opts: ReportsStubOpts = {}) {
  return vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const request = input instanceof Request ? input : null;
    const url = String(request ? request.url : input);
    const method = request ? request.method : (init?.method ?? "GET");
    if (method === "GET" && url.includes("/derivation")) {
      opts.onDerivation?.(url);
      return jsonResponse(opts.derivation ?? {});
    }
    if (url.includes("/pipelines")) {
      return jsonResponse({
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
            ],
          },
        ],
        page: { next_cursor: null },
      });
    }
    if (method === "POST" && url.includes("/reports/")) {
      const match = url.match(/\/reports\/([^/?]+)/);
      const key = match ? match[1] : "";
      const body = request
        ? await request.json()
        : JSON.parse(String(init?.body));
      opts.onRun?.(key, body);
      const rows =
        key === "forecast"
          ? (opts.forecastRows ?? [])
          : key === "open-deals-per-company"
            ? (opts.companyRows ?? [])
            : (opts.stageRows ?? [
                {
                  stage_id: "pl-s1",
                  raw_minor: 100000,
                  deal_count: 2,
                  currency: "EUR",
                },
              ]);
      return jsonResponse({
        report: key,
        plan: {},
        columns: [],
        rows,
        // The frame the server sends with every result. A fixture that omits
        // it models a response no live server produces, and the screen would
        // then be tested against a shape it never meets.
        as_of: "2026-03-04T09:00:00Z",
        ...(opts.partialFrame
          ? {}
          : {
              timezone: "Europe/Berlin",
              base_currency: "EUR",
              fiscal_year_start_month: 1,
            }),
        derivation_url: `/v1/reports/${key}/derivation?by=stage_id&agg=sum:amount_minor:raw_minor&stage_id=pl-s1`,
      });
    }
    return jsonResponse({ data: [], page: { next_cursor: null } });
  });
}

// The pipeline reports live behind their own tab now, and Forecast is the
// section a reader lands on. A test that wants deals-by-stage opens the tab
// the way a reader does, rather than asserting against a default that moved.
async function openPipeline() {
  await userEvent
    .setup()
    .click(await screen.findByRole("button", { name: "Pipeline" }));
}

describe("AnalyticsScreen", () => {
  it("renders unweighted/weighted columns under Pipeline", async () => {
    vi.stubGlobal("fetch", reportsStub());
    render(<AnalyticsScreen />);
    await openPipeline();
    await waitFor(() => expect(screen.getByText("Qualify")).toBeTruthy());
  });

  it("switching to Forecast groups by forecast_category and renders category tiles", async () => {
    const bodies: { key: string; body: Record<string, unknown> }[] = [];
    vi.stubGlobal(
      "fetch",
      reportsStub({
        onRun: (key, body) => bodies.push({ key, body }),
        forecastRows: [
          {
            forecast_category: "commit",
            raw_minor: 500000,
            weighted_minor: 300000,
            deal_count: 3,
            currency: "EUR",
          },
        ],
      }),
    );
    render(<AnalyticsScreen />);
    await userEvent.click(
      await screen.findByRole("button", { name: "Forecast" }),
    );
    await waitFor(() => expect(screen.getByText("Commit")).toBeTruthy());
    expect(
      bodies.some(
        (b) =>
          b.key === "forecast" &&
          Array.isArray(b.body.group_by) &&
          b.body.group_by.includes("forecast_category"),
      ),
    ).toBe(true);
    // AC-F1: the weighted forecast is the server's own figure —
    // requested and rendered, not left computed-and-unshown.
    expect(
      bodies.some(
        (b) =>
          b.key === "forecast" &&
          Array.isArray(b.body.aggregates) &&
          (b.body.aggregates as { field?: string }[]).some(
            (a) => a.field === "weighted_amount_minor",
          ),
      ),
    ).toBe(true);
  });

  it("renders the slipped bucket in the Forecast tiles", async () => {
    vi.stubGlobal(
      "fetch",
      reportsStub({
        forecastRows: [
          {
            forecast_category: "slipped",
            raw_minor: 90000,
            weighted_minor: 45000,
            deal_count: 1,
            currency: "EUR",
          },
        ],
      }),
    );
    render(<AnalyticsScreen />);
    await userEvent.click(
      await screen.findByRole("button", { name: "Forecast" }),
    );
    await waitFor(() => expect(screen.getByText("Slipped")).toBeTruthy());
  });

  it("deals-by-stage requests and renders the server's weighted_minor, not a client re-derivation", async () => {
    const bodies: { key: string; body: Record<string, unknown> }[] = [];
    vi.stubGlobal(
      "fetch",
      reportsStub({
        onRun: (key, body) => bodies.push({ key, body }),
        // 12343 × 20% per deal (2469 × 2 = 4938) — a figure a client can only
        // reproduce by rounding round(24686 × 20%) = 4937, the wrong way.
        stageRows: [
          {
            stage_id: "pl-s1",
            raw_minor: 24686,
            weighted_minor: 4938,
            deal_count: 2,
            currency: "EUR",
          },
        ],
      }),
    );
    render(<AnalyticsScreen />);
    await openPipeline();
    await waitFor(() => expect(screen.getByText("Qualify")).toBeTruthy());
    expect(
      bodies.some(
        (b) =>
          b.key === "deals-by-stage" &&
          Array.isArray(b.body.aggregates) &&
          (b.body.aggregates as { field?: string }[]).some(
            (a) => a.field === "weighted_amount_minor",
          ),
      ),
    ).toBe(true);
    expect(await screen.findByText("€49.38")).toBeTruthy();
    expect(screen.queryByText("€49.37")).toBeNull();
  });

  it("switching to Open deals per company groups by organization_id and renders a table", async () => {
    vi.stubGlobal(
      "fetch",
      reportsStub({
        companyRows: [
          {
            organization_id: "o1",
            raw_minor: 250000,
            deal_count: 4,
            currency: "EUR",
          },
        ],
      }),
    );
    render(<AnalyticsScreen />);
    await userEvent.click(
      await screen.findByRole("button", { name: "Pipeline" }),
    );
    await waitFor(() => expect(screen.getByText("o1")).toBeTruthy());
  });

  it("explain fetches the derivation and renders source rows, not raw JSON", async () => {
    const derivationUrls: string[] = [];
    vi.stubGlobal(
      "fetch",
      reportsStub({
        onDerivation: (u) => derivationUrls.push(u),
        derivation: {
          report: "deals-by-stage",
          definition: "Sum over open deals",
          plan: {},
          columns: ["name"],
          rows: [{ name: "Fleet retrofit" }],
        },
      }),
    );
    render(<AnalyticsScreen />);
    await userEvent.click(
      await screen.findByRole("button", { name: /Explain/ }),
    );
    await waitFor(() =>
      expect(screen.getByText("Fleet retrofit")).toBeTruthy(),
    );
    expect(screen.queryByText(/"plan":/)).toBeNull();
    // The equality predicate from derivation_url must survive to the request —
    // by/agg alone would explain the wrong slice.
    expect(derivationUrls[0]).toContain("stage_id=pl-s1");
    expect(derivationUrls[0]).toContain("by=stage_id");
  });
});

describe("parseDerivationQuery", () => {
  it("pulls by/agg + predicate params from a derivation_url", () => {
    const q = parseDerivationQuery(
      "/v1/reports/deals-by-stage/derivation?by=stage_id&agg=sum:amount_minor:raw&stage_id=s1",
    );
    expect(q.by).toEqual(["stage_id"]);
    expect(q.agg).toEqual(["sum:amount_minor:raw"]);
    expect(q.stage_id).toBe("s1");
  });
});

// Money never sums across currencies (data-semantics §1 r4, AC-DS-FX1): every
// plan this screen runs groups by currency, so a stage or a category holding
// two currencies has two totals and the screen shows both. What it must never
// show is one figure that added them, or a currency it invented for a figure
// the server sent without one.
describe("reports never sum money across currencies", () => {
  // Deliberately out of pipeline order, so the ordering assertions below prove a
  // sort rather than an echo of the fixture.
  const stage = (id: string, name: string, position: number): Stage => ({
    id,
    pipeline_id: "pl",
    name,
    position,
    semantic: "open",
    win_probability: 20,
  });
  const STAGES: Stage[] = [
    stage("pl-s2", "Propose", 2),
    stage("pl-s1", "Qualify", 1),
  ];

  it("asks the server to group every money plan by currency", async () => {
    const bodies: { key: string; body: Record<string, unknown> }[] = [];
    vi.stubGlobal(
      "fetch",
      reportsStub({ onRun: (key, body) => bodies.push({ key, body }) }),
    );
    render(<AnalyticsScreen />);
    await openPipeline();
    await waitFor(() => expect(screen.getByText("Qualify")).toBeTruthy());
    const stagePlan = bodies.find((sent) => sent.key === "deals-by-stage");
    expect(stagePlan?.body).toMatchObject({
      group_by: ["stage_id", "currency"],
    });
  });

  it("renders a stage's two currencies as two rows and no combined figure", async () => {
    vi.stubGlobal(
      "fetch",
      reportsStub({
        stageRows: [
          {
            stage_id: "pl-s1",
            raw_minor: 100_000,
            weighted_minor: 20_000,
            deal_count: 2,
            currency: "EUR",
          },
          {
            stage_id: "pl-s1",
            raw_minor: 4_500_000_000,
            weighted_minor: 900_000_000,
            deal_count: 3,
            currency: "VND",
          },
        ],
      }),
    );
    render(<AnalyticsScreen />);
    await openPipeline();
    expect(
      await screen.findByText(formatMoney(100_000, "EUR", "en")),
    ).toBeTruthy();
    expect(
      screen.getByText(formatMoney(4_500_000_000, "VND", "en")),
    ).toBeTruthy();
    expect(screen.getAllByText("EUR").length).toBeGreaterThan(0);
    expect(screen.getAllByText("VND").length).toBeGreaterThan(0);
    // The figure the old ungrouped plan printed: both currencies added and
    // labelled with whichever one happened to arrive first.
    expect(
      screen.queryByText(formatMoney(4_500_100_000, "EUR", "en")),
    ).toBeNull();
  });

  it("renders an unpriced stage as absent rather than as zero euros", async () => {
    vi.stubGlobal(
      "fetch",
      reportsStub({
        stageRows: [
          {
            stage_id: "pl-s1",
            raw_minor: null,
            weighted_minor: null,
            deal_count: 4,
            currency: null,
          },
        ],
      }),
    );
    render(<AnalyticsScreen />);
    await openPipeline();
    await waitFor(() => expect(screen.getByText("Qualify")).toBeTruthy());
    // The count is real and stays; only the money is unknown.
    expect(screen.getByText("4")).toBeTruthy();
    expect(screen.getAllByText(MONEY_ABSENT).length).toBeGreaterThan(0);
    expect(screen.queryByText(formatMoney(0, "EUR", "en"))).toBeNull();
  });

  it("renders a forecast category with no deals as absent rather than as zero euros", async () => {
    vi.stubGlobal("fetch", reportsStub({ forecastRows: [] }));
    render(<AnalyticsScreen />);
    await userEvent.setup().click(await screen.findByText("Forecast"));
    await waitFor(() =>
      expect(screen.getAllByText(MONEY_ABSENT).length).toBeGreaterThan(0),
    );
    expect(screen.queryByText(formatMoney(0, "EUR", "en"))).toBeNull();
  });

  it("orders stage rows down the pipeline, then by currency code", () => {
    const rows = [
      { stage_id: "pl-s2", currency: "VND", raw_minor: 1, deal_count: 1 },
      { stage_id: "pl-s1", currency: "VND", raw_minor: 1, deal_count: 1 },
      { stage_id: "pl-s2", currency: "EUR", raw_minor: 1, deal_count: 1 },
      { stage_id: "pl-s1", currency: "EUR", raw_minor: 1, deal_count: 1 },
    ];
    expect(
      buildStageAggregates(rows, STAGES).map(
        (row) => `${row.stageName}/${row.currency}`,
      ),
    ).toEqual(["Qualify/EUR", "Qualify/VND", "Propose/EUR", "Propose/VND"]);
  });

  it("sorts a row whose stage the pipeline no longer carries to the end", () => {
    const rows = [
      { stage_id: "gone", currency: "EUR", raw_minor: 1, deal_count: 1 },
      { stage_id: "pl-s1", currency: "EUR", raw_minor: 1, deal_count: 1 },
    ];
    expect(
      buildStageAggregates(rows, STAGES).map((row) => row.stageId),
    ).toEqual(["pl-s1", "gone"]);
  });

  it("keeps an absent measure absent rather than reading it as zero", () => {
    const [row] = buildStageAggregates(
      [
        {
          stage_id: "pl-s1",
          currency: null,
          raw_minor: null,
          weighted_minor: null,
          deal_count: 3,
        },
      ],
      STAGES,
    );
    expect(row.rawMinor).toBeNull();
    expect(row.weightedMinor).toBeNull();
    expect(row.currency).toBeNull();
    // A count of zero would be a claim; the server did send this one.
    expect(row.count).toBe(3);
  });

  // The wire allows a deal to carry no forecast category — nobody has said which
  // way it is going — and the five named categories match none of it. A tile set
  // built from the enum alone drops those deals off the screen entirely: the
  // money is not moved to another tile, it leaves. On the demo dataset that was
  // 22 of 27 open deals.
  it("shows the deals that carry no forecast category at all", async () => {
    vi.stubGlobal(
      "fetch",
      reportsStub({
        forecastRows: [
          {
            forecast_category: "omitted",
            currency: "EUR",
            raw_minor: 58_860_000,
            weighted_minor: 31_502_500,
            deal_count: 5,
          },
          {
            forecast_category: null,
            currency: "EUR",
            raw_minor: 202_720_000,
            weighted_minor: 76_460_000,
            deal_count: 17,
          },
          {
            forecast_category: null,
            currency: "VND",
            raw_minor: 262_000_000_000,
            weighted_minor: 185_500_000_000,
            deal_count: 2,
          },
        ],
      }),
    );
    render(<AnalyticsScreen />);
    await userEvent.setup().click(await screen.findByText("Forecast"));
    // Both currencies of the uncategorised pipeline, each in its own unit.
    expect(
      await screen.findByText(formatMoney(202_720_000, "EUR", "en")),
    ).toBeTruthy();
    expect(
      screen.getByText(formatMoney(262_000_000_000, "VND", "en")),
    ).toBeTruthy();
    // And never folded into one of the named categories.
    expect(
      screen.queryByText(formatMoney(261_580_000, "EUR", "en")),
    ).toBeNull();
  });

  // An installation whose deals are all categorised should not be shown an empty
  // sixth tile asking about a state it never reaches.
  it("draws no uncategorised tile when every deal has a category", async () => {
    vi.stubGlobal(
      "fetch",
      reportsStub({
        forecastRows: [
          {
            forecast_category: "commit",
            currency: "EUR",
            raw_minor: 1000,
            weighted_minor: 900,
            deal_count: 1,
          },
        ],
      }),
    );
    render(<AnalyticsScreen />);
    await userEvent.setup().click(await screen.findByText("Forecast"));
    await waitFor(() =>
      expect(screen.getByText(formatMoney(1000, "EUR", "en"))).toBeTruthy(),
    );
    expect(screen.queryByText("No category yet")).toBeNull();
  });
});

// The address names a SECTION, and a section may hold several reports. The old
// addresses named a REPORT — those links are in bookmarks and in sent mail, so
// each one still has to answer.
describe("sectionFromAddress", () => {
  it("takes a section straight from the address", () => {
    expect(sectionFromAddress("pipeline")).toBe("pipeline");
    expect(sectionFromAddress("forecast")).toBe("forecast");
  });

  // #/analytics/deals-by-stage was a real address for as long as the picker
  // existed. Answering it with the default section would drop the reader on a
  // page that is not the one they saved.
  it("answers an old report address with the section that now holds it", () => {
    expect(sectionFromAddress("deals-by-stage")).toBe("pipeline");
    expect(sectionFromAddress("open-deals-per-company")).toBe("pipeline");
  });

  // A segment is whatever a reader typed. Anything unrecognized lands on the
  // first section rather than rendering an empty screen.
  it("falls back to the first section for anything it does not know", () => {
    expect(sectionFromAddress(undefined)).toBe("forecast");
    expect(sectionFromAddress("nonsense")).toBe("forecast");
  });
});

// Pipeline draws deals-by-stage and open-deals-per-company, so its captions
// come in pairs. Named rather than written as a bare 2, so a third report
// added to the section reads as a deliberate change here.
const SECTION_REPORT_COUNT_PIPELINE = 2;

describe("the report frame", () => {
  // A total with no zone and no currency beside it is a number the reader
  // places by assumption, and the assumption is their own zone.
  it("names the instant, the zone and the currency the figures were cut in", async () => {
    vi.stubGlobal("fetch", reportsStub());
    render(<AnalyticsScreen />);
    await openPipeline();
    // One per report, not one per screen: each result carries its own frame,
    // and a section holding two reports could be showing two results computed
    // moments apart. A single caption over both would claim they share an
    // instant they do not.
    const captions = await screen.findAllByText(/Europe\/Berlin/);
    expect(captions).toHaveLength(SECTION_REPORT_COUNT_PIPELINE);
    expect(captions[0].textContent).toContain("EUR");
  });

  // A server mid-upgrade sends a partial frame. Naming two of the three would
  // be worse than naming none, so the caption is drawn or it is not.
  it("draws no caption at all when the server sent only part of the frame", async () => {
    vi.stubGlobal("fetch", reportsStub({ partialFrame: true }));
    render(<AnalyticsScreen />);
    await openPipeline();
    // The REPORT still renders. That is the half that makes this test mean
    // something: a guard checking only `as_of` also produces no caption here,
    // but by throwing inside formatDateTime — which takes the whole card down
    // and leaves the reader with no figures at all. Asserting the absence of
    // the caption alone would pass against that.
    await waitFor(() => expect(screen.getByText("Qualify")).toBeTruthy());
    expect(screen.queryByText(/Europe\/Berlin/)).toBeNull();
    expect(screen.queryByText(/As of/)).toBeNull();
  });
});
