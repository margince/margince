// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment jsdom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { ToastProvider, ToastRegion } from "../design-system/toast";
import { LocaleProvider } from "../i18n";
import { en } from "../i18n/en";
import { WorklistScreen } from "./worklist";

// Why a row is worth the reader's morning, drawn.
//
// Two lanes could state only how LONG something had been true. A lapsed
// relationship said "quiet for 63 days" whether it was the rep's strongest
// contact with an open deal behind them or a cc who had drifted, and a task
// nobody had taken read exactly like the reader's own. Both facts were already
// in the server's hand and dropped on the way out.
//
// These assert the drawn phrase rather than the wire field: a reason that
// reaches the client and has no sentence for it is silently omitted, so a test
// on the payload would pass over a row that says nothing.

type Worklist = components["schemas"]["Worklist"];
type WorklistItem = components["schemas"]["WorklistItem"];

function jsonResponse(body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "content-type": "application/json" },
  });
}

function day(queue: WorklistItem[]): Worklist {
  return {
    as_of: "2026-08-31T09:00:00Z",
    scope: "mine",
    scope_options: ["mine"],
    queue,
    summary: {
      urgent: 0,
      due: queue.length,
      lower_priority: 0,
      total: queue.length,
    },
    sources_unavailable: [],
    readings: {
      revenue_at_risk_minor: null,
      buyer_replies: 0,
      prospecting: 0,
      review: 0,
      more_available: false,
    },
    reach: [],
    counts: [],
  };
}

function draw(queue: WorklistItem[]) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => jsonResponse(day(queue))),
  );
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <ToastProvider>
          <WorklistScreen />
          <ToastRegion />
        </ToastProvider>
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function decayRow(because: WorklistItem["because"]): WorklistItem {
  return {
    id: "d1",
    source: "relationship_decay",
    category: "system",
    level: 3,
    consequence: "data_drifts",
    title: "Dana Weiss",
    because,
    actions: [],
  };
}

describe("a lapsed relationship says what it was worth", () => {
  it("says an open deal rests on the contact", async () => {
    draw([
      decayRow([
        { kind: "quiet_days", value: { kind: "days", days: 63 } },
        { kind: "expected_revenue" },
      ]),
    ]);

    expect(await screen.findByText(/an open deal rests on this/)).toBeTruthy();
    // Beside the span, not instead of it: the rep reads both — how long, and
    // why it matters that long.
    expect(await screen.findByText(/quiet for 63 days/)).toBeTruthy();
  });

  // The row that carries nothing says only how long. This is what makes the
  // line above mean something: if every decay row claimed a deal, the claim
  // would be decoration rather than a fact.
  it("claims no deal when none rests on the contact", async () => {
    draw([
      decayRow([{ kind: "quiet_days", value: { kind: "days", days: 63 } }]),
    ]);

    expect(await screen.findByText(/quiet for 63 days/)).toBeTruthy();
    expect(screen.queryByText(/an open deal rests on this/)).toBeNull();
  });
});

describe("a task says who holds it", () => {
  it("says nobody owns the one nobody has taken", async () => {
    draw([
      {
        id: "t1",
        source: "task",
        category: "tasks",
        level: 3,
        consequence: "task_slips",
        title: "Send the retrofit quote",
        because: [{ kind: "due_today" }, { kind: "unassigned" }],
        actions: [],
      },
    ]);

    expect(await screen.findByText(/nobody owns it/)).toBeTruthy();
  });

  it("says nothing about ownership on a task somebody holds", async () => {
    draw([
      {
        id: "t2",
        source: "task",
        category: "tasks",
        level: 3,
        consequence: "task_slips",
        title: "Send the retrofit quote",
        because: [{ kind: "due_today" }],
        actions: [],
      },
    ]);

    expect(await screen.findByText("Send the retrofit quote")).toBeTruthy();
    expect(screen.queryByText(/nobody owns it/)).toBeNull();
  });
});

// A row states its deadline ONCE.
//
// `when` prints the moment — "due 06.07.2026, 15:00" — and `due_today` is that
// same clock in a coarser register. Drawn together the row said the deadline
// twice, one line under the other, which reads as two findings about one clock
// and cost a 19px line on the screen with none to spare.
//
// Whichever of the two survives has to be the MOMENT: it names the hour a rep
// is racing, where the phrase names only the day.
describe("a row says its deadline once", () => {
  function dueRow(overrides: Partial<WorklistItem> = {}): WorklistItem {
    return {
      id: "t1",
      source: "task",
      category: "tasks",
      level: 2,
      consequence: "task_slips",
      title: "Send the retrofit quote",
      due_at: "2026-08-31T15:00:00Z",
      because: [{ kind: "due_today" }, { kind: "unassigned" }],
      actions: [],
      ...overrides,
    };
  }

  it("drops the due-today phrase where the moment is drawn", async () => {
    const { container } = draw([dueRow()]);

    // The moment survives, carrying the hour. Matched as "due <something>"
    // rather than a literal clock time: the moment is rendered in the
    // installation's own zone, so a hard-coded hour would assert the suite's
    // timezone rather than that the line was drawn.
    const when = await screen.findByText(/^due \d/);
    expect(when.className).toContain("worklist-row-when");

    // The coarser phrase is gone, so the clock is stated once. Read from the
    // because line itself: "due today" as a page-wide query would also match
    // the summary sentence above the queue.
    expect(
      container.querySelector(".worklist-row-because")?.textContent,
    ).not.toMatch(/due today/i);
  });

  // The other reasons on the row are untouched: this drops ONE duplicated
  // fact, not the line it sat in.
  it("keeps every other reason beside the moment", async () => {
    const { container } = draw([dueRow()]);

    await screen.findByText(/^due \d/);
    expect(
      container.querySelector(".worklist-row-because")?.textContent,
    ).toMatch(/nobody owns it/i);
  });

  // OVERDUE is the same duplicate said a THIRD time. It sits beside a danger
  // badge that already says "Overdue" and above a moment that says the hour, so
  // the phrase is the one register a reader gains nothing from. It reaches the
  // row from the other arm of the very `if` that sends `due_today`, both
  // guarded by the same `due_at` the moment is drawn from — a rule naming one
  // of them names half a condition.
  it("drops the overdue phrase where the moment is drawn", async () => {
    const { container } = draw([
      dueRow({ because: [{ kind: "overdue" }, { kind: "unassigned" }] }),
    ]);

    await screen.findByText(/^due \d/);
    const because = container.querySelector(
      ".worklist-row-because",
    )?.textContent;
    expect(because).not.toMatch(/overdue/i);
    expect(because).toMatch(/nobody owns it/i);
  });

  // MEETING_SOON has no non-duplicating case at all: it fires only where the
  // meeting carries a `due_at`, which is exactly when its own when line draws
  // "starts 14:12".
  it("drops the starting-shortly phrase where the moment is drawn", async () => {
    const { container } = draw([
      dueRow({
        id: "m1",
        source: "meeting",
        category: "meetings",
        consequence: "meeting_unprepared",
        title: "Quarterly review",
        because: [{ kind: "meeting_soon" }],
      }),
    ]);

    await screen.findByText(/^starts \d/);
    expect(container.querySelector(".worklist-row-because")).toBeNull();
  });

  // A DEADLINE REASON WHOSE MOMENT IS NEVER DRAWN KEEPS ITS PHRASE.
  //
  // It holds the `whenDrawn` GUARD, not the membership of the set. `closing_soon`
  // rides on `deal_at_risk`, a source whenKeyFor answers null for, so the guard
  // is false and the set is never consulted — this row keeps its phrase whether
  // or not somebody adds `closing_soon` to it. That is the behaviour the rule
  // wants and it is worth saying which half the test can see: the guard is what
  // stops a source with no moment losing its only statement of the fact, and it
  // is proven by removing the guard, which is what the approval row below
  // catches.
  //
  // What the set's membership costs is checked one test up, where a source that
  // DOES draw a moment must lose the duplicate.
  it("keeps a deadline phrase whose source draws no moment", async () => {
    const { container } = draw([
      dueRow({
        id: "d1",
        source: "deal_at_risk",
        category: "deals_at_risk",
        consequence: "deal_drifts",
        title: "Turbinenbau retrofit",
        because: [{ kind: "closing_soon" }],
      }),
    ]);

    await screen.findByText(/Turbinenbau retrofit/);

    // No moment is drawn for this source, so the phrase is the only place the
    // fact is said.
    expect(container.querySelector(".worklist-row-when")).toBeNull();
    // The PHRASE ITSELF, not merely a non-empty line: a row carries other
    // reasons, so asserting the line has text would pass just as happily with
    // this fact dropped.
    expect(
      container.querySelector(".worklist-row-because")?.textContent,
    ).toMatch(new RegExp(en["worklist.because.closing_soon"]));
  });

  // WHERE THE MOMENT IS REFUSED THE PHRASE STAYS. An approval's `due_at` is
  // when the proposal lapses — a fact about the staged work rather than a
  // deadline the rep owes — so the when line draws nothing for it. Dropping the
  // phrase there too would take the fact off the row entirely rather than say
  // it once.
  it("keeps the due-today phrase on a row whose moment is not drawn", async () => {
    const { container } = draw([
      dueRow({
        id: "a1",
        source: "approval",
        category: "decisions",
        title: "A proposal waiting on you",
      }),
    ]);

    await screen.findByText(/A proposal waiting on you/);
    expect(
      container.querySelector(".worklist-row-because")?.textContent,
    ).toMatch(/due today/i);
  });
});
