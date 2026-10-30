/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import {
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it } from "vitest";
import { formatHours, formatMoneyCompact } from "../../format/format";
import { en } from "../../i18n/en";
import { reportingStoryEvaluation } from "../../testing/reporting.fixtures";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../../testing/story-utils";
import { AnalyticsScreen } from "./analytics";
import { render as renderScreen } from "./analytics.testkit";
import type { ReportingEvaluation } from "./reporting.model";
import { ReportingOverview } from "./reporting.overview";
import {
  reportingLeadsEvaluation,
  reportingPartnersEvaluation,
  reportingSectionRoutes,
  reportingSectionsCatalog,
  reportingStoryScope,
} from "./reporting.story-fixtures";
import {
  metricView,
  REPORTING_VIEWS,
  type ReportingView,
  VIEW_SHAPE,
} from "./reporting.views";

const originalFetch = globalThis.fetch;
afterEach(() => {
  cleanup();
  globalThis.fetch = originalFetch;
  window.location.hash = "";
});

type Catalog = typeof reportingSectionsCatalog;

// The Performance page on one view, answering every evaluation with one
// fixture. Returns the addresses it asked, so a case can read what it sent.
function showView(
  view: ReportingView,
  evaluation: ReportingEvaluation,
  catalog: Catalog = reportingSectionsCatalog,
  address = "",
) {
  window.location.hash = `#/analytics/${VIEW_SHAPE[view].section}${address}`;
  installFetchStub({
    ...reportingSectionRoutes(evaluation),
    "GET /analytics/metrics": () =>
      jsonResponse({ ...catalog, schedule_ready: true }),
  });
  const stub = globalThis.fetch;
  const asked: URL[] = [];
  const recording: typeof fetch = (
    input: RequestInfo | URL,
    init?: RequestInit,
  ) => {
    asked.push(
      new URL(
        input instanceof Request ? input.url : String(input),
        "http://localhost",
      ),
    );
    return stub(input, init);
  };
  globalThis.fetch = recording;
  render(
    <StoryProviders>
      <ReportingOverview view={view} scope={reportingStoryScope} />
    </StoryProviders>,
  );
  return asked;
}

const titled = (metric: string) =>
  en["reporting.breakdownTitle"].replace("{metric}", metric);

async function panel(title: string) {
  const heading = await screen.findByRole("heading", { name: title });
  const card = heading.closest("section");
  if (!(card instanceof HTMLElement)) throw new Error(`${title} has no panel`);
  return within(card);
}

describe("the Leads view", () => {
  it("reads the lead metrics without a pipeline and splits them by group", async () => {
    const asked = showView("leads", reportingLeadsEvaluation);

    const created = await panel(titled(en["reporting.leads_created"]));
    expect(created.getByRole("button", { name: "Website: 12" })).toBeVisible();
    const evaluate = asked.find((url) =>
      url.pathname.endsWith("/analytics/evaluate"),
    );
    expect(evaluate?.searchParams.getAll("metrics")).toEqual([
      "leads_created",
      "lead_status_moves",
      "leads_promoted",
      "lead_first_response",
    ]);
    // Lead readers refuse a pipeline, so none is sent and none is offered.
    expect(evaluate?.searchParams.get("pipeline_id")).toBeNull();
    expect(screen.queryByRole("combobox", { name: /Pipeline/ })).toBeNull();
  });

  it("asks with the default close window whatever the address names", async () => {
    const asked = showView(
      "leads",
      reportingLeadsEvaluation,
      reportingSectionsCatalog,
      "?close=fiscal_quarter",
    );

    await panel(titled(en["reporting.leads_created"]));
    const evaluate = asked.find((url) =>
      url.pathname.endsWith("/analytics/evaluate"),
    );
    expect(evaluate?.searchParams.get("close_window")).toBe("all_open");
  });

  it("states a first response in hours and a qualified share out of its total", async () => {
    showView("leads", reportingLeadsEvaluation);

    // The headline and the metrics table both state it.
    expect(await screen.findAllByText(formatHours(3.5, "en"))).not.toHaveLength(
      0,
    );
    expect(
      screen.getByText(
        new RegExp(en["reporting.outOf"].replace("{total}", "24")),
      ),
    ).toBeVisible();
  });

  it("opens a group's own records from its bar", async () => {
    const user = userEvent.setup({ delay: null });
    const asked = showView("leads", reportingLeadsEvaluation);

    const created = await panel(titled(en["reporting.leads_created"]));
    await user.click(created.getByRole("button", { name: "Website: 12" }));

    const drawer = within(await screen.findByRole("dialog"));
    expect(drawer.getByText(/Website/)).toBeVisible();
    const evidence = asked.find((url) =>
      url.pathname.endsWith("/analytics/evidence"),
    );
    expect(evidence?.searchParams.get("group_key")).toBe("origin:web");
    expect(evidence?.searchParams.get("metric")).toBe("leads_created");
  });

  it("says why a breakdown has nothing to draw", async () => {
    const empty: ReportingEvaluation = {
      ...reportingLeadsEvaluation,
      charts: reportingLeadsEvaluation.charts.map((chart) => ({
        ...chart,
        points: [],
        coverage: {
          status: "no_data",
          withheld: false,
          reason: "No lead came in during this period.",
        },
      })),
    };
    showView("leads", empty);

    const created = await panel(titled(en["reporting.leads_created"]));
    expect(
      created.getByText("No lead came in during this period."),
    ).toBeVisible();
    expect(created.queryByRole("button", { name: /Website/ })).toBeNull();
  });

  it("says once for the view, not on each card, that it leaves out what the reader may not see", async () => {
    const withheld: ReportingEvaluation = {
      ...reportingLeadsEvaluation,
      metrics: reportingLeadsEvaluation.metrics.map((metric) => ({
        ...metric,
        coverage: { ...metric.coverage, withheld: true },
      })),
      charts: reportingLeadsEvaluation.charts.map((chart) => ({
        ...chart,
        coverage: { status: "ok", withheld: true },
      })),
    };
    showView("leads", withheld);

    const created = await panel(titled(en["reporting.leads_created"]));
    expect(created.queryByText(en["reporting.withheld"])).toBeNull();
    expect(screen.getAllByText(en["reporting.withheld"])).toHaveLength(1);
  });
});

describe("the Partners view", () => {
  it("names a partner the reader cannot open in words, never by its key", async () => {
    showView("partners", reportingPartnersEvaluation);

    const pipeline = await panel(titled(en["reporting.partner_pipeline"]));
    const amount = formatMoneyCompact(500_000, "EUR", "en");
    expect(
      pipeline.getByRole("button", {
        name: `Partner you cannot see · Sourced: ${amount}`,
      }),
    ).toBeVisible();
    expect(pipeline.queryByText(/partner:hidden/)).toBeNull();
  });

  it("counts the records behind a money group, and not behind a count", async () => {
    showView("partners", reportingPartnersEvaluation);

    const commission = await panel(titled(en["reporting.partner_commission"]));
    expect(
      commission.getByRole("columnheader", {
        name: en["reporting.recordCount"],
      }),
    ).toBeInTheDocument();
    const activity = await panel(titled(en["reporting.partner_activity"]));
    expect(
      activity.queryByRole("columnheader", {
        name: en["reporting.recordCount"],
      }),
    ).toBeNull();
  });
});

const TAB_LABEL = {
  sales: "analytics.sectionPerformance",
  sdr: "reporting.sdr",
  leads: "reporting.leads",
  partners: "reporting.partners",
} as const;

describe("which views a seat reads", () => {
  const withoutLeads: Catalog = {
    metrics: reportingSectionsCatalog.metrics.filter(
      (metric) => metricView(metric.id) !== "leads",
    ),
    presets: reportingSectionsCatalog.presets,
  };

  // The whole screen, because which views are tabs is the screen's to decide.
  function showScreen(address: string, catalog: Catalog) {
    window.location.hash = address;
    installFetchStub({
      ...reportingSectionRoutes(reportingPartnersEvaluation),
      "GET /analytics/metrics": () =>
        jsonResponse({ ...catalog, schedule_ready: true }),
    });
    renderScreen(<AnalyticsScreen />);
  }

  it("offers each view the seat's catalog holds as a tab", async () => {
    showScreen("#/analytics/partners", reportingSectionsCatalog);

    for (const view of REPORTING_VIEWS) {
      expect(
        await screen.findByRole("button", {
          name: en[TAB_LABEL[view]],
          pressed: view === "partners",
        }),
      ).toBeInTheDocument();
    }
  });

  it("is not offered it, and its address opens the first view the seat has", async () => {
    showScreen("#/analytics/leads", withoutLeads);

    expect(
      await screen.findByRole("button", {
        name: en["analytics.sectionPerformance"],
        pressed: true,
      }),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: en["reporting.leads"] }),
    ).toBeNull();
  });

  it("opens the tab an old view-switch address named", async () => {
    showScreen("#/analytics/performance?view=partners", withoutLeads);

    expect(
      await screen.findByRole("button", {
        name: en["reporting.partners"],
        pressed: true,
      }),
    ).toBeInTheDocument();
  });

  it("offers no Sales view to a reader whose catalog publishes no sales preset", async () => {
    showScreen("#/analytics/performance", {
      metrics: [
        {
          id: "forecast_landing",
          version: "1",
          unit: "money",
          temporal_basis: "state_at",
          supports_target: false,
          definition:
            "The deal forecast's expected finish for this fiscal quarter.",
          blocks: ["forecast_support", "metric_reading"],
        },
        ...reportingSectionsCatalog.metrics.filter(
          (metric) => metricView(metric.id) === "leads",
        ),
      ],
      presets: [],
    });

    expect(
      await screen.findByRole("button", {
        name: en["reporting.leads"],
        pressed: true,
      }),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("button", {
        name: en["analytics.sectionPerformance"],
      }),
    ).toBeNull();
  });

  it("says why there is no view to choose when it reads none", async () => {
    showScreen("#/analytics/performance", { metrics: [], presets: [] });

    expect(await screen.findByText(en["reporting.noViews"])).toBeVisible();
    expect(
      screen.queryByRole("button", { name: en["reporting.sdr"] }),
    ).toBeNull();
  });
});

// The opening request of a preset view is the server's definition from the
// catalog. The browser adds no metric of its own and drops none.
describe("the preset views", () => {
  async function evaluated(asked: URL[]) {
    let evaluate: URL | undefined;
    await waitFor(() => {
      evaluate = asked.find((url) =>
        url.pathname.endsWith("/analytics/evaluate"),
      );
      expect(evaluate).toBeDefined();
    });
    return {
      metrics: evaluate?.searchParams.getAll("metrics"),
      blocks: evaluate?.searchParams.getAll("blocks"),
    };
  }

  it.each(["sales", "sdr"] as const)(
    "asks the %s preset the catalog publishes",
    async (view) => {
      const preset = reportingSectionsCatalog.presets.find(
        (entry) => entry.id === view,
      );
      const asked = showView(view, reportingStoryEvaluation);

      expect(await evaluated(asked)).toEqual({
        metrics: preset?.metrics,
        blocks: preset?.blocks,
      });
    },
  );

  it("follows a narrower preset rather than every metric of the view", async () => {
    const narrow: Catalog = {
      ...reportingSectionsCatalog,
      presets: [
        { id: "sales", metrics: ["bookings_won"], blocks: ["bookings_trend"] },
      ],
    };
    const asked = showView("sales", reportingStoryEvaluation, narrow);

    expect(await evaluated(asked)).toEqual({
      metrics: ["bookings_won"],
      blocks: ["bookings_trend"],
    });
  });

  it("asks nothing for a view whose preset the reader is not offered", async () => {
    const asked = showView("sdr", reportingStoryEvaluation, {
      ...reportingSectionsCatalog,
      presets: [],
    });

    expect(await screen.findByText(en["reporting.noViews"])).toBeVisible();
    expect(
      asked.some((url) => url.pathname.endsWith("/analytics/evaluate")),
    ).toBe(false);
  });
});
