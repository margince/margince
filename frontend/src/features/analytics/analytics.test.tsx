/** @vitest-environment happy-dom */
import { cleanup, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { formatMoney } from "../../format/format";
import { en } from "../../i18n/en";
import { SECTIONS, sectionFromAddress } from "../../lib/analytics.address";
import { AnalyticsScreen } from "./analytics";
import { analyticsTabs } from "./analytics.tabs";
import {
  openPipeline,
  ownLensContext,
  render,
  reportsStub,
  stageTable,
} from "./analytics.testkit";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

it("draws the tabs in the row's own order, whatever order the sections are listed in", () => {
  expect(analyticsTabs(SECTIONS)).toEqual([
    "overview",
    "performance",
    "sdr",
    "leads",
    "partners",
    "forecast",
    "pipeline",
    "outcomes",
    "delivery",
  ]);
  expect(analyticsTabs([...SECTIONS].reverse())).toEqual(
    analyticsTabs(SECTIONS),
  );
});

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

    expect(
      await screen.findByText(formatMoney(400000, "EUR", "en")),
    ).toBeTruthy();
    const link = await screen.findByRole("link", { name: "Rollout Nord" });
    expect(link.getAttribute("href")).toBe("/#/projects/p1");
    expect(
      screen.getByText("No project in delivery has gone quiet."),
    ).toBeTruthy();
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
    expect(link.getAttribute("href")).toBe("/#/projects/p9");
    expect(screen.queryByText("2026-08-20T09:00:00Z")).toBeNull();
    expect(
      (
        await screen.findAllByText(
          "No projects yet. Create one under Projects with New project. A won deal moves its project to Delivering.",
        )
      ).length,
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
      .click(await screen.findByRole("button", { name: "Setup" }));
    await userEvent
      .setup()
      .click(await screen.findByRole("button", { name: "Data coverage" }));
    expect(await screen.findByText("Checked")).toBeTruthy();
    expect(screen.getByText("mailbox")).toBeTruthy();
    expect(screen.getByText("Not connected")).toBeTruthy();
    expect(screen.getByText("—")).toBeTruthy();
  });

  it("says a fresh installation was never looked at, in words", async () => {
    vi.stubGlobal("fetch", reportsStub({ coverage: { status: 404 } }));
    render(<AnalyticsScreen />);
    await userEvent
      .setup()
      .click(await screen.findByRole("button", { name: "Setup" }));
    await userEvent
      .setup()
      .click(await screen.findByRole("button", { name: "Data coverage" }));
    expect(
      await screen.findByText(
        "No check has run yet. An unchecked installation is not the same as a healthy one.",
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

    expect(
      await screen.findByText(en["analytics.meetingsAsTheyStand"]),
    ).toBeTruthy();
    expect(screen.getByText("Held")).toBeTruthy();
    expect(screen.getByText("3")).toBeTruthy();
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
        await screen.findByText(en["analytics.outcomesOwnLensOnly"]),
      ).toBeTruthy();
      // And it fetched nothing: numbers under this heading would have
      // measured the default population, not the contact.
      expect(bodies.some((sent) => sent.key === "activities-by-kind")).toBe(
        false,
      );
    } finally {
      window.location.hash = "";
    }
  });

  // A reading with no way out. Both figures are one row of the pipeline report,
  // and the section that draws that report is on this very screen — a rep who
  // wanted the deals behind "4" had to find the tab themselves.
  it("opens the pipeline section from each of the seat's readings", async () => {
    vi.stubGlobal(
      "fetch",
      reportsStub({
        context: ownLensContext,
        stageRows: [{ deal_count: 4, raw_minor: 250000 }],
      }),
    );
    const user = userEvent.setup();
    try {
      render(<AnalyticsScreen />);
      await user.click(
        await screen.findByRole("button", { name: "My outcomes" }),
      );
      await screen.findByText("4");

      // TWO doors, and every door in the product carries the same word — so
      // what tells them apart for a screen reader is each one's own reading.
      // The meetings cards below read from a different report and offer none.
      const doors = screen.getAllByRole("button", { name: "Open" });
      expect(doors).toHaveLength(2);
      expect([
        screen.getByRole("button", { name: "Open", description: "Open deals" }),
        screen.getByRole("button", {
          name: "Open",
          description: "Deal value · EUR",
        }),
      ]).toEqual(doors);

      await user.click(doors[0]);
      expect(window.location.hash).toBe("#/analytics/pipeline");
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
    expect((await stageTable()).getByText("€49.38")).toBeTruthy();
    expect(screen.queryByText("€49.37")).toBeNull();
  });

  it("switching to Open deals per company groups by company_id and renders a table", async () => {
    vi.stubGlobal(
      "fetch",
      reportsStub({
        companyRows: [
          {
            company_id: "o1",
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
              company_id: "o1",
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
      expect(door.getAttribute("href")).toContain("company_id=o1");
      expect(door.getAttribute("href")).toContain("status=open");
    });

    it("draws the count plainly when the company has two currency rows", async () => {
      vi.stubGlobal(
        "fetch",
        reportsStub({
          companyRows: [
            {
              company_id: "o1",
              raw_minor: 250000,
              deal_count: 4,
              currency: "EUR",
            },
            {
              company_id: "o1",
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

  // Performance's views were once one page with a switch named by `?view=`.
  it("answers an old view-switch address with that view's tab", () => {
    expect(sectionFromAddress("performance", "performance", "sdr")).toBe("sdr");
    expect(sectionFromAddress(undefined, "performance", "leads")).toBe("leads");
    expect(sectionFromAddress("performance", "performance", "sales")).toBe(
      "performance",
    );
    expect(sectionFromAddress("forecast", "performance", "sdr")).toBe(
      "forecast",
    );
    expect(
      sectionFromAddress("performance", "performance", "constructor"),
    ).toBe("performance");
  });

  // A segment is whatever a reader typed. Anything unrecognized lands on the
  // first section rather than rendering an empty screen.
  it("falls back to the first section for anything it does not know", () => {
    expect(sectionFromAddress(undefined)).toBe("overview");
    expect(sectionFromAddress("nonsense")).toBe("overview");
  });
});
