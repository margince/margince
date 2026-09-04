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
  context?: Record<string, unknown>;
};

function reportsStub(opts: ReportsStubOpts = {}) {
  return vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const request = input instanceof Request ? input : null;
    const url = String(request ? request.url : input);
    const method = request ? request.method : (init?.method ?? "GET");
    // Every Analytics surface reads its frame first: which population these
    // numbers cover, and whether this reader may publish a forecast. A stub
    // without it leaves the screen waiting and the assertions below looking
    // like a rendering bug.
    if (url.includes("/analytics/context")) {
      return jsonResponse(
        opts.context ?? {
          default_scope: { kind: "workspace", label: "Whole workspace" },
          allowed_scopes: [{ kind: "workspace", label: "Whole workspace" }],
          capabilities: {
            view_manager_forecast: true,
            submit_manager_forecast: true,
          },
          as_of: "2026-09-04T00:00:00Z",
          timezone: "Europe/Berlin",
          base_currency: "EUR",
        },
      );
    }
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
      await screen.findByRole("button", { name: "Pipeline" }),
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
      await screen.findByRole("button", { name: "Pipeline" }),
    );
    await waitFor(() => expect(screen.getByText("Slipped")).toBeTruthy());
  });

  it("renders the server's weighted figure, never a client re-derivation", async () => {
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
          b.key === "pipeline-current" &&
          Array.isArray(b.body.aggregates) &&
          (b.body.aggregates as { field?: string }[]).some(
            (a) => a.field === "weighted_base_minor",
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

  // A count is a promise about a set, and the link beside it is that promise
  // kept. `/deals` reads no `currency` dial, so a row grouped by currency can
  // only be addressed when its key has no second currency row to be confused
  // with — these two hold both directions of that, because a gate that only
  // proves the link appears would pass a version that always draws it.
  describe("a count opens exactly the deals it counted", () => {
    const openPipelineTab = async () =>
      userEvent.click(await screen.findByRole("button", { name: "Pipeline" }));

    it("addresses a company trading in one currency, and says the deals are open", async () => {
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
      await openPipelineTab();
      const door = await screen.findByRole("link", { name: "4" });
      expect(door.getAttribute("href")).toContain("organization_id=o1");
      expect(door.getAttribute("href")).toContain("status=open");
    });

    it("draws the count plainly when the company has two currency rows", async () => {
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
            {
              organization_id: "o1",
              raw_minor: 900000,
              deal_count: 3,
              currency: "VND",
            },
          ],
        }),
      );
      render(<AnalyticsScreen />);
      await openPipelineTab();
      // Both figures are on screen; neither is a door, because a link narrowed
      // to the company alone would open all seven deals beside a figure that
      // counted four.
      await waitFor(() => expect(screen.getByText("4")).toBeTruthy());
      expect(screen.getByText("3")).toBeTruthy();
      expect(screen.queryByRole("link", { name: "4" })).toBeNull();
      expect(screen.queryByRole("link", { name: "3" })).toBeNull();
    });
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
    await openPipeline();
    // The FIRST card's explain. Pipeline draws three reports and each carries
    // its own control, so a query that matched one of them would have been
    // matching whichever happened to render first.
    const explains = await screen.findAllByRole("button", { name: /Explain/ });
    await userEvent.click(explains[0]);
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

  it("asks the server for the CONVERTED measures, grouped by stage alone", async () => {
    const bodies: { key: string; body: Record<string, unknown> }[] = [];
    vi.stubGlobal(
      "fetch",
      reportsStub({ onRun: (key, body) => bodies.push({ key, body }) }),
    );
    render(<AnalyticsScreen />);
    await openPipeline();
    await waitFor(() => expect(screen.getByText("Qualify")).toBeTruthy());
    const stagePlan = bodies.find((sent) => sent.key === "pipeline-current");
    // No currency in the grouping: the server converted each deal before
    // summing, so a stage is one row rather than one per currency it trades in.
    expect(stagePlan?.body).toMatchObject({ group_by: ["stage_id"] });
    expect(stagePlan?.body.aggregates).toEqual([
      { fn: "sum", field: "amount_base_minor", as: "raw_minor" },
      { fn: "sum", field: "weighted_base_minor", as: "weighted_minor" },
      { fn: "count", as: "deal_count" },
    ]);
  });

  // The money plans that are still NATIVE keep their currency grouping: the
  // company table and the forecast strip sum amount_minor, and a total spanning
  // currencies there would be a number with no unit.
  it("still groups the native money plans by currency", async () => {
    const bodies: { key: string; body: Record<string, unknown> }[] = [];
    vi.stubGlobal(
      "fetch",
      reportsStub({ onRun: (key, body) => bodies.push({ key, body }) }),
    );
    render(<AnalyticsScreen />);
    await openPipeline();
    await waitFor(() => expect(screen.getByText("Qualify")).toBeTruthy());
    for (const key of ["forecast", "open-deals-per-company"]) {
      const plan = bodies.find((sent) => sent.key === key);
      expect(plan?.body.group_by).toContain("currency");
    }
  });

  it("draws one row per stage, in the installation's base currency", async () => {
    vi.stubGlobal(
      "fetch",
      reportsStub({
        // What the converted report returns: one row for the stage, its money
        // already added up across the currencies the deals were written in.
        stageRows: [
          {
            stage_id: "pl-s1",
            raw_minor: 250_000,
            weighted_minor: 50_000,
            deal_count: 5,
          },
        ],
      }),
    );
    render(<AnalyticsScreen />);
    await openPipeline();

    expect(
      await screen.findByText(formatMoney(250_000, "EUR", "en")),
    ).toBeTruthy();
    // One stage, one row: the count cell appears once.
    expect(screen.getAllByText("Qualify")).toHaveLength(1);
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
    await userEvent.setup().click(await screen.findByText("Pipeline"));
    await waitFor(() =>
      expect(screen.getAllByText(MONEY_ABSENT).length).toBeGreaterThan(0),
    );
    expect(screen.queryByText(formatMoney(0, "EUR", "en"))).toBeNull();
  });

  // One stage is ONE row now: the server converts each deal before summing, so
  // there is no per-currency split left to order within a stage.
  it("orders stage rows down the pipeline", () => {
    const rows = [
      { stage_id: "pl-s2", raw_minor: 1, deal_count: 1 },
      { stage_id: "pl-s1", raw_minor: 1, deal_count: 1 },
    ];
    expect(
      buildStageAggregates(rows, STAGES).map((row) => row.stageName),
    ).toEqual(["Qualify", "Propose"]);
  });

  it("sorts a row whose stage the pipeline no longer carries to the end", () => {
    const rows = [
      { stage_id: "gone", raw_minor: 1, deal_count: 1 },
      { stage_id: "pl-s1", raw_minor: 1, deal_count: 1 },
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
          raw_minor: null,
          weighted_minor: null,
          deal_count: 3,
        },
      ],
      STAGES,
    );
    expect(row.rawMinor).toBeNull();
    expect(row.weightedMinor).toBeNull();
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
    await userEvent.setup().click(await screen.findByText("Pipeline"));
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
    await userEvent.setup().click(await screen.findByText("Pipeline"));
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
const SECTION_REPORT_COUNT_PIPELINE = 3;

describe("the report frame", () => {
  // A total with no zone beside it is a number the reader places by
  // assumption, and the assumption is their own zone.
  it("names the instant and the zone the figures were cut in", async () => {
    vi.stubGlobal("fetch", reportsStub());
    render(<AnalyticsScreen />);
    await openPipeline();
    // One per report, not one per screen: each result carries its own frame,
    // and a section holding two reports could be showing two results computed
    // moments apart. A single caption over both would claim they share an
    // instant they do not.
    const captions = await screen.findAllByText(/Europe\/Berlin/);
    expect(captions).toHaveLength(SECTION_REPORT_COUNT_PIPELINE);
  });

  // And it names NO currency. Every report on this tab is denominated per
  // currency — the stage table prints a row per stage per currency — so a
  // code in the frame reads as the denomination of numbers that were never
  // converted into it. The figures on screen here are EUR and USD at once,
  // and a reader taking the old caption at its word read the USD total as
  // euros.
  it("claims no currency in the frame, because the blocks under it differ", async () => {
    vi.stubGlobal("fetch", reportsStub());
    render(<AnalyticsScreen />);
    await openPipeline();

    // The frame sits under ALL THREE blocks, and they are not in one currency:
    // the stage table is converted into the base currency, while the forecast
    // strip and the company table are still per-currency native sums. A code in
    // the frame would be the denomination of only one of the three.
    const captions = await screen.findAllByText(/Europe\/Berlin/);
    for (const caption of captions) {
      expect(caption.textContent).not.toMatch(/\b(EUR|USD|VND)\b/);
    }
  });

  // The converted table says its own currency, in the line under its title,
  // where the statement is true of every figure beneath it.
  it("names the base currency on the converted stage table", async () => {
    vi.stubGlobal("fetch", reportsStub());
    render(<AnalyticsScreen />);
    await openPipeline();

    expect(await screen.findByText(/each converted into EUR/)).toBeTruthy();
  });

  // A server mid-upgrade sends a partial frame. Naming one of the two would be
  // worse than naming none, so the caption is drawn or it is not.
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
