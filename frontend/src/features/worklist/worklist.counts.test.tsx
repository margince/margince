/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { LocaleProvider, translate } from "../../i18n";
import { WorklistScreen } from "./worklist";
import { completenessText, pillCount } from "./worklist.completeness";

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
    readings: {
      changed_since_brief: 0,
      revenue_at_risk_minor: null,
      buyer_replies: 0,
      prospecting: 0,
      review: 0,
      more_available: false,
    },
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
    // reader hears "Approvals, 12" rather than meeting a bare figure.
    await waitFor(() => {
      expect(
        screen.getByRole("button", { name: /Approvals.*12/ }),
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
      expect(screen.getByText(/1 of 9 items shown/)).toBeTruthy();
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
      expect(screen.getByRole("button", { name: /Approvals/ })).toBeTruthy();
    });
    // The server knows a floor, not a total. A floor printed as a count is a
    // wrong number rather than a missing one, so the pill draws none.
    expect(screen.queryByRole("button", { name: /Approvals.*200/ })).toBeNull();
  });

  it("does not print a fraction whose denominator is a floor", async () => {
    stub(
      day([
        {
          category: "decisions",
          considered: 200,
          shown: 200,
          more_available: true,
        },
      ]),
    );
    renderWorklist();

    // Waited on the caveat, so the absence below is asserted over a drawn line.
    await waitFor(() => {
      expect(
        screen.getByText(/Approvals \(200\+\) may have more items/),
      ).toBeTruthy();
    });
    // "200 of 200 shown - 1 source has more" contradicts itself in one
    // sentence: the number it divides by is a floor, not a total.
    expect(screen.queryByText(/200 of 200/)).toBeNull();
  });

  it("names the cut whose read hit its limit", async () => {
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
      expect(
        screen.getByText(
          "1 item shown · Approvals (200+) may have more items than this list shows.",
        ),
      ).toBeTruthy();
    });
  });

  it("names every limited cut where several are bounded", async () => {
    stub(
      day([
        {
          category: "decisions",
          considered: 200,
          shown: 1,
          more_available: true,
        },
        {
          category: "tasks",
          considered: 40,
          shown: 2,
          more_available: true,
        },
      ]),
    );
    renderWorklist();

    await waitFor(() => {
      expect(
        screen.getByText(/Approvals \(200\+\), Tasks \(40\+\) may have more/),
      ).toBeTruthy();
    });
  });

  it("names a bounded cut that read nothing without a zero beside it", async () => {
    stub(
      day([
        {
          category: "decisions",
          considered: 200,
          shown: 1,
          more_available: true,
        },
        {
          category: "deals_at_risk",
          considered: 0,
          shown: 0,
          more_available: true,
        },
      ]),
    );
    renderWorklist();

    await waitFor(() => {
      expect(
        screen.getByText(
          "1 item shown · Approvals (200+), Deals at risk may have more items than this list shows.",
        ),
      ).toBeTruthy();
    });
  });

  // A narrowed page reads only the cut the reader is looking at.
  //
  // `considered` is snapshotted before the narrowing, so the other categories
  // keep their full figures and contribute nothing shown. Summing them all
  // answers "5 of 35" on a page showing every one of the five things that were
  // asked for — conflating "you filtered this out" with "this did not fit".
  it("counts only the cut the reader asked for", () => {
    const page = day([
      {
        category: "deals_at_risk",
        considered: 5,
        shown: 5,
        more_available: false,
      },
      { category: "tasks", considered: 30, shown: 0, more_available: false },
    ]);
    const t = ((key: string, vars?: Record<string, string>) =>
      `${key}:${vars?.shown}/${vars?.considered}`) as never;

    const filtered = completenessText(page, "deals_at_risk", t, "en");
    const unfiltered = completenessText(page, "all", t, "en");

    if (filtered !== null) {
      throw new Error(
        `a page showing all five of what was asked for still reported ${filtered}`,
      );
    }
    // Unfiltered, the same day genuinely IS hiding the thirty tasks.
    expect(unfiltered).toContain("5/35");
  });

  // `changed_since_brief` cuts across categories on a per-row freshness the
  // counts do not carry. `except_decisions` excludes by source, not category.
  // Its category complement would drop an introduction request the server keeps.
  // So neither gets a figure; a bounded day still says it may hold more.
  it("gives no figure where the counts cannot describe the narrowing", () => {
    const page = day([
      { category: "tasks", considered: 30, shown: 5, more_available: true },
      { category: "decisions", considered: 4, shown: 4, more_available: false },
    ]);
    const t = ((key: string, vars?: Record<string, string>) =>
      vars ? `${key}:${vars.shown}/${vars.considered}` : key) as never;

    expect(completenessText(page, "changed_since_brief", t, "en")).toBe(
      "worklist.completeness.boundedSome",
    );
    // Not "5/30" either: that is the category complement, which is a DIFFERENT
    // population from the one the server returns.
    expect(completenessText(page, "except_decisions", t, "en")).toBe(
      "worklist.completeness.boundedSome",
    );
    // The control: the same day, asked a question the counts DO answer, speaks.
    expect(completenessText(page, "tasks", t, "en")).toContain("bounded");
  });

  it("says nothing under such a narrowing when no category was bounded", () => {
    const page = day([
      { category: "tasks", considered: 30, shown: 5, more_available: false },
    ]);
    const t = ((key: string) => key) as never;

    expect(completenessText(page, "urgent", t, "en")).toBeNull();
  });
});

// Cards and items are two units: a folded group is one card and many items.
describe("the line compares items with items", () => {
  type Item = Worklist["queue"][number];
  const pairs = (count: number, atLeast = false): Item => ({
    id: "duplicates",
    source: "batch",
    category: "decisions",
    destination: "review",
    level: 6,
    consequence: "data_drifts",
    because: [],
    actions: [],
    batch: { key: "duplicates", count, at_least: atLeast },
  });
  const task = (i: number): Item => ({
    id: `task-${i}`,
    source: "task",
    category: "tasks",
    level: 4,
    consequence: "task_slips",
    title: `Task ${i}`,
    because: [],
    actions: [],
  });
  const t = ((key: string, vars?: Record<string, string>) =>
    translate("en", key as never, vars)) as never;

  it("gives the review panel no exact fraction over a group cut at its limit", async () => {
    const page = day([
      {
        category: "decisions",
        considered: 230,
        shown: 200,
        more_available: true,
      },
    ]);
    page.queue = [pairs(200, true)];
    page.summary.buckets = { urgent: 0, due_today: 0, planned: 0, review: 230 };
    stub(page);
    renderWorklist();

    await waitFor(() => {
      expect(
        screen.getByText(
          /^200\+ shown\. There may be more items than this list shows\.$/,
        ),
      ).toBeTruthy();
    });
    expect(screen.queryByText(/200 of 230/)).toBeNull();
  });

  it("calls eight pairs on one card complete, on the line and in review", async () => {
    const page = day([
      { category: "decisions", considered: 8, shown: 8, more_available: false },
    ]);
    page.queue = [pairs(8)];
    page.summary.buckets = { urgent: 0, due_today: 0, planned: 0, review: 8 };
    stub(page);
    renderWorklist();

    await waitFor(() => {
      expect(screen.getByText(/8 possible duplicates/)).toBeTruthy();
    });
    expect(screen.queryByText(/of 8/)).toBeNull();
  });

  it("states items and cards once a group sits among partly loaded work", () => {
    const page = day([
      { category: "decisions", considered: 8, shown: 8, more_available: false },
      { category: "tasks", considered: 30, shown: 2, more_available: false },
    ]);

    expect(
      completenessText(page, "all", t, "en", [pairs(8), task(1), task(2)]),
    ).toBe("10 of 38 items shown · 3 cards on screen");
  });

  it("prints no fraction over a group cut at its read limit", () => {
    const page = day([
      { category: "decisions", considered: 8, shown: 8, more_available: false },
    ]);

    expect(completenessText(page, "all", t, "en", [pairs(8, true)])).toBe(
      "8 items shown · 1 card on screen",
    );
  });

  it("does not fall silent over a source that did not load", () => {
    const page = day([
      { category: "tasks", considered: 1, shown: 1, more_available: false },
    ]);
    page.sources_unavailable = [
      { source: "task", category: "tasks", reason: "failed" },
      {
        source: "calendar",
        category: "meetings",
        reason: "failed",
        contributes_rows: false,
      },
    ];

    expect(completenessText(page, "all", t, "en", [task(1)])).toBe(
      "1 of 1 items shown · 1 source did not load",
    );
    page.sources_unavailable = page.sources_unavailable.slice(1);
    expect(completenessText(page, "all", t, "en", [task(1)])).toBeNull();
  });
});

it("does not print an exact pill total over a failed or withheld source", () => {
  const page = day([
    { category: "tasks", considered: 0, shown: 0, more_available: false },
  ]);
  for (const reason of ["failed", "withheld"] as const) {
    page.sources_unavailable = [{ source: "task", category: "tasks", reason }];
    expect(pillCount(page, "tasks")).toBeUndefined();
    expect(pillCount(page, "all")).toBeUndefined();
  }
  page.sources_unavailable = [];
  expect(pillCount(page, "tasks")).toBe(0);
});
