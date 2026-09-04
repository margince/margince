// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment jsdom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { ToastProvider, ToastRegion } from "../design-system/toast";
import { LocaleProvider } from "../i18n";
import { WorklistScreen } from "./worklist";

// The reader's zone is an input to every moment on this page, so it is injected
// rather than inherited from whatever machine runs the suite: a test that agrees
// with the runner's clock proves nothing about a reader east of it. Berlin
// because its offset is not zero — in UTC a test cannot tell "the viewer's day"
// from "the server's day", which is the distinction the same-day rule turns on.
const viewer = vi.hoisted(() => ({ zone: "Europe/Berlin" }));
vi.mock("../format/timezone", async (importOriginal) => {
  const actual = await importOriginal<typeof import("../format/timezone")>();
  return { ...actual, viewerZone: () => viewer.zone };
});

// The clock a row is racing, drawn.
//
// `due_at` reached the client on meetings and tasks and no worklist file read
// it. A meeting said "starting shortly" — the same three words whether it began
// in four minutes or in fifty — so the one row a rep must open BEFORE a
// wall-clock time was the row that would not say the time. A task said
// "Overdue" and left the reader to find out by how much.
//
// The clock is frozen here. The rule under test is which SIDE of the reader's
// own day the moment falls on, and a test reading the wall clock would assert
// something different every afternoon.

type Worklist = components["schemas"]["Worklist"];
type WorklistItem = components["schemas"]["WorklistItem"];

// The moment the page is read at, fixed so "today" is a stable question.
const NOW = new Date("2026-08-31T09:00:00Z");

function jsonResponse(body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "content-type": "application/json" },
  });
}

function day(queue: WorklistItem[]): Worklist {
  return {
    as_of: NOW.toISOString(),
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

beforeEach(() => {
  // shouldAdvanceTime, because react-query's own timers have to keep running:
  // a frozen clock never resolves the query and every assertion below times out
  // rather than failing on what it meant to check.
  vi.useFakeTimers({ shouldAdvanceTime: true });
  vi.setSystemTime(NOW);
});

afterEach(() => {
  cleanup();
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

// The moment line itself, found by its own class rather than by matching text.
//
// The page's summary says "1 due · ..." in its lead, so a text match for /due /
// finds that line and reports a row saying something it never said. Asking for
// the element the row actually renders is the difference between testing this
// change and testing the header.
function momentLine(): HTMLElement | null {
  return document.querySelector(".worklist-row-when");
}

function meetingAt(dueAt: string): WorklistItem {
  return {
    id: "m1",
    source: "meeting",
    category: "meetings",
    level: 1,
    consequence: "meeting_unprepared",
    title: "Quarterly review with Turbinenbau",
    due_at: dueAt,
    because: [{ kind: "meeting_soon" }],
    actions: [],
  };
}

function taskDue(dueAt: string, overdue: boolean): WorklistItem {
  return {
    id: "t1",
    source: "task",
    category: "tasks",
    level: 3,
    consequence: "task_slips",
    title: "Send the retrofit quote",
    due_at: dueAt,
    overdue,
    because: [{ kind: overdue ? "overdue" : "due_today" }],
    actions: [],
  };
}

describe("a meeting says when it starts", () => {
  // The case the whole change is for: "starting shortly" is the same three
  // words at four minutes and at fifty, and a rep planning a morning around it
  // needs the number.
  it("draws today's start as a wall-clock time", async () => {
    draw([meetingAt("2026-08-31T12:30:00Z")]);

    // 12:30 UTC is 14:30 in Berlin — the VIEWER's clock, not the server's.
    expect(await screen.findByText(/starts 14:30/)).toBeTruthy();
  });

  // A meeting on another day shows the date too. A bare "09:00" on a row two
  // days out is a time the reader acts on this morning, which is the failure
  // this branch exists to avoid rather than a nicety.
  it("draws a later day's start with its date", async () => {
    draw([meetingAt("2026-09-02T07:00:00Z")]);

    await screen.findByText("Quarterly review with Turbinenbau");
    expect(momentLine()?.textContent).toMatch(/02/);
    expect(momentLine()?.textContent).toMatch(/09:00/);
  });

  // The case that tells the VIEWER's day from the server's.
  //
  // 22:30 UTC on the 31st is 00:30 on the 1st in Berlin — tomorrow to the
  // person reading it, still today to a machine in UTC. Every other case here
  // lands on the same calendar day under either rule, so without this one the
  // zone-aware comparison could be swapped for a UTC one and nothing would
  // fail. That is the shape of a test suite that agrees with itself.
  it("reads the day in the reader's zone, not the server's", async () => {
    draw([meetingAt("2026-08-31T22:30:00Z")]);

    await screen.findByText("Quarterly review with Turbinenbau");
    // Dated, because in Berlin this is tomorrow. A bare "00:30" would tell a
    // rep scanning their morning that it starts within the hour.
    expect(momentLine()?.textContent).toMatch(/01/);
    expect(momentLine()?.textContent).toMatch(/00:30/);
  });
});

describe("a task says when it is due", () => {
  it("draws today's due moment as a time", async () => {
    draw([taskDue("2026-08-31T15:00:00Z", false)]);

    expect(await screen.findByText(/due 17:00/)).toBeTruthy();
  });

  // The overdue badge said "Overdue" and stopped. By how long is the fact a rep
  // triages on, and it was on the wire the whole time.
  it("says how far past due an overdue task is, beside the badge", async () => {
    draw([taskDue("2026-08-28T08:00:00Z", true)]);

    expect(await screen.findByText("Overdue")).toBeTruthy();
    expect(momentLine()?.textContent).toMatch(/28/);
  });
});

describe("rows whose date is not a clock the reader owes", () => {
  // An approval's `due_at` is when the staged proposal LAPSES — a fact about
  // the proposal, not a deadline the rep owes. Drawing it as "due" would turn
  // "this offer goes stale" into "you are late", which is the row telling the
  // reader something untrue about their own day.
  it("says nothing about an approval's lapse moment", async () => {
    draw([
      {
        id: "a1",
        source: "approval",
        category: "decisions",
        level: 5,
        consequence: "work_blocked",
        title: "Send the follow-up to Anna Weber",
        due_at: "2026-08-31T16:00:00Z",
        because: [],
        actions: ["decide"],
      },
    ]);

    expect(
      await screen.findByText("Send the follow-up to Anna Weber"),
    ).toBeTruthy();
    expect(momentLine()).toBeNull();
  });

  // And a dated source drawing nothing when the server sent no moment: absent
  // is a real state, not a zero.
  it("says nothing when the row carries no moment at all", async () => {
    const undated = meetingAt("2026-08-31T12:30:00Z");
    undated.due_at = undefined;
    draw([undated]);

    expect(
      await screen.findByText("Quarterly review with Turbinenbau"),
    ).toBeTruthy();
    expect(momentLine()).toBeNull();
  });
});
