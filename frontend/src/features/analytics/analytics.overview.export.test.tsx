/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";
import { cleanup, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { forgetSeat } from "../../lib/storage";
import {
  isSetup,
  OVERVIEW_SCENARIOS,
  overviewAddress,
} from "./analytics.overview.scenarios.fixtures";
import { openOverview, readings } from "./analytics.overview.testkit";

const saved = vi.hoisted(() => ({ files: [] as string[] }));
vi.mock("../../lib/download", async (actual) => ({
  ...(await actual<typeof import("../../lib/download")>()),
  downloadBytes: async (bytes: Blob) => {
    saved.files.push(await bytes.text());
  },
}));

const originalFetch = globalThis.fetch;
beforeEach(() => {
  forgetSeat();
  saved.files = [];
});
afterEach(() => {
  cleanup();
  globalThis.fetch = originalFetch;
  window.location.hash = "";
});

const HEADER =
  "metric,block,label,value,target,comparison,upper,unit,currency,context,coverage,captured_at";

// The server's CSV of one evaluation, as reportingCSVRows writes it: the
// figures, then a row per chart point. A label with a comma is quoted.
function serverCsv(url: URL): string {
  const rows = [HEADER];
  for (const metric of url.searchParams.getAll("metrics"))
    rows.push(`${metric},headline,"${metric}, read",1,,,,count,,interval,ok,`);
  for (const block of url.searchParams.getAll("blocks"))
    rows.push(
      `bookings_won,${block},"Point, ${block}",2,,,,count,,interval,ok,`,
    );
  return `${rows.join("\n")}\n`;
}

describe("Sales review's export", () => {
  it("holds the figures once, then the three board charts", async () => {
    const user = userEvent.setup({ delay: null });
    const pair = OVERVIEW_SCENARIOS.populated.views.sales;
    if (!isSetup(pair)) throw new Error("populated draws Sales review");
    const csvAsked: URL[] = [];
    // The server echoes the selection it was asked, charts included.
    const echoed = (url: URL) => {
      const evaluation = pair.answer(url);
      const asked = <T,>(name: string) => url.searchParams.getAll(name) as T[];
      return {
        ...evaluation,
        selection: {
          ...evaluation.selection,
          metrics:
            asked<(typeof evaluation.selection.metrics)[number]>("metrics"),
          blocks: asked<(typeof evaluation.selection.blocks)[number]>("blocks"),
        },
      };
    };
    openOverview(overviewAddress("sales"), echoed, pair.routes);
    const app = globalThis.fetch;
    globalThis.fetch = (async (
      input: RequestInfo | URL,
      init?: RequestInit,
    ) => {
      const url = new URL(
        input instanceof Request ? input.url : String(input),
        "http://localhost",
      );
      if (!url.pathname.endsWith("/analytics/evaluate.csv"))
        return app(input, init);
      csvAsked.push(url);
      return new Response(serverCsv(url), {
        headers: { "content-type": "text/csv" },
      });
    }) as typeof fetch;
    await readings();
    await screen.findByRole("heading", { name: "Open pipeline by stage" });

    await user.click(screen.getByRole("button", { name: "More" }));
    await user.click(await screen.findByRole("button", { name: "Export CSV" }));

    await waitFor(() => expect(saved.files).toHaveLength(1));
    const rows = (saved.files[0] ?? "").trim().split("\n");
    const blocks = rows.slice(1).map((row) => row.split(",")[1]);
    expect(rows[0]).toBe(HEADER);
    expect(blocks.filter((block) => block === "headline").length).toBe(
      csvAsked[0]?.searchParams.getAll("metrics").length,
    );
    expect(blocks.filter((block) => block !== "headline")).toEqual([
      "bookings_trend",
      "stage_distribution",
      "owner_attainment",
    ]);
    // Each read is exported under its own receipt.
    expect(
      csvAsked.map((url) => url.searchParams.has("evaluation_key")),
    ).toEqual([true, true]);
  });
});
