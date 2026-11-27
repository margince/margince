// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { QueryClient } from "@tanstack/react-query";
import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { en as englishCopy } from "../../i18n/en";
import { forgetSeat } from "../../lib/storage";
import { jsonResponse } from "../../testing/story-utils";
import {
  isSetup,
  OVERVIEW_SCENARIOS,
  type OverviewView,
  overviewAddress,
  type ScenarioName,
} from "./analytics.overview.scenarios.fixtures";
import {
  type AskedRequest,
  bySeat,
  openOverview,
  readings,
} from "./analytics.overview.testkit";

const originalFetch = globalThis.fetch;
beforeEach(() => forgetSeat());
afterEach(() => {
  cleanup();
  globalThis.fetch = originalFetch;
  window.location.hash = "";
});

const evidenceAsked = (asked: readonly AskedRequest[]) =>
  asked
    .map(({ url }) => url)
    .filter((url) => url.pathname.endsWith("/analytics/evidence"));

// The read of Sales review's charts, which the figures never wait for.
const isChartRead = (url: URL) =>
  url.pathname.endsWith("/analytics/evaluate") &&
  url.searchParams.getAll("blocks").includes("bookings_trend");

function openScenario(name: ScenarioName, view: OverviewView) {
  const pair = OVERVIEW_SCENARIOS[name].views[view];
  if (!isSetup(pair)) throw new Error(`${name} excludes ${view}`);
  return openOverview(overviewAddress(view), pair.answer, pair.routes);
}

describe("one evidence interaction on Sales review", () => {
  it("opens the records behind a reading under its receipt, and returns focus to its door", async () => {
    const user = userEvent.setup({ delay: null });
    const asked = openOverview("#/analytics", bySeat("leader"));
    const strip = await readings();
    const door = strip.getByRole("button", {
      name: englishCopy["reporting.viewRecords"],
      description: "Sales won",
    });

    await user.click(door);

    const dialog = within(
      await screen.findByRole("dialog", { name: "Sales won" }),
    );
    expect(dialog.getByText(/^Figures read .+, in EUR\./)).toBeVisible();
    await waitFor(() => expect(evidenceAsked(asked)).toHaveLength(1));
    const [request] = evidenceAsked(asked);
    expect(request?.searchParams.get("metric")).toBe("bookings_won");
    expect(request?.searchParams.get("evaluation_key")).toBe(
      "fixture-evaluation",
    );
    await user.keyboard("{Escape}");
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(door).toHaveFocus();
  });

  it("opens quarter pipeline records through the supplied evidence reference", async () => {
    const asked = openOverview("#/analytics", bySeat("leader"));
    const strip = await readings();
    const door = strip.getByRole("button", {
      name: englishCopy["reporting.viewRecords"],
      description: englishCopy["analytics.outlook.periodPipeline"],
    });
    await userEvent.click(door);
    await screen.findByRole("dialog", {
      name: englishCopy["analytics.outlook.periodPipeline"],
    });
    await waitFor(() => expect(evidenceAsked(asked)).toHaveLength(1));
    expect(evidenceAsked(asked)[0]?.searchParams.get("context_id")).toBe(
      "forecast_period_deals",
    );
    expect(
      strip.getByRole("button", {
        name: englishCopy["reporting.viewRecords"],
        description: englishCopy["analytics.outlook.periodPipeline"],
      }),
    ).toBeVisible();
  });

  it("keeps an open drawer on the receipt it was opened from when the figures are read again", async () => {
    const user = userEvent.setup({ delay: null });
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    const leader = bySeat("leader");
    let reads = 0;
    // Each headline read answers with a new receipt, an hour later.
    const answer = (url: URL) => {
      const evaluation = leader(url);
      if (url.searchParams.has("blocks") || evaluation instanceof Response)
        return evaluation;
      reads += 1;
      return {
        ...evaluation,
        evaluation_key: `receipt-${reads}`,
        context: {
          ...evaluation.context,
          evaluated_at: `2026-09-30T${String(8 + reads).padStart(2, "0")}:00:00Z`,
        },
      };
    };
    const asked = openOverview("#/analytics", answer, {}, client);
    const strip = await readings();
    await user.click(
      strip.getByRole("button", {
        name: englishCopy["reporting.viewRecords"],
        description: "Sales won",
      }),
    );
    const dialog = within(await screen.findByRole("dialog"));
    const receipt = dialog.getByText(/^Figures read /).textContent;
    await waitFor(() => expect(evidenceAsked(asked)).toHaveLength(1));
    const opened = reads;

    await client.invalidateQueries({ queryKey: ["reporting-evaluation"] });

    await waitFor(() => expect(reads).toBeGreaterThan(opened));
    expect(dialog.getByText(/^Figures read /).textContent).toBe(receipt);
    expect(
      evidenceAsked(asked).map((url) => url.searchParams.get("evaluation_key")),
    ).toEqual([`receipt-${opened}`]);
  });

  it("keeps Forecast a link that goes there, never the records door", async () => {
    openOverview("#/analytics", bySeat("leader"));
    const strip = await readings();
    const forecast = strip.getByRole("link", {
      name: "Forecast",
      description: englishCopy["forecast.basis.likely"],
    });
    expect(forecast.getAttribute("href")).toMatch(/^\/#\/analytics\/forecast/);
    expect(
      strip.queryByRole("button", {
        description: englishCopy["forecast.basis.likely"],
      }),
    ).toBeNull();
  });

  it("shows each figure on the chart as its tab, with the chart's own door", async () => {
    const user = userEvent.setup({ delay: null });
    openScenario("populated", "sales");
    const strip = within(
      await screen.findByRole("region", {
        name: englishCopy["analytics.overview.readings"],
      }),
    );
    const won = strip.getByRole("button", { description: "Sales won" });
    const forecast = strip.getByRole("button", {
      description: englishCopy["forecast.basis.likely"],
    });
    expect(won).toHaveAttribute("aria-pressed", "true");
    expect(forecast).toHaveAttribute("aria-pressed", "false");

    await user.click(forecast);

    expect(forecast).toHaveAttribute("aria-pressed", "true");
    const plot = within(
      screen.getByRole("region", {
        name: englishCopy["analytics.hero.chart.forecast"],
      }),
    );
    expect(
      plot
        .getByRole("link", { name: englishCopy["analytics.hero.openForecast"] })
        .getAttribute("href"),
    ).toMatch(/^\/#\/analytics\/forecast/);
    expect(
      plot.queryByRole("button", {
        name: englishCopy["reporting.viewRecords"],
      }),
    ).toBeNull();
  });

  it("opens a measured zero as its explanation and an empty permitted population", async () => {
    const user = userEvent.setup({ delay: null });
    const asked = openScenario("trueZero", "sales");
    // Sales won heads the chart, so its records open from the chart's foot.
    const plot = within(
      await screen.findByRole("region", {
        name: englishCopy["analytics.hero.chart.won"],
      }),
    );

    await user.click(
      plot.getByRole("button", { name: englishCopy["reporting.viewRecords"] }),
    );

    const dialog = within(await screen.findByRole("dialog"));
    expect(
      dialog.getByText(englishCopy["reporting.evidenceZero"]),
    ).toBeVisible();
    expect(await dialog.findByText("0 records shown")).toBeVisible();
    expect(evidenceAsked(asked)[0]?.searchParams.get("metric")).toBe(
      "bookings_won",
    );
  });

  it("says why a failed figure has no records and offers a retry, asking for none", async () => {
    const user = userEvent.setup({ delay: null });
    const asked = openScenario("sourceUnavailable", "sales");
    // Sales won heads the chart, so its records open from the chart's foot.
    const plot = within(
      await screen.findByRole("region", {
        name: englishCopy["analytics.hero.chart.won"],
      }),
    );

    await user.click(
      plot.getByRole("button", { name: englishCopy["reporting.viewRecords"] }),
    );

    const dialog = within(await screen.findByRole("dialog"));
    expect(
      dialog.getByText(englishCopy["reporting.evidenceUnavailable"]),
    ).toBeVisible();
    expect(dialog.getByRole("button", { name: "Retry" })).toBeVisible();
    expect(dialog.queryByRole("table")).toBeNull();
    expect(evidenceAsked(asked)).toHaveLength(0);
  });
});

describe("one evidence interaction on the prospecting views", () => {
  it("keeps a hidden figure's records and their count hidden, asking for none", async () => {
    const user = userEvent.setup({ delay: null });
    const asked = openScenario("readingsWithheld", "prospecting");
    const strip = await readings();

    await user.click(
      strip.getByRole("button", {
        name: englishCopy["reporting.viewRecords"],
        description: englishCopy["analytics.team.meetings"],
      }),
    );

    const dialog = within(await screen.findByRole("dialog"));
    expect(
      dialog.getByText(englishCopy["reporting.evidenceWithheld"]),
    ).toBeVisible();
    expect(dialog.queryByText(/records? shown/)).toBeNull();
    expect(dialog.queryByRole("button", { name: "Retry" })).toBeNull();
    expect(evidenceAsked(asked)).toHaveLength(0);
  });

  it("opens a team bottleneck's population in the same frame, with its basis", async () => {
    const user = userEvent.setup({ delay: null });
    openScenario("populated", "team");
    const standing = within(
      await screen.findByRole("region", {
        name: englishCopy["analytics.team.standing"],
      }),
    );
    const [first] = await standing.findAllByRole("button");
    if (!first) throw new Error("the populated team has a bottleneck");

    await user.click(first);

    const dialog = await screen.findByRole("dialog");
    expect(within(dialog).getAllByText(/./).length).toBeGreaterThan(1);
    await user.keyboard("{Escape}");
    expect(first).toHaveFocus();
  });
});

describe("headline, actions and charts are ready on their own", () => {
  it("draws the figures and the actions while the chart read never answers", async () => {
    openOverview("#/analytics", bySeat("leader"), {}, undefined, isChartRead);
    const strip = await readings();
    expect(
      strip.getByText(englishCopy["analytics.outlook.periodPipeline"]),
    ).toBeVisible();
    expect(
      within(
        screen.getByRole("region", {
          name: englishCopy["analytics.overview.readings"],
        }),
      ).getByText("€420,000"),
    ).toBeVisible();
    expect(screen.queryByRole("region", { name: "Summary" })).toBeNull();
    expect(
      screen.queryByRole("heading", {
        name: englishCopy["reporting.stage_distribution"],
      }),
    ).toBeNull();
  });

  it("says a failed chart read in its own place, under figures that stand", async () => {
    const leader = bySeat("leader");
    openOverview("#/analytics", (url) =>
      isChartRead(url)
        ? jsonResponse({ title: "Unavailable", status: 503 }, 503)
        : leader(url),
    );
    const strip = await readings();
    // Announced where the charts would stand, with its own retry.
    expect(await screen.findByRole("alert")).toBeVisible();
    expect(screen.getByRole("button", { name: "Retry" })).toBeVisible();
    expect(
      strip.getByText(englishCopy["analytics.outlook.periodPipeline"]),
    ).toBeVisible();
  });
});

describe("a reading hidden whole", () => {
  it("says it is hidden, never that only a part of it is", async () => {
    openScenario("readingsWithheld", "team");
    const strip = await readings();
    expect(
      strip.getAllByText(englishCopy["analytics.prospect.withheld"]).length,
    ).toBeGreaterThan(0);
    expect(
      strip.queryByText(englishCopy["analytics.overview.partlyHidden"]),
    ).toBeNull();
  });
});

describe("a drawer belongs to the selection it was opened in", () => {
  it("stays closed when the reader changes the selection and comes back", async () => {
    const user = userEvent.setup({ delay: null });
    openOverview("#/analytics/overview?overview=sales", bySeat("leader"));
    const strip = await readings();
    await user.click(
      strip.getByRole("button", {
        name: englishCopy["reporting.viewRecords"],
        description: "Sales won",
      }),
    );
    await screen.findByRole("dialog");

    window.location.hash =
      "#/analytics/overview?overview=sales&period=this_month";
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    window.location.hash = "#/analytics/overview?overview=sales";
    await readings();

    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("keeps a team bottleneck's list closed after the period changes and changes back", async () => {
    const user = userEvent.setup({ delay: null });
    openScenario("populated", "team");
    const standing = within(
      await screen.findByRole("region", {
        name: englishCopy["analytics.team.standing"],
      }),
    );
    const [first] = await standing.findAllByRole("button");
    if (!first) throw new Error("the populated team has a bottleneck");
    await user.click(first);
    await screen.findByRole("dialog");
    const address = window.location.hash;

    window.location.hash = `${address}&period=last_week`;
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    window.location.hash = address;
    await screen.findByRole("region", {
      name: englishCopy["analytics.team.standing"],
    });

    expect(screen.queryByRole("dialog")).toBeNull();
  });
});

// Presses Tab from the top of the page until `target` has focus.
async function tabTo(
  user: ReturnType<typeof userEvent.setup>,
  target: HTMLElement,
): Promise<number> {
  (document.activeElement as HTMLElement | null)?.blur();
  for (let presses = 1; presses <= 200; presses += 1) {
    await user.tab();
    if (document.activeElement === target) return presses;
  }
  throw new Error(`Tab never reached ${target.outerHTML.slice(0, 120)}`);
}

describe("a keyboard-only path", () => {
  it("reaches Sales review's first action and a chart's table", async () => {
    const user = userEvent.setup({ delay: null });
    openScenario("largeBacklog", "sales");
    const actions = within(
      await screen.findByRole("region", {
        name: englishCopy["analytics.attention"],
      }),
    );
    const [first] = await actions.findAllByRole("link");
    if (!first) throw new Error("the backlog review has an action");
    await tabTo(user, first);
    const [label] = await screen.findAllByText(englishCopy["reporting.data"]);
    const table = label?.closest<HTMLElement>("summary, button");
    if (!table) throw new Error("a chart offers its table");
    await tabTo(user, table);
  });

  it("reaches My prospecting's first move", async () => {
    const user = userEvent.setup({ delay: null });
    openScenario("populated", "prospecting");
    const moves = within(
      await screen.findByRole("region", { name: "Do next" }),
    );
    const [first] = await moves.findAllByRole("link");
    if (!first) throw new Error("the populated week has a move");
    await tabTo(user, first);
  });

  it("reaches Team prospecting's first bottleneck", async () => {
    const user = userEvent.setup({ delay: null });
    openScenario("populated", "team");
    const standing = within(
      await screen.findByRole("region", {
        name: englishCopy["analytics.team.standing"],
      }),
    );
    const [first] = await standing.findAllByRole("button");
    if (!first) throw new Error("the populated team has a bottleneck");
    await tabTo(user, first);
  });
});

describe("a month with no target", () => {
  it("tells a seat that may not set one who can", async () => {
    openScenario("noTarget", "prospecting");
    expect(
      await screen.findByText(
        /^No monthly target set\. An admin or a sales manager can usually set one\.$/,
      ),
    ).toBeVisible();
  });
});
