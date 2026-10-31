/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";
import {
  act,
  cleanup,
  fireEvent,
  renderHook,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { pickOption } from "../../design-system/select-testing";
import { forgetSeat } from "../../lib/storage";
import { meFixture } from "../../testing/mefixture";
import { reportingFixtureFrames } from "../../testing/reporting.fixtures";
import { jsonResponse } from "../../testing/story-utils";
import {
  chooseContext,
  rememberedContext,
  rememberView,
} from "./analytics.overview.context";
import { backlogFindings } from "./analytics.overview.scenariodata";
import {
  isSetup,
  OVERVIEW_SCENARIOS,
} from "./analytics.overview.scenarios.fixtures";
import {
  ADMIN_GRANTS,
  type AskedRequest,
  isMovement,
  openOverview as open,
  prospectingEvaluation,
  readings,
  SELF,
  salesEvaluation,
  startingMovement,
  TEAM,
  teamEvaluation,
  twoPipelines,
} from "./analytics.overview.testkit";
import { useSelectionEvidence } from "./reporting.evidence";
import type { ReportingEvaluation } from "./reporting.model";

const READER = meFixture({}).user.id;
const SOMEONE_ELSE = "00000000-0000-4000-8000-0000000000e9";
const TWO = { pipelines: twoPipelines, template: "sdr" } as const;

const originalFetch = globalThis.fetch;
beforeEach(() => forgetSeat());
afterEach(() => {
  cleanup();
  globalThis.fetch = originalFetch;
  window.location.hash = "";
});

// Each view's evaluation, told apart by what it asks.
function answer(url: URL): ReportingEvaluation {
  if (isMovement(url)) return startingMovement();
  if (url.searchParams.get("scope_kind") === "owner")
    return prospectingEvaluation();
  return url.searchParams.getAll("metrics").includes("forecast_landing")
    ? salesEvaluation()
    : teamEvaluation();
}

const salesAsked = (asked: readonly AskedRequest[]) =>
  asked.filter(
    ({ url }) =>
      url.pathname.endsWith("/analytics/evaluate") &&
      !isMovement(url) &&
      url.searchParams.getAll("metrics").includes("forecast_landing"),
  );

const lastSales = (asked: readonly AskedRequest[]) => {
  const last = salesAsked(asked).at(-1);
  if (!last) throw new Error("Sales review asked nothing");
  return last.url.searchParams;
};

const forecastAsked = (asked: readonly AskedRequest[]) =>
  asked.filter(({ url }) => url.pathname.endsWith("/forecast"));

// The query of a link's address, which is a hash under the app's root.
function dialsOfLink(link: HTMLElement): Record<string, string> {
  const href = link.getAttribute("href") ?? "";
  return Object.fromEntries(
    new URLSearchParams(href.slice(href.indexOf("?") + 1)),
  );
}

const view = (name: string) => screen.findByRole("button", { name });

async function chooseAllPipelines(user: ReturnType<typeof userEvent.setup>) {
  // Sales review keeps the pipeline behind Filters.
  const filters = screen.queryByRole("button", { name: "Filters" });
  if (filters?.getAttribute("aria-expanded") === "false")
    await user.click(filters);
  await pickOption(
    user,
    await screen.findByRole("combobox", { name: "Pipeline" }),
    "All pipelines",
  );
  await waitFor(() => expect(window.location.hash).toContain("pipeline=all"));
}

describe("each Overview view keeps its own question", () => {
  it("brings all pipelines back to Sales review after the other two views", async () => {
    const user = userEvent.setup({ delay: null });
    const asked = open("#/analytics", answer, TWO);
    await readings();
    await chooseAllPipelines(user);

    await user.click(await view("My prospecting"));
    expect(window.location.hash).not.toContain("pipeline=");
    await user.click(await view("Team prospecting"));
    expect(window.location.hash).not.toContain("pipeline=");
    await user.click(await view("Sales review"));

    await waitFor(() => expect(window.location.hash).toContain("pipeline=all"));
    await readings();
    expect(screen.getByText("Pipeline: All pipelines")).toBeVisible();
    expect(lastSales(asked).has("pipeline_id")).toBe(false);
  });

  it("brings an untouched default back as the default, not as all pipelines", async () => {
    const user = userEvent.setup({ delay: null });
    const asked = open("#/analytics", answer, TWO);
    await readings();
    await user.click(await view("Team prospecting"));
    await user.click(await view("Sales review"));
    await readings();
    expect(window.location.hash).not.toContain("pipeline=");
    expect(lastSales(asked).get("pipeline_id")).toBe("sales");
  });

  it("brings custom dates back after a switch, and carries them out", async () => {
    const user = userEvent.setup({ delay: null });
    open(
      "#/analytics/overview?overview=sales&period=custom&from=2026-10-01&through=2026-10-15",
      answer,
      TWO,
    );
    await readings();
    await user.click(await view("Team prospecting"));
    expect(window.location.hash).not.toContain("from=");
    await user.click(await view("Sales review"));
    await readings();
    const pipeline = await screen.findByRole("link", { name: "Pipeline" });
    expect(dialsOfLink(pipeline)).toEqual({
      overview: "sales",
      scope: `team:${TEAM.id}`,
      pipeline: "sales",
      period: "custom",
      from: "2026-10-01",
      through: "2026-10-15",
      close: "all_open",
    });
  });

  it("keeps the team view's period apart from Sales review's", async () => {
    const user = userEvent.setup({ delay: null });
    open("#/analytics/overview?overview=team", answer, TWO);
    await readings();
    await pickOption(
      user,
      screen.getByRole("combobox", { name: "Date range" }),
      "Last completed week",
    );
    await waitFor(() =>
      expect(window.location.hash).toContain("period=last_week"),
    );
    await user.click(await view("Sales review"));
    expect(window.location.hash).not.toContain("period=");
    await user.click(await view("Team prospecting"));
    await waitFor(() =>
      expect(window.location.hash).toContain("period=last_week"),
    );
  });
});

describe("which question wins", () => {
  it("opens a shared link's question over the remembered one, and keeps no choice from it", async () => {
    chooseContext(READER, "sales", { pipeline: "all" });
    const asked = open(
      "#/analytics/overview?overview=sales&pipeline=renewals",
      answer,
      TWO,
    );
    await readings();
    expect(lastSales(asked).get("pipeline_id")).toBe("renewals");
    // A new tab: what this reader chose is untouched by the link they opened.
    sessionStorage.clear();
    expect(rememberedContext(READER, "sales")).toEqual({ pipeline: "all" });
  });

  it("restores the chosen question after a reload and in a new tab", async () => {
    const user = userEvent.setup({ delay: null });
    open("#/analytics", answer, TWO);
    await readings();
    await chooseAllPipelines(user);
    cleanup();

    const reloaded = open("#/analytics", answer, TWO);
    await readings();
    expect(window.location.hash).toContain("pipeline=all");
    expect(lastSales(reloaded).has("pipeline_id")).toBe(false);
    cleanup();

    sessionStorage.clear();
    const newTab = open("#/analytics", answer, TWO);
    await readings();
    expect(lastSales(newTab).has("pipeline_id")).toBe(false);
  });

  it("opens nothing another account chose", async () => {
    chooseContext(SOMEONE_ELSE, "sales", { pipeline: "all" });
    rememberView(SOMEONE_ELSE, "team");
    const asked = open("#/analytics", answer, TWO);
    await readings();
    expect(await view("Sales review")).toHaveAttribute("aria-pressed", "true");
    expect(lastSales(asked).get("pipeline_id")).toBe("sales");
  });

  it("falls back to the recommended view when the remembered one is no longer offered", async () => {
    rememberView(READER, "team");
    open("#/analytics", answer, { seat: "sdr" });
    expect(await view("My prospecting")).toHaveAttribute(
      "aria-pressed",
      "true",
    );
    expect(screen.queryByRole("button", { name: "Team prospecting" })).toBe(
      null,
    );
  });

  it("forgets every choice at sign-out", async () => {
    chooseContext(READER, "sales", { pipeline: "all" });
    forgetSeat();
    const asked = open("#/analytics", answer, TWO);
    await readings();
    expect(lastSales(asked).get("pipeline_id")).toBe("sales");
  });
});

describe("a remembered or shared question the reader can no longer ask", () => {
  it("drops a remembered team the reader lost and reads their default", async () => {
    chooseContext(READER, "team", { scope: "team:gone", period: "last_week" });
    const asked = open("#/analytics/overview?overview=team", answer, TWO);
    await readings();
    expect(window.location.hash).toContain("period=last_week");
    expect(window.location.hash).not.toContain("scope=");
    const review = asked.find(({ url }) =>
      url.pathname.endsWith("/analytics/sdr-team"),
    );
    expect(review?.url.searchParams.get("scope_id")).toBe(TEAM.id);
  });

  it("says so for a pipeline that is gone, and reads nothing until one is chosen", async () => {
    const user = userEvent.setup({ delay: null });
    chooseContext(READER, "sales", { pipeline: "retired" });
    const asked = open("#/analytics", answer, TWO);
    expect(
      await screen.findByText("This view names a pipeline you cannot open"),
    ).toBeVisible();
    expect(salesAsked(asked)).toHaveLength(0);
    await user.click(
      screen.getByRole("button", { name: "Use the default pipeline" }),
    );
    await readings();
    expect(lastSales(asked).get("pipeline_id")).toBe("sales");
  });

  it("refuses a shared link's team the reader may not choose, instead of showing their own", async () => {
    const user = userEvent.setup({ delay: null });
    const asked = open(
      "#/analytics/overview?overview=sales&scope=team:elsewhere",
      answer,
      TWO,
    );
    expect(
      await screen.findByText("This link names figures you cannot open"),
    ).toBeVisible();
    expect(salesAsked(asked)).toHaveLength(0);
    await user.click(screen.getByRole("button", { name: "Show DACH" }));
    await readings();
    expect(lastSales(asked).get("scope_id")).toBe(TEAM.id);
  });
});

describe("every way out of Overview carries its question", () => {
  async function expectedFinishDoor() {
    const strip = await readings();
    return strip.getByRole("link", { description: "Expected finish" });
  }

  it("opens Forecast from Expected finish with the question, and says what it cannot narrow to", async () => {
    const user = userEvent.setup({ delay: null });
    const asked = open("#/analytics", answer, TWO);
    const door = await expectedFinishDoor();
    expect(dialsOfLink(door)).toEqual({
      overview: "sales",
      scope: `team:${TEAM.id}`,
      pipeline: "sales",
      period: "this_quarter",
      close: "all_open",
    });

    await user.click(door);
    expect(window.location.hash).toMatch(/^#\/analytics\/forecast\?/);
    expect(
      await screen.findByText("The forecast covers every pipeline"),
    ).toBeVisible();
    expect(forecastAsked(asked)).toHaveLength(0);

    await user.click(
      screen.getByRole("button", { name: "Show all pipelines" }),
    );
    await waitFor(() => expect(forecastAsked(asked).length).toBeGreaterThan(0));
    expect(forecastAsked(asked)[0].url.searchParams.get("period")).toBe(
      "quarter",
    );
  });

  it("reads the forecast straight away where there is one pipeline", async () => {
    const user = userEvent.setup({ delay: null });
    const asked = open("#/analytics", answer, { template: "sdr" });
    await user.click(await expectedFinishDoor());
    await waitFor(() => expect(forecastAsked(asked).length).toBeGreaterThan(0));
    expect(screen.queryByText("The forecast covers every pipeline")).toBeNull();
  });

  it("leaves a modified click to the browser and follows Enter", async () => {
    const user = userEvent.setup({ delay: null });
    open("#/analytics", answer, TWO);
    // Not prevented: the browser opens the address in a new tab.
    expect(fireEvent.click(await expectedFinishDoor(), { ctrlKey: true })).toBe(
      true,
    );
    cleanup();

    open("#/analytics", answer, TWO);
    const door = await expectedFinishDoor();
    door.focus();
    await user.keyboard("{Enter}");
    expect(window.location.hash).toMatch(/^#\/analytics\/forecast\?/);
  });

  it("carries the whole question to the deeper sections", async () => {
    const user = userEvent.setup({ delay: null });
    open("#/analytics", answer, TWO);
    await readings();
    await chooseAllPipelines(user);
    const pipeline = await screen.findByRole("link", { name: "Pipeline" });
    expect(dialsOfLink(pipeline)).toEqual({
      overview: "sales",
      scope: `team:${TEAM.id}`,
      pipeline: "all",
      period: "this_quarter",
      close: "all_open",
    });
  });

  it("opens a finding for the scope its list was read over, and its record with no Analytics dials", async () => {
    const clock = reportingFixtureFrames.quarterCutoff.clock;
    open("#/analytics", answer, {
      ...TWO,
      checks: () => jsonResponse({ data: backlogFindings(clock) }),
    });
    await readings();
    const review = (await screen.findAllByRole("link", { name: "Answer" }))[0];
    expect(dialsOfLink(review)).toEqual({
      overview: "sales",
      scope: `team:${TEAM.id}`,
      finding: backlogFindings(clock)[0]?.id,
    });
    const row = review.closest(".analytics-attention-row");
    if (!(row instanceof HTMLElement)) throw new Error("a finding has a row");
    const record = within(row)
      .getAllByRole("link")
      .find((link) => link !== review);
    expect(record?.getAttribute("href")).not.toMatch(/pipeline|period|scope/);
  });

  it("keeps My prospecting's doors on the reader's own records", async () => {
    open("#/analytics/overview?overview=prospecting", answer, TWO);
    await readings();
    const outcomes = await screen.findByRole("link", { name: "SDR outcomes" });
    expect(dialsOfLink(outcomes)).toEqual({
      overview: "prospecting",
      scope: `owner:${SELF}`,
      period: "this_week",
    });
  });
});

describe("a response that arrives late", () => {
  it("never puts the earlier question's figures under the later one", async () => {
    const user = userEvent.setup({ delay: null });
    const won = (minor: number) =>
      salesEvaluation({
        metrics: salesEvaluation().metrics.map((metric) =>
          metric.id === "bookings_won" ? { ...metric, value: minor } : metric,
        ),
      });
    let releaseDefault: (() => void) | undefined;
    const asked = open(
      "#/analytics",
      (url) => {
        if (isMovement(url)) return startingMovement();
        if (url.searchParams.get("pipeline_id") !== "sales")
          return won(99_000_000);
        return new Promise<Response>((resolve) => {
          releaseDefault = () => resolve(jsonResponse(won(11_000_000)));
        });
      },
      TWO,
    );
    await waitFor(() => expect(releaseDefault).toBeDefined());
    await chooseAllPipelines(user);
    expect(await screen.findByText("€990k")).toBeVisible();

    releaseDefault?.();
    await waitFor(() =>
      expect(salesAsked(asked)[0].signal?.aborted).toBe(true),
    );
    expect(screen.getByText("€990k")).toBeVisible();
    expect(screen.queryByText("€110k")).toBeNull();
  });
});

describe("evidence a reader opened", () => {
  it("closes when the question it was read from changes", () => {
    const reference = salesEvaluation().metrics[0].evidence;
    const { result, rerender } = renderHook(
      ({ question }) => useSelectionEvidence(question),
      { initialProps: { question: "pipeline=sales" } },
    );
    act(() => result.current.openEvidence(reference));
    expect(result.current.evidence).toEqual(reference);
    rerender({ question: "pipeline=all" });
    expect(result.current.evidence).toBeNull();
  });
});

describe("Save view", () => {
  // The server's echo of what it answered, as the real evaluator gives it.
  function echo(url: URL): ReportingEvaluation {
    if (isMovement(url)) return startingMovement();
    const base = salesEvaluation();
    return {
      ...base,
      selection: {
        ...base.selection,
        pipeline_id: url.searchParams.get("pipeline_id") ?? undefined,
        close_window:
          url.searchParams.get("close_window") === "fiscal_quarter"
            ? "fiscal_quarter"
            : "all_open",
      },
    };
  }

  it("saves the question on screen as a report, and says Overview stays as it is", async () => {
    const user = userEvent.setup({ delay: null });
    open("#/analytics", echo, { ...TWO, grants: ADMIN_GRANTS });
    const posted: unknown[] = [];
    const routed = globalThis.fetch;
    globalThis.fetch = (async (
      input: RequestInfo | URL,
      init?: RequestInit,
    ) => {
      if (
        input instanceof Request &&
        input.method === "POST" &&
        input.url.endsWith("/analytics/reports")
      ) {
        posted.push(await input.clone().json());
        return jsonResponse({ id: "report-1" }, 201);
      }
      return routed(input, init);
    }) as typeof fetch;
    await readings();
    await chooseAllPipelines(user);
    await pickOption(
      user,
      screen.getByRole("combobox", { name: "Expected close window" }),
      "Fiscal quarter",
    );
    await waitFor(() =>
      expect(window.location.hash).toContain("close=fiscal_quarter"),
    );
    await readings();

    await user.click(screen.getByRole("button", { name: "More" }));
    await user.click(screen.getByRole("button", { name: "Save view" }));
    expect(
      await screen.findByText(
        "Saving adds a report under Reports. Overview stays as it is.",
      ),
    ).toBeVisible();
    await user.type(
      screen.getByRole("textbox", { name: "Report name" }),
      "Quarter, every pipeline",
    );
    await user.click(screen.getByRole("button", { name: "Save report" }));

    await waitFor(() => expect(posted).toHaveLength(1));
    expect(posted[0]).toMatchObject({
      selection: {
        period: "this_quarter",
        close_window: "fiscal_quarter",
      },
    });
    expect(posted[0]).not.toHaveProperty("selection.pipeline_id");
  });
});

describe("a copied link", () => {
  it("answers for the population it names, never the opener's default", async () => {
    const asked = open(
      `#/analytics/overview?overview=sales&scope=owner:${SELF}`,
      answer,
      TWO,
    );
    await readings();
    expect(lastSales(asked).get("scope_kind")).toBe("owner");
    expect(lastSales(asked).get("scope_id")).toBe(SELF);
  });

  it("is refused to a second reader who may not see its team, instead of showing their own", async () => {
    open("#/analytics", answer, TWO);
    const door = (await readings()).getByRole("link", {
      description: "Expected finish",
    });
    const href = door.getAttribute("href") ?? "";
    cleanup();

    const asked = open(href.slice(href.indexOf("#")), answer, {
      ...TWO,
      seat: "sdr",
    });
    expect(
      await screen.findByText("This link names figures you cannot open"),
    ).toBeVisible();
    expect(forecastAsked(asked)).toHaveLength(0);
  });
});

describe("doors to setup and templates", () => {
  const noTarget = OVERVIEW_SCENARIOS.noTarget.views.prospecting;

  it("opens the target form on the reader's own records from My prospecting", async () => {
    if (!isSetup(noTarget)) throw new Error("noTarget draws My prospecting");
    const user = userEvent.setup({ delay: null });
    open("#/analytics/overview?overview=prospecting", noTarget.answer, {
      ...noTarget.routes,
      seat: "leader",
      grants: ADMIN_GRANTS,
    });
    const setTargets = await screen.findByRole("link", {
      name: "Set targets",
    });
    expect(dialsOfLink(setTargets)).toEqual({ scope: `owner:${SELF}` });
    // Not prevented: a modified click is the browser's, a new tab.
    expect(fireEvent.click(setTargets, { metaKey: true })).toBe(true);

    await user.click(setTargets);
    await user.click(await screen.findByRole("button", { name: "Set target" }));
    const dialog = await screen.findByRole("dialog");
    expect(within(dialog).getByText("Riley Rep")).toBeVisible();
    expect(within(dialog).queryByText("DACH")).toBeNull();
  });

  it("opens Report templates as a link for the view's scope", async () => {
    open("#/analytics", answer, TWO);
    await readings();
    const user = userEvent.setup({ delay: null });
    await user.click(screen.getByRole("button", { name: "More" }));
    const templates = await screen.findByRole("link", {
      name: "Report templates",
    });
    expect(dialsOfLink(templates)).toEqual({ scope: `team:${TEAM.id}` });
    expect(fireEvent.click(templates, { ctrlKey: true })).toBe(true);
  });
});
