/** @vitest-environment jsdom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { LocaleProvider } from "../i18n";
import { WorklistScreen } from "./worklist";

// What the page says about the work it is NOT showing.
//
// The queue is a cut, and a cut that says nothing about the rest lets a full
// first page read as an empty backlog. These are the promises that fixes: a
// pill states what its cut holds, the line under them states what did not fit,
// and neither prints a number the server could not stand behind.

type Worklist = components["schemas"]["Worklist"];
type WorklistCount = components["schemas"]["WorklistCount"];

function jsonResponse(body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "content-type": "application/json" },
  });
}

function stub(day: Worklist) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input instanceof Request ? input.url : input);
      return url.includes("/worklist")
        ? jsonResponse(day)
        : jsonResponse({ data: [] });
    }),
  );
}

function day(counts: WorklistCount[], shown = 1): Worklist {
  return {
    as_of: "2026-08-31T09:00:00Z",
    scope: "mine",
    scope_options: ["mine"],
    queue: Array.from({ length: shown }, (_, i) => ({
      id: `row-${i}`,
      source: "task" as const,
      category: "tasks" as const,
      level: 4,
      consequence: "task_slips" as const,
      title: `Task ${i}`,
      because: [],
      actions: [],
    })),
    summary: { urgent: 0, due: 0, lower_priority: 0, total: shown },
    sources_unavailable: [],
    reach: [],
    counts,
  };
}

function renderWorklist() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <WorklistScreen />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("what the page says about what it is not showing", () => {
  it("counts each cut on the pill that opens it", async () => {
    stub(
      day([
        { category: "tasks", considered: 4, shown: 1, more_available: false },
        {
          category: "decisions",
          considered: 12,
          shown: 0,
          more_available: false,
        },
      ]),
    );
    renderWorklist();

    // The number rides inside the button, so it joins the accessible name: a
    // reader hears "Decisions, 12" rather than meeting a bare figure.
    await waitFor(() => {
      expect(
        screen.getByRole("button", { name: /Decisions.*12/ }),
      ).toBeTruthy();
    });
  });

  it("says how much of the day did not fit", async () => {
    stub(
      day([
        { category: "tasks", considered: 9, shown: 1, more_available: false },
      ]),
    );
    renderWorklist();

    // One row drawn out of nine read. Before this line the reader had no way
    // to tell that page from a finished day.
    await waitFor(() => {
      expect(screen.getByText(/1 of 9 shown/)).toBeTruthy();
    });
  });

  it("draws no line on a day it is carrying whole", async () => {
    stub(
      day([
        { category: "tasks", considered: 1, shown: 1, more_available: false },
      ]),
    );
    renderWorklist();

    await waitFor(() => {
      expect(screen.getByText("Task 0")).toBeTruthy();
    });
    // Nothing was cut, so there is nothing to report. "1 of 1 shown" is noise.
    expect(screen.queryByText(/shown/)).toBeNull();
  });

  it("prints no count for a cut whose source hit its bound", async () => {
    stub(
      day([
        {
          category: "decisions",
          considered: 200,
          shown: 1,
          more_available: true,
        },
      ]),
    );
    renderWorklist();

    await waitFor(() => {
      expect(screen.getByRole("button", { name: /Decisions/ })).toBeTruthy();
    });
    // The server knows a floor, not a total. A floor printed as a count is a
    // wrong number rather than a missing one, so the pill draws none.
    expect(screen.queryByRole("button", { name: /Decisions.*200/ })).toBeNull();
  });

  it("says a source has more rather than claiming a total", async () => {
    stub(
      day([
        {
          category: "decisions",
          considered: 200,
          shown: 1,
          more_available: true,
        },
      ]),
    );
    renderWorklist();

    await waitFor(() => {
      expect(screen.getByText(/1 source(s)? ha(s|ve) more/)).toBeTruthy();
    });
  });
});
