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
import { meFixture } from "../app/mefixture";
import { formatMoney, MONEY_ABSENT } from "../format/format";
import { LocaleProvider } from "../i18n";
import { en } from "../i18n/en";

type Stage = components["schemas"]["Stage"];

import {
  AnalyticsScreen,
  buildStageAggregates,
  derivationCellCurrency,
  derivationColumns,
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
  winLossRows?: Record<string, unknown>[];
  stageAgeRows?: Record<string, unknown>[];
  meetingRows?: Record<string, unknown>[];
  phaseRows?: Record<string, unknown>[];
  commitmentRows?: Record<string, unknown>[];
  quietRows?: Record<string, unknown>[];
  // The coverage read: a payload, a status (403 for a seat without the ops
  // grant, 404 for a fresh installation), or omitted for the default 403.
  coverage?: { status: number; body?: unknown };
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
    if (url.endsWith("/me")) {
      return jsonResponse(
        meFixture({
          roles: ["rep"],
          allow: {
            data_coverage:
              opts.coverage !== undefined && opts.coverage.status !== 403
                ? ["read"]
                : [],
          },
        }),
      );
    }
    if (url.includes("/analytics/coverage")) {
      const cov = opts.coverage ?? { status: 403 };
      return jsonResponse(
        cov.body ?? { title: "Forbidden", status: cov.status },
        cov.status,
      );
    }
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
          : key === "activities-by-kind"
            ? (opts.meetingRows ?? [])
            : key === "projects-by-phase"
              ? (opts.phaseRows ?? [])
              : key === "project-commitments"
                ? (opts.commitmentRows ?? [])
                : key === "projects-gone-quiet"
                  ? (opts.quietRows ?? [])
                  : key === "win-loss"
                    ? (opts.winLossRows ?? [])
                    : key === "stage-age"
                      ? (opts.stageAgeRows ?? [])
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

async function openPerformance() {
  await userEvent
    .setup()
    .click(await screen.findByRole("button", { name: "Performance" }));
}

describe("the delivery section", () => {
  it("draws the three project reports with converted money and real links", async () => {
    const bodies: { key: string; body: Record<string, unknown> }[] = [];
    vi.stubGlobal(
      "fetch",
      reportsStub({
        onRun: (key, body) => bodies.push({ key, body }),
        phaseRows: [
          {
            phase: "delivering",
            projects: 3,
            open_deal_value_minor: 400000,
            won_deal_value_minor: 900000,
          },
        ],
        commitmentRows: [
          {
            project_id: "p1",
            name: "Rollout Nord",
            phase: "delivering",
            open_commitments: 5,
            overdue_commitments: 2,
          },
        ],
        quietRows: [],
      }),
    );
    render(<AnalyticsScreen />);
    await userEvent
      .setup()
      .click(await screen.findByRole("button", { name: "Delivery" }));

    // Server-converted money, only formatted here.
    expect(
      await screen.findByText(formatMoney(400000, "EUR", "en")),
    ).toBeTruthy();
    // The project name is the way into the project, not a dead label.
    const link = await screen.findByRole("link", { name: "Rollout Nord" });
    expect(link.getAttribute("href")).toBe("#/projects/p1");
    // Nothing quiet is the good answer, said in words.
    expect(
      screen.getByText("No delivering project has gone quiet."),
    ).toBeTruthy();
    // An empty plan takes the report's own declared defaults server-side.
    const phase = bodies.find((sent) => sent.key === "projects-by-phase");
    expect(phase?.body.group_by).toEqual([]);
    expect(phase?.body.aggregates).toEqual([]);
  });

  it("names a quiet project with the instant it fell silent", async () => {
    vi.stubGlobal(
      "fetch",
      reportsStub({
        phaseRows: [],
        commitmentRows: [],
        quietRows: [
          {
            project_id: "p9",
            name: "Rollout Süd",
            phase: "delivering",
            quiet_since: "2026-08-20T09:00:00Z",
          },
        ],
      }),
    );
    render(<AnalyticsScreen />);
    await userEvent
      .setup()
      .click(await screen.findByRole("button", { name: "Delivery" }));
    const link = await screen.findByRole("link", { name: "Rollout Süd" });
    expect(link.getAttribute("href")).toBe("#/projects/p9");
    // The instant is formatted in the frame's own zone, not left as ISO.
    expect(screen.queryByText("2026-08-20T09:00:00Z")).toBeNull();
    // A young install's other two cards say so in words.
    expect(
      (await screen.findAllByText("No projects yet — a won deal opens one."))
        .length,
    ).toBe(2);
  });
});

describe("the data coverage section", () => {
  it("names each source's state in words, with the instant a read one reached", async () => {
    vi.stubGlobal(
      "fetch",
      reportsStub({
        coverage: {
          status: 200,
          body: {
            run_id: "r1",
            as_of: "2026-09-05T02:00:00Z",
            sources: [
              {
                source: "mail",
                state: "checked",
                checked_through: "2026-09-05T01:30:00Z",
              },
              { source: "offers", state: "not_connected" },
            ],
          },
        },
      }),
    );
    render(<AnalyticsScreen />);
    await userEvent
      .setup()
      .click(await screen.findByRole("button", { name: "Data coverage" }));
    expect(await screen.findByText("Checked")).toBeTruthy();
    // The source column speaks the reader's words, not the wire's.
    expect(screen.getByText("the mailbox")).toBeTruthy();
    // An unconnected source is a decision, not a repair — its words say so.
    expect(
      screen.getByText("Not connected — nothing to fix, something to decide"),
    ).toBeTruthy();
    // Only the read source carries a date; the unread one shows absence.
    expect(screen.getByText("—")).toBeTruthy();
  });

  it("says a fresh installation was never looked at, in words", async () => {
    vi.stubGlobal("fetch", reportsStub({ coverage: { status: 404 } }));
    render(<AnalyticsScreen />);
    await userEvent
      .setup()
      .click(await screen.findByRole("button", { name: "Data coverage" }));
    expect(
      await screen.findByText(
        "No check has run yet. A fresh installation has not been looked at — different from one that was looked at and found healthy.",
      ),
    ).toBeTruthy();
  });

  it("does not request coverage without the ops grant", async () => {
    const fetch = reportsStub({ coverage: { status: 403 } });
    vi.stubGlobal("fetch", fetch);
    render(<AnalyticsScreen />);
    await screen.findByRole("button", { name: "Pipeline" });
    await waitFor(() =>
      expect(
        screen.queryByRole("button", { name: "Data coverage" }),
      ).toBeNull(),
    );
    expect(
      fetch.mock.calls.some(([input]) =>
        String(input instanceof Request ? input.url : input).includes(
          "/analytics/coverage",
        ),
      ),
    ).toBe(false);
  });
});

// A context whose default lens is one seat: the rep's own.
const ownLensContext = {
  default_scope: { kind: "owner", id: "u-rep-1", label: "Riley Rep" },
  allowed_scopes: [{ kind: "owner", id: "u-rep-1", label: "Riley Rep" }],
  capabilities: {
    view_manager_forecast: false,
    submit_manager_forecast: false,
  },
  as_of: "2026-09-04T00:00:00Z",
  timezone: "Europe/Berlin",
  base_currency: "EUR",
};

describe("the my-outcomes section", () => {
  it("shows the tab under an owner lens and answers with the seat's own facts", async () => {
    const bodies: { key: string; body: Record<string, unknown> }[] = [];
    vi.stubGlobal(
      "fetch",
      reportsStub({
        context: ownLensContext,
        onRun: (key, body) => bodies.push({ key, body }),
        stageRows: [{ deal_count: 4, raw_minor: 250000 }],
        meetingRows: [
          { meeting_status: "held", meetings: 3 },
          { meeting_status: "booked", meetings: 2 },
        ],
      }),
    );
    render(<AnalyticsScreen />);
    await userEvent
      .setup()
      .click(await screen.findByRole("button", { name: "My outcomes" }));

    // The meetings card states current standing, not a funnel.
    expect(
      await screen.findByText(
        "Meetings you host, by where each stands today — a held meeting no longer counts as booked.",
      ),
    ).toBeTruthy();
    expect(screen.getByText("Held")).toBeTruthy();
    expect(screen.getByText("3")).toBeTruthy();
    // A status with no meetings is an honest zero, not an absent tile.
    expect(screen.getByText("No-show")).toBeTruthy();

    // The meetings question is pinned to the seat: hosted by this user, and
    // only meetings — the server filters, the browser never sifts rows.
    const meetings = bodies.find((sent) => sent.key === "activities-by-kind");
    expect(meetings?.body.filters).toEqual({
      kind: "meeting",
      host_user_id: "u-rep-1",
    });
    expect(meetings?.body.group_by).toEqual(["meeting_status"]);

    // The pipeline card is pinned to the seat too — the heading says "my",
    // and the request must say it rather than trusting a server default.
    const pipeline = bodies.find(
      (sent) => sent.key === "pipeline-current" && sent.body.filters != null,
    );
    expect(pipeline?.body.filters).toEqual({ owner_id: "u-rep-1" });
    expect(await screen.findByText("4")).toBeTruthy();
  });

  it("explains itself instead of fetching under a wider lens", async () => {
    const bodies: { key: string; body: Record<string, unknown> }[] = [];
    vi.stubGlobal(
      "fetch",
      reportsStub({ onRun: (key, body) => bodies.push({ key, body }) }),
    );
    window.location.hash = "#/analytics/outcomes";
    try {
      render(<AnalyticsScreen />);
      expect(
        await screen.findByText(
          "This view answers for one seat. Your lens covers more than your own records, so the wider sections carry your numbers.",
        ),
      ).toBeTruthy();
      // And it fetched nothing: numbers under this heading would have
      // measured the default population, not the person.
      expect(bodies.some((sent) => sent.key === "activities-by-kind")).toBe(
        false,
      );
    } finally {
      window.location.hash = "";
    }
  });

  it("hides the tab when the lens covers more than one seat", async () => {
    vi.stubGlobal("fetch", reportsStub());
    render(<AnalyticsScreen />);
    await screen.findByRole("button", { name: "Pipeline" });
    expect(screen.queryByRole("button", { name: "My outcomes" })).toBeNull();
  });
});

describe("the performance section", () => {
  it("renders won and lost with converted value and computed durations", async () => {
    vi.stubGlobal(
      "fetch",
      reportsStub({
        winLossRows: [
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
        ],
        stageAgeRows: [
          { stage_id: "pl-s1", deal_count: 6, median_days: 12, p75_days: 30 },
        ],
      }),
    );
    render(<AnalyticsScreen />);
    await openPerformance();

    // Both outcomes, by their words rather than a status key.
    expect(await screen.findByText("Won")).toBeTruthy();
    expect(screen.getByText("Lost")).toBeTruthy();
    // The value arrives converted; the screen only formats it.
    expect(screen.getByText(formatMoney(500000, "EUR", "en"))).toBeTruthy();
    // Durations are the server's medians, never a quotient made here.
    expect(screen.getByText("21 days")).toBeTruthy();
    // A withheld percentile is words, not a zero and not a dash: below the
    // sample floor the engine answers null, and the cell says why.
    expect(screen.getByText("Too few to say")).toBeTruthy();
    // The stage-age card names the stage from the pipeline, not by UUID.
    expect(screen.getByText("Qualify")).toBeTruthy();
    expect(screen.getByText("12 days")).toBeTruthy();
  });

  it("asks the server for the vocabulary it renders, computing nothing", async () => {
    const bodies: { key: string; body: Record<string, unknown> }[] = [];
    vi.stubGlobal(
      "fetch",
      reportsStub({
        onRun: (key, body) => bodies.push({ key, body }),
        winLossRows: [],
        stageAgeRows: [],
      }),
    );
    render(<AnalyticsScreen />);
    await openPerformance();
    await waitFor(() => {
      expect(bodies.some((sent) => sent.key === "win-loss")).toBe(true);
      expect(bodies.some((sent) => sent.key === "stage-age")).toBe(true);
    });
    const winLoss = bodies.find((sent) => sent.key === "win-loss");
    expect(winLoss?.body.aggregates).toEqual([
      { fn: "count", as: "deal_count" },
      { fn: "sum", field: "amount_base_minor", as: "raw_minor" },
      { fn: "median", field: "days_to_close", as: "median_days" },
      { fn: "p75", field: "days_to_close", as: "p75_days" },
    ]);
  });
});

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
    //
    // Matched by SHAPE rather than by the exact field name, because the
    // forecast moved from the native weighted column to the converted one and
    // a test pinned to either spelling asserts which column is fashionable
    // rather than that the figure is asked for at all.
    expect(
      bodies.some(
        (b) =>
          b.key === "forecast" &&
          Array.isArray(b.body.aggregates) &&
          (b.body.aggregates as { field?: string }[]).some((a) =>
            a.field?.startsWith("weighted_"),
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

  // A link minted before the handle carried an instant. The figures were
  // recomputed at a NEW moment, so a rate sheet effective in between makes them
  // disagree with the number they explain — and this is opened by someone
  // checking a figure they already doubt.
  it("says the figures were recalculated when the link pinned no instant", async () => {
    vi.stubGlobal(
      "fetch",
      reportsStub({
        derivation: {
          report: "deals-by-stage",
          definition: "Sum over open deals",
          plan: {},
          columns: ["name"],
          rows: [{ name: "Fleet retrofit" }],
          as_of_pinned: false,
        },
      }),
    );
    render(<AnalyticsScreen />);
    await openPipeline();
    const explains = await screen.findAllByRole("button", { name: /Explain/ });
    await userEvent.click(explains[0]);
    expect(await screen.findByText(en["explain.mayHaveMoved"])).toBeTruthy();
    // The rows are still shown: saying they were recomputed is the fix,
    // withholding them is not.
    expect(screen.getByText("Fleet retrofit")).toBeTruthy();
  });

  // The ordinary case says nothing, because a caveat on every drill-through is
  // a caveat nobody reads.
  it("stays silent when the link pinned the headline's instant", async () => {
    vi.stubGlobal(
      "fetch",
      reportsStub({
        derivation: {
          report: "deals-by-stage",
          definition: "Sum over open deals",
          plan: {},
          columns: ["name"],
          rows: [{ name: "Fleet retrofit" }],
          as_of_pinned: true,
        },
      }),
    );
    render(<AnalyticsScreen />);
    await openPipeline();
    const explains = await screen.findAllByRole("button", { name: /Explain/ });
    await userEvent.click(explains[0]);
    await waitFor(() =>
      expect(screen.getByText("Fleet retrofit")).toBeTruthy(),
    );
    expect(screen.queryByText(en["explain.mayHaveMoved"])).toBeNull();
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
      { fn: "count", field: "amount_base_minor", as: "priced_deals" },
    ]);
  });

  // The money plans that are still NATIVE keep their currency grouping: the
  // company table sums amount_minor, and a total spanning currencies there
  // would be a number with no unit.
  //
  // Every plan is checked, not a named list of them — a list would go on
  // passing after a plan was converted (its entry simply removed) or after one
  // was added summing native money nobody thought to add. The rule is about
  // what a plan ASKS FOR: sum a native minor-unit column and currency must be
  // in the grouping.
  //
  // The scan reads the field NAME, which is as strong as the convention behind
  // it. Every money measure the report engine offers today ends in `_minor`
  // (`amount_minor`, `weighted_amount_minor`) or `_base_minor` (converted),
  // and nothing on the server enforces that — a measure named `amount_cents`
  // would sum across currencies with this test still green. Two things would
  // have to happen for that: somebody adds a money measure AND spells it
  // outside the convention. If you are that person, the fix is #4131's — the
  // server refusing the combination — not a longer suffix list here.
  it("groups every native money plan by currency", async () => {
    const bodies: { key: string; body: Record<string, unknown> }[] = [];
    vi.stubGlobal(
      "fetch",
      reportsStub({ onRun: (key, body) => bodies.push({ key, body }) }),
    );
    render(<AnalyticsScreen />);
    // Every section, so a report added to a NEW section enters this census by
    // construction rather than by somebody remembering to widen the walk.
    await openPipeline();
    await waitFor(() => expect(screen.getByText("Qualify")).toBeTruthy());
    await openPerformance();
    await waitFor(() =>
      expect(bodies.some((sent) => sent.key === "stage-age")).toBe(true),
    );

    expect(bodies.length).toBeGreaterThan(0);
    let nativePlans = 0;
    for (const { key, body } of bodies) {
      const aggregates = (body.aggregates ?? []) as { field?: string }[];
      const sumsNative = aggregates.some(
        (aggregate) =>
          aggregate.field != null &&
          aggregate.field.endsWith("_minor") &&
          !aggregate.field.endsWith("_base_minor"),
      );
      if (!sumsNative) {
        continue;
      }
      nativePlans += 1;
      expect(
        body.group_by,
        `${key} sums a native minor-unit column without grouping by currency`,
      ).toContain("currency");
    }
    // A scan that matched nothing would pass over a screen where every plan
    // had quietly gone native.
    expect(nativePlans).toBeGreaterThan(0);
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

  // priced_deals answers how many of `deal_count` the sums above actually
  // cover — a deal in a currency the rate sheet cannot price is counted but
  // priced nowhere in either total (margince#4201).
  it("carries how many of a stage's deals the money actually covers", () => {
    const [row] = buildStageAggregates(
      [
        {
          stage_id: "pl-s1",
          raw_minor: 100,
          weighted_minor: 90,
          deal_count: 2,
          priced_deals: 1,
        },
      ],
      STAGES,
    );
    expect(row.pricedDeals).toBe(1);
  });

  // A row that omits priced_deals answers "the server did not say", not "zero
  // deals were priced" — folding the two together would print a claim nobody
  // made.
  it("answers null rather than zero when a stage row omits priced_deals", () => {
    const [row] = buildStageAggregates(
      [
        {
          stage_id: "pl-s1",
          raw_minor: 100,
          weighted_minor: 90,
          deal_count: 2,
        },
      ],
      STAGES,
    );
    expect(row.pricedDeals).toBeNull();
  });

  it("shows a stage row's money is short a deal the rate sheet cannot price", async () => {
    vi.stubGlobal(
      "fetch",
      reportsStub({
        stageRows: [
          {
            stage_id: "pl-s1",
            raw_minor: 100,
            weighted_minor: 90,
            deal_count: 2,
            priced_deals: 1,
          },
        ],
      }),
    );
    render(<AnalyticsScreen />);
    await openPipeline();
    await waitFor(() => expect(screen.getByText("Qualify")).toBeTruthy());
    expect(
      screen.getByText((text) => text.includes("1 of 2 priced")),
    ).toBeTruthy();
  });

  it("shows no priced footnote when the row omits priced_deals entirely", async () => {
    vi.stubGlobal(
      "fetch",
      reportsStub({
        stageRows: [
          {
            stage_id: "pl-s1",
            raw_minor: 100,
            weighted_minor: 90,
            deal_count: 2,
          },
        ],
      }),
    );
    render(<AnalyticsScreen />);
    await openPipeline();
    await waitFor(() => expect(screen.getByText("Qualify")).toBeTruthy());
    expect(screen.queryByText((text) => text.includes("priced"))).toBeNull();
  });

  it("shows no priced footnote once every counted deal was priced", async () => {
    vi.stubGlobal(
      "fetch",
      reportsStub({
        stageRows: [
          {
            stage_id: "pl-s1",
            raw_minor: 100,
            weighted_minor: 90,
            deal_count: 2,
            priced_deals: 2,
          },
        ],
      }),
    );
    render(<AnalyticsScreen />);
    await openPipeline();
    await waitFor(() => expect(screen.getByText("Qualify")).toBeTruthy());
    expect(screen.queryByText((text) => text.includes("priced"))).toBeNull();
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
        // Converted rows: one per category, the server having already summed
        // each deal in the base currency. The uncategorised row is the one
        // this test is about — on the demo dataset it was 22 of 27 open deals,
        // and a slot set built from the named enum alone does not move that
        // money to another tile, it drops it off the screen.
        forecastRows: [
          {
            forecast_category: "omitted",
            raw_minor: 58_860_000,
            weighted_minor: 31_502_500,
            deal_count: 5,
          },
          {
            forecast_category: null,
            raw_minor: 202_720_000,
            weighted_minor: 76_460_000,
            deal_count: 17,
          },
        ],
      }),
    );
    render(<AnalyticsScreen />);
    await userEvent.setup().click(await screen.findByText("Pipeline"));

    expect(
      await screen.findByText(formatMoney(202_720_000, "EUR", "en")),
    ).toBeTruthy();
    // And never folded into a named category: the two totals added together
    // is the number that must NOT appear anywhere.
    expect(
      screen.queryByText(formatMoney(261_580_000, "EUR", "en")),
    ).toBeNull();
  });

  // The forecast is ONE comparison now, not one per currency.
  //
  // It drew a band per currency for as long as the report summed native money,
  // and that was honest but defeated the point: a manager comparing commit
  // against best case compared them inside each currency and never across the
  // business. The server converts, so there is one denomination — and the
  // proof is that a currency CODE no longer appears as a heading over the
  // tiles.
  it("draws the categories as one comparison, in the base currency", async () => {
    const bodies: { key: string; body: Record<string, unknown> }[] = [];
    vi.stubGlobal(
      "fetch",
      reportsStub({
        onRun: (key, body) => bodies.push({ key, body }),
        forecastRows: [
          {
            forecast_category: "commit",
            raw_minor: 2_500_000,
            weighted_minor: 250_000,
            deal_count: 1,
          },
          {
            forecast_category: "best_case",
            raw_minor: 920_000,
            weighted_minor: 460_000,
            deal_count: 1,
          },
        ],
      }),
    );
    render(<AnalyticsScreen />);
    await userEvent.setup().click(await screen.findByText("Pipeline"));

    // Both categories, both figures, all in the one base currency.
    expect(
      await screen.findByText(formatMoney(2_500_000, "EUR", "en")),
    ).toBeTruthy();
    expect(screen.getByText(formatMoney(920_000, "EUR", "en"))).toBeTruthy();

    // The plan asks for converted money and does NOT group by currency: those
    // two go together, and a request that changed one without the other would
    // either band a single denomination or sum minor units across several.
    const plan = bodies.find((sent) => sent.key === "forecast");
    expect(plan?.body.group_by).toEqual(["forecast_category"]);
    expect(plan?.body.aggregates).toContainEqual({
      fn: "sum",
      field: "amount_base_minor",
      as: "raw_minor",
    });
  });

  // A converted total silently skips a deal the rate sheet cannot price, so the
  // tile says how many deals it is about as well as what they are worth.
  //
  // The sum has no honest alternative — guessing a rate would invent money —
  // but a tile printing the total alone tells a reader a category is worth
  // €25,000 when it holds two deals and one of them was left out. The count is
  // what makes that visible: two deals, one figure.
  it("says how many deals a category holds, not only what it is worth", async () => {
    vi.stubGlobal(
      "fetch",
      reportsStub({
        forecastRows: [
          {
            forecast_category: "commit",
            // Two deals; one is in a currency the sheet cannot price, so it
            // contributes nothing to the sum and is still counted.
            raw_minor: 2_500_000,
            weighted_minor: 250_000,
            deal_count: 2,
            priced_deals: 1,
          },
        ],
      }),
    );
    render(<AnalyticsScreen />);
    await userEvent.setup().click(await screen.findByText("Pipeline"));

    expect(
      await screen.findByText(formatMoney(2_500_000, "EUR", "en")),
    ).toBeTruthy();
    // The count rides on the tile's second line beside the weighted figure.
    expect(screen.getByText((text) => text.includes("Deals: 2"))).toBeTruthy();
    // The priced count reaches the screen too (margince#4201) — without it
    // the €2,500,000 total reads as covering both deals when it covers one.
    expect(
      screen.getByText((text) => text.includes("1 of 2 priced")),
    ).toBeTruthy();
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

// A drill-through row carries money in two different currencies at once, and
// which one a cell is written in depends on the COLUMN.
//
// `pipeline-current` converts server-side and exposes `amount_base_minor`, in
// the installation's base currency. The forecast does not convert: it exposes
// the deal's own `amount_minor` with the currency it was written in on the
// same row. Formatting both against the base currency puts a euro sign on a
// dollar deal — a wrong number wearing a right-looking symbol, which is the
// misreading the whole renderer exists to prevent.
describe("drill-through money", () => {
  it("writes a converted measure in the base currency", () => {
    const row = { amount_base_minor: 500000, currency: "USD" };
    expect(derivationCellCurrency("amount_base_minor", row, "EUR")).toBe("EUR");
  });

  it("writes an unconverted measure in the deal's own currency", () => {
    const row = { amount_minor: 500000, currency: "USD" };
    expect(derivationCellCurrency("amount_minor", row, "EUR")).toBe("USD");
  });

  // A row that names no currency has nothing to write the figure in. Falling
  // back to the base currency would be a guess presented as a fact.
  it("names no currency for an unconverted measure on a row without one", () => {
    expect(derivationCellCurrency("amount_minor", {}, "EUR")).toBeNull();
    expect(
      derivationCellCurrency("amount_minor", { currency: "" }, "EUR"),
    ).toBeNull();
  });
});

// The id is noise beside a name — but only when every row HAS one. Labelling
// is per row, so a reader who may not read one record gets a label column
// with a gap in it.
describe("drill-through columns", () => {
  const derivation = (columns: string[], rows: Record<string, unknown>[]) =>
    ({ columns, rows }) as unknown as Parameters<typeof derivationColumns>[0];

  it("drops the id once every row is named", () => {
    expect(
      derivationColumns(
        derivation(
          ["id", "label", "amount_minor"],
          [
            { id: "a", label: "Acme" },
            { id: "b", label: "Globex" },
          ],
        ),
      ),
    ).toEqual(["label", "amount_minor"]);
  });

  // The row whose name was withheld is the one a reader can least account
  // for. Dropping the id here would leave it showing a blank and nothing else.
  it("keeps the id when any row's name was withheld", () => {
    expect(
      derivationColumns(
        derivation(
          ["id", "label", "amount_minor"],
          [{ id: "a", label: "Acme" }, { id: "b" }],
        ),
      ),
    ).toEqual(["id", "label", "amount_minor"]);
  });

  it("keeps the id when no row could be named", () => {
    expect(
      derivationColumns(derivation(["id", "amount_minor"], [{ id: "a" }])),
    ).toEqual(["id", "amount_minor"]);
  });
});
