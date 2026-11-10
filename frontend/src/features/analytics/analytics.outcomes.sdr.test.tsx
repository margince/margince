/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import {
  act,
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { en } from "../../i18n/en";
import {
  REPORTING_FIXTURE_ZONE,
  reportingStoryEvaluation,
} from "../../testing/reporting.fixtures";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { day, row } from "../../testing/worklist.testkit";
import { EVALUATION_WAIT_MS, MySdrWork } from "./analytics.outcomes.sdr";
import { reportingSectionsCatalog } from "./reporting.story-fixtures";

type SdrSummary = components["schemas"]["SdrSummary"];
type Page = components["schemas"]["SdrWaitingHandoffPage"];

const originalFetch = globalThis.fetch;
afterEach(() => {
  cleanup();
  globalThis.fetch = originalFetch;
});

const SELF = "u-rep-1";
const AS_OF = "2026-09-21T12:00:00Z";
// Names for the copies of the view a test mounts side by side.
const VIEW_NAMES = ["first", "second"] as const;

const handoffSummary: SdrSummary = {
  as_of: AS_OF,
  scope: { kind: "owner", id: SELF, label: "Rep One" },
  workflow: "handoff",
  handoffs: {
    waiting: 3,
    unassigned: 1,
    blocked: 1,
    returned: 0,
    oldest_waiting_since: "2026-09-18T12:00:00Z",
  },
  upcoming_meetings: 2,
};

const firstPage: Page = {
  as_of: AS_OF,
  rows: [
    {
      id: "h-1",
      waiting_since: "2026-09-18T12:00:00Z",
      resubmitted: false,
      restricted: false,
      subject: { type: "contact", id: "c-1", label: "Jamie Buyer" },
      recipient: { id: "u-ae", label: "Alex Seller", active: false },
    },
    {
      id: "h-2",
      waiting_since: "2026-09-20T12:00:00Z",
      resubmitted: false,
      restricted: true,
    },
  ],
  next_cursor: "cursor-2",
};

const secondPage: Page = {
  as_of: AS_OF,
  rows: [
    {
      id: "h-3",
      waiting_since: "2026-09-21T09:00:00Z",
      resubmitted: true,
      restricted: false,
      subject: { type: "lead", id: "l-1", label: "Inbound lead" },
    },
  ],
};

const WINDOW = {
  start_at: AS_OF,
  end_at: "2026-10-04T22:00:00Z",
  timezone: REPORTING_FIXTURE_ZONE,
};

const returnedPage: Page = {
  as_of: AS_OF,
  rows: [
    {
      id: "h-9",
      waiting_since: "2026-09-19T12:00:00Z",
      resubmitted: false,
      restricted: false,
      subject: { type: "lead", id: "l-9", label: "Returned lead" },
      reason: "Budget not confirmed",
    },
  ],
};

function show(
  over: Readonly<{
    summary?: SdrSummary | "failed";
    worklist?: () => Response;
    evaluation?: unknown;
    evaluationHeld?: Promise<unknown>;
    // Answers a request before the routes do, for a test that reads its URL.
    intercept?: (url: URL) => Response | Promise<Response> | undefined;
    // How many copies of the view share one client, as two panels on a page do.
    views?: number;
  }> = {},
) {
  const asked: URL[] = [];
  installFetchStub({
    "GET /me": meRoute({
      lead: ["read"],
      report_definition: ["read"],
    }),
    "GET /analytics/metrics": () => jsonResponse(reportingSectionsCatalog),
    "GET /analytics/evaluate": async () => {
      await over.evaluationHeld;
      return over.evaluation === undefined
        ? jsonResponse({ title: "Unavailable", status: 503 }, 503)
        : jsonResponse(over.evaluation);
    },
    "GET /analytics/sdr-summary": () =>
      over.summary === "failed"
        ? jsonResponse({ title: "Unavailable", status: 503 }, 503)
        : jsonResponse(over.summary ?? handoffSummary),
    "GET /analytics/sdr-summary/handoffs": () => jsonResponse(firstPage),
    "GET /analytics/sdr-summary/meetings": () =>
      jsonResponse({ as_of: AS_OF, window: WINDOW, rows: [] }),
    "GET /worklist": over.worklist ?? (() => jsonResponse(day())),
  });
  // The stub keys on the path alone, so the cursor's page is answered here.
  // Every URL is kept to say what was asked.
  const routed = globalThis.fetch;
  globalThis.fetch = (async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = new URL(
      input instanceof Request ? input.url : String(input),
      "https://storybook.local",
    );
    asked.push(url);
    const intercepted = over.intercept?.(url);
    if (intercepted) {
      return intercepted;
    }
    if (url.searchParams.get("state") === "returned") {
      return jsonResponse(returnedPage);
    }
    if (url.searchParams.get("cursor") === "cursor-2") {
      return jsonResponse(secondPage);
    }
    return routed(input, init);
  }) as typeof fetch;
  render(
    <StoryProviders>
      {VIEW_NAMES.slice(0, over.views ?? 1).map((view) => (
        <MySdrWork key={view} self={SELF} locale="en" />
      ))}
    </StoryProviders>,
  );
  return asked;
}

describe("MySdrWork", () => {
  it("reads the summary at the instant its lead readings were evaluated", async () => {
    const asked = show({ evaluation: reportingStoryEvaluation });
    const isSummary = (url: URL) =>
      url.pathname.endsWith("/analytics/sdr-summary");
    await waitFor(() => expect(asked.some(isSummary)).toBe(true));
    const summary = asked.find(isSummary);
    expect(summary?.searchParams.get("as_of")).toBe(
      reportingStoryEvaluation.context.evaluated_at,
    );
  });

  it("re-pins once when its instant is too old, and never asks the stale summary again", async () => {
    const stale = "2026-09-21T11:30:00Z";
    const fresh = reportingStoryEvaluation.context.evaluated_at;
    const refresh = heldOpen();
    let evaluations = 0;
    const asked = show({
      intercept: (url) => {
        if (url.pathname.endsWith("/analytics/evaluate")) {
          evaluations += 1;
          return evaluations === 1
            ? evaluatedAt(stale)
            : refresh.held.then(() => evaluatedAt(fresh));
        }
        if (isSummaryAt(url, stale)) {
          return asOfRefused("too_old");
        }
        return echoedSummary(url);
      },
    });
    await waitFor(() => expect(evaluations).toBe(2));
    await settle();
    expect(asked.filter((url) => isSummaryAt(url, stale))).toHaveLength(1);
    refresh.release();
    expect(
      await screen.findByText(/Alex Seller can no longer act/),
    ).toBeVisible();
    expect(asked.some((url) => isSummaryAt(url, fresh))).toBe(true);
    expect(asked.filter((url) => isSummaryAt(url, stale))).toHaveLength(1);
    expect(evaluations).toBe(2);
  });

  it("re-pins a refused instant once when two views read it", async () => {
    const stale = "2026-09-21T11:30:00Z";
    const refresh = heldOpen();
    let evaluations = 0;
    show({
      views: 2,
      intercept: (url) => {
        if (url.pathname.endsWith("/analytics/evaluate")) {
          evaluations += 1;
          return evaluations === 1
            ? evaluatedAt(stale)
            : refresh.held.then(() => evaluatedAt(stale));
        }
        if (isSummaryAt(url, stale)) {
          return asOfRefused("too_old");
        }
        return echoedSummary(url);
      },
    });
    await waitFor(() => expect(evaluations).toBe(2));
    await settle();
    expect(evaluations).toBe(2);
    refresh.release();
  });

  it("re-pins once when a later page crosses the window, and never asks that page again", async () => {
    const first = reportingStoryEvaluation.context.evaluated_at;
    const second = "2026-09-21T12:20:00Z";
    const refresh = heldOpen();
    let evaluations = 0;
    const isLaterPageAt = (url: URL, at: string) =>
      url.pathname.endsWith("/analytics/sdr-summary/handoffs") &&
      url.searchParams.get("cursor") === "cursor-2" &&
      url.searchParams.get("as_of") === at;
    const asked = show({
      intercept: (url) => {
        if (url.pathname.endsWith("/analytics/evaluate")) {
          evaluations += 1;
          return evaluations === 1
            ? evaluatedAt(first)
            : refresh.held.then(() => evaluatedAt(second));
        }
        if (isLaterPageAt(url, first)) {
          return asOfRefused("too_old");
        }
        return echoedSummary(url);
      },
    });
    await userEvent.click(
      await screen.findByRole("button", { name: en["analytics.sdrShowMore"] }),
    );
    await waitFor(() => expect(evaluations).toBe(2));
    await settle();
    expect(asked.filter((url) => isLaterPageAt(url, first))).toHaveLength(1);
    expect(asked.filter((url) => isSummaryAt(url, first))).toHaveLength(1);
    refresh.release();
    await waitFor(() =>
      expect(
        asked.some(
          (url) =>
            url.pathname.endsWith("/analytics/sdr-summary/handoffs") &&
            url.searchParams.get("as_of") === second,
        ),
      ).toBe(true),
    );
    expect(asked.filter((url) => isLaterPageAt(url, first))).toHaveLength(1);
    expect(evaluations).toBe(2);
  });

  it("re-pins once when the server's clock says its instant is in the future", async () => {
    const ahead = "2026-09-21T12:00:05Z";
    const fresh = reportingStoryEvaluation.context.evaluated_at;
    let evaluations = 0;
    show({
      intercept: (url) => {
        if (url.pathname.endsWith("/analytics/evaluate")) {
          evaluations += 1;
          return evaluatedAt(evaluations === 1 ? ahead : fresh);
        }
        if (isSummaryAt(url, ahead)) {
          return asOfRefused("future");
        }
        return echoedSummary(url);
      },
    });
    expect(
      await screen.findByText(/Alex Seller can no longer act/),
    ).toBeVisible();
    expect(evaluations).toBe(2);
  });

  it("does not read again on a refusal a fresh instant does not cure", async () => {
    let evaluations = 0;
    show({
      intercept: (url) => {
        if (url.pathname.endsWith("/analytics/evaluate")) {
          evaluations += 1;
          return jsonResponse(reportingStoryEvaluation);
        }
        if (url.pathname.endsWith("/analytics/sdr-summary")) {
          return asOfRefused("invalid");
        }
        return undefined;
      },
    });
    const panel = (await screen.findByText(en["analytics.sdrWaiting"])).closest(
      "section",
    );
    expect(
      await within(panel as HTMLElement).findByText(en["state.failed"]),
    ).toBeVisible();
    await settle();
    expect(evaluations).toBe(1);
  });

  it("still reads the summary, at now, when the lead readings failed", async () => {
    const asked = show();
    const isSummary = (url: URL) =>
      url.pathname.endsWith("/analytics/sdr-summary");
    await waitFor(() => expect(asked.some(isSummary)).toBe(true));
    expect(asked.find(isSummary)?.searchParams.has("as_of")).toBe(false);
  });

  it("stops waiting for a stalled evaluation, then moves to its instant", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    try {
      let release = () => {};
      const evaluationHeld = new Promise<void>((resolve) => {
        release = resolve;
      });
      const asked = show({
        evaluation: reportingStoryEvaluation,
        evaluationHeld,
      });
      const summaries = () =>
        asked.filter((url) => url.pathname.endsWith("/analytics/sdr-summary"));
      await waitFor(() =>
        expect(
          asked.some((url) => url.pathname.endsWith("/analytics/evaluate")),
        ).toBe(true),
      );
      expect(summaries()).toEqual([]);

      await act(() => vi.advanceTimersByTimeAsync(EVALUATION_WAIT_MS));
      await waitFor(() => expect(summaries()).toHaveLength(1));
      expect(summaries()[0]?.searchParams.has("as_of")).toBe(false);

      release();
      await waitFor(() => expect(summaries()).toHaveLength(2));
      expect(summaries()[1]?.searchParams.get("as_of")).toBe(
        reportingStoryEvaluation.context.evaluated_at,
      );
    } finally {
      vi.useRealTimers();
    }
  });

  it("names a blocked recipient and a restricted prospect, and pages on", async () => {
    const asked = show();
    expect(await screen.findByText(en["analytics.sdrWaiting"])).toBeVisible();
    expect(
      await screen.findByText(/Alex Seller can no longer act/),
    ).toBeVisible();
    expect(
      screen.getByText(en["analytics.sdrRestrictedProspect"]),
    ).toBeVisible();
    await userEvent.click(
      screen.getByRole("button", { name: en["analytics.sdrShowMore"] }),
    );
    expect(
      await screen.findByText(/since you submitted it again/),
    ).toBeVisible();
    const pages = asked.filter((url) =>
      url.pathname.endsWith("/analytics/sdr-summary/handoffs"),
    );
    expect(pages.map((url) => url.searchParams.get("cursor"))).toEqual([
      null,
      "cursor-2",
    ]);
    expect(
      screen.queryByRole("button", { name: en["analytics.sdrShowMore"] }),
    ).toBeNull();
  });

  it("lists a handoff sent back with the reason the recipient gave", async () => {
    show({
      summary: {
        ...handoffSummary,
        handoffs: {
          waiting: 3,
          unassigned: 1,
          blocked: 1,
          returned: 1,
          oldest_waiting_since: "2026-09-18T12:00:00Z",
        },
      },
    });
    expect(await screen.findByText("Returned lead")).toBeVisible();
    expect(
      screen.getByText(/^Sent back .* ago · Budget not confirmed$/),
    ).toBeVisible();
  });

  it("marks a waiting handoff whose recipient cannot act", async () => {
    show();
    expect(
      await screen.findByText(en["analytics.prospect.blocked"]),
    ).toBeVisible();
  });

  it("leaves handoffs out without a handoff workflow and shows meetings coming up", async () => {
    const asked = show({
      summary: {
        as_of: AS_OF,
        scope: { kind: "owner", id: SELF, label: "Rep One" },
        workflow: "qualification",
        upcoming_meetings: 2,
      },
    });
    expect(screen.queryByText(en["analytics.sdrNothingNeedsYou"])).toBeNull();
    await screen.findByText(en["analytics.sdrLeads"]);
    expect(screen.queryByText(en["analytics.sdrWaiting"])).toBeNull();
    expect(
      asked.some((url) => url.pathname.endsWith("/sdr-summary/handoffs")),
    ).toBe(false);
  });

  it("shows a failed summary as failed, never as a workspace without handoffs", async () => {
    show({ summary: "failed" });
    expect(await screen.findByText(en["analytics.sdrWaiting"])).toBeVisible();
    const panel = screen
      .getByText(en["analytics.sdrWaiting"])
      .closest("section");
    expect(panel).not.toBeNull();
    expect(
      await within(panel as HTMLElement).findByText(en["state.failed"]),
    ).toBeVisible();
    expect(screen.queryByText(en["analytics.prospect.upcoming"])).toBeNull();
  });

  it("never says nothing needs you while a source failed", async () => {
    show({
      worklist: () =>
        jsonResponse(
          day({ sources_unavailable: [{ source: "tasks", reason: "failed" }] }),
        ),
    });
    expect(
      await screen.findByText(en["analytics.sdrWorkIncomplete"]),
    ).toBeVisible();
    expect(screen.queryByText(en["analytics.sdrNothingNeedsYou"])).toBeNull();
  });

  it("omits a work panel and an assurance when every source answered empty", async () => {
    show();
    await screen.findByText(en["analytics.sdrLeads"]);
    expect(screen.queryByText(en["analytics.sdrNothingNeedsYou"])).toBeNull();
  });

  it("shows a failed day as failed, not as a clear one", async () => {
    show({
      worklist: () => jsonResponse({ title: "Unavailable", status: 503 }, 503),
    });
    expect(await screen.findByText(en["state.failed"])).toBeVisible();
    expect(screen.queryByText(en["analytics.sdrNothingNeedsYou"])).toBeNull();
  });

  it("opens a work row's own record, with its reason and its clock", async () => {
    show({
      worklist: () =>
        jsonResponse(
          day({
            queue: [
              row({
                id: "handoff-review",
                title: "Review the handoff for Kestrel Fleet",
                subject: { type: "lead", id: "l-kestrel", label: "Kestrel" },
                due_at: "2026-09-21T09:00:00Z",
                overdue: true,
                because: [{ kind: "overdue" }],
                primary_action: "open",
                actions: ["open"],
              }),
            ],
            summary: { urgent: 1, due: 1, lower_priority: 0, total: 1 },
          }),
        ),
    });
    const open = await screen.findByRole("link", {
      name: /^Open Review the handoff for Kestrel Fleet/,
    });
    expect(open.getAttribute("href")).toContain("/leads/l-kestrel");
    expect(open.getAttribute("href")).not.toContain("companies");
    expect(screen.getByText(en["worklist.overdue"])).toBeVisible();
  });

  it("names the Worklist's top items in its own order", async () => {
    show({
      worklist: () =>
        jsonResponse(
          day({
            queue: [
              row({ id: "a", title: "Reply to Jamie" }),
              row({ id: "b", title: "Call Priya" }),
            ],
            summary: { urgent: 1, due: 1, lower_priority: 0, total: 2 },
          }),
        ),
    });
    const first = await screen.findByText("Reply to Jamie");
    const second = screen.getByText("Call Priya");
    expect(
      first.compareDocumentPosition(second) & Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
  });
});

// The 422 the server answers when it refuses an SDR read's instant.
function asOfRefused(code: string): Response {
  return jsonResponse(
    {
      status: 422,
      code: "validation_error",
      detail: "choose another instant",
      details: {
        errors: [{ field: "as_of", code, message: "choose another instant" }],
      },
    },
    422,
  );
}

// A response the test holds back until it says, as a slow network would.
function heldOpen() {
  let release = () => {};
  const held = new Promise<void>((resolve) => {
    release = resolve;
  });
  return { held, release: () => release() };
}

// Lets every queued fetch and effect run, so a loop has room to show itself.
async function settle() {
  for (let turn = 0; turn < 10; turn += 1) {
    await new Promise((resolve) => setTimeout(resolve, 0));
  }
}

function evaluatedAt(at: string): Response {
  return jsonResponse({
    ...reportingStoryEvaluation,
    context: { ...reportingStoryEvaluation.context, evaluated_at: at },
  });
}

function isSummaryAt(url: URL, at: string): boolean {
  return (
    url.pathname.endsWith("/analytics/sdr-summary") &&
    url.searchParams.get("as_of") === at
  );
}

// The summary as the server answers it: at the instant it was asked for.
function echoedSummary(url: URL): Response | undefined {
  if (!url.pathname.endsWith("/analytics/sdr-summary")) {
    return undefined;
  }
  return jsonResponse({
    ...handoffSummary,
    as_of: url.searchParams.get("as_of") ?? AS_OF,
  });
}
