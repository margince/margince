/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it } from "vitest";
import type { components } from "../../api/schema";
import { en } from "../../i18n/en";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { day, row } from "../../testing/worklist.testkit";
import { MySdrWork } from "./analytics.outcomes.sdr";
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

function show(
  over: Readonly<{
    summary?: SdrSummary;
    worklist?: () => Response;
  }> = {},
) {
  const asked: URL[] = [];
  installFetchStub({
    "GET /me": meRoute({
      lead: ["read"],
      report_definition: ["read"],
    }),
    "GET /analytics/metrics": () => jsonResponse(reportingSectionsCatalog),
    "GET /analytics/evaluate": () =>
      jsonResponse({ title: "Unavailable", status: 503 }, 503),
    "GET /analytics/sdr-summary": () =>
      jsonResponse(over.summary ?? handoffSummary),
    "GET /analytics/sdr-summary/handoffs": () => jsonResponse(firstPage),
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
    if (url.searchParams.get("cursor") === "cursor-2") {
      return jsonResponse(secondPage);
    }
    return routed(input, init);
  }) as typeof fetch;
  render(
    <StoryProviders>
      <MySdrWork self={SELF} locale="en" />
    </StoryProviders>,
  );
  return asked;
}

describe("MySdrWork", () => {
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

  it("leaves handoffs out without a handoff workflow and shows meetings coming up", async () => {
    const asked = show({
      summary: {
        as_of: AS_OF,
        scope: { kind: "owner", id: SELF, label: "Rep One" },
        workflow: "qualification",
        upcoming_meetings: 2,
      },
    });
    expect(await screen.findByText(en["analytics.sdrNeedsYou"])).toBeVisible();
    await screen.findByText(en["analytics.sdrLeads"]);
    expect(screen.queryByText(en["analytics.sdrWaiting"])).toBeNull();
    expect(
      asked.some((url) => url.pathname.endsWith("/sdr-summary/handoffs")),
    ).toBe(false);
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

  it("says nothing needs you only when every source answered empty", async () => {
    show();
    expect(
      await screen.findByText(en["analytics.sdrNothingNeedsYou"]),
    ).toBeVisible();
  });

  it("shows a failed day as failed, not as a clear one", async () => {
    show({
      worklist: () => jsonResponse({ title: "Unavailable", status: 503 }, 503),
    });
    expect(await screen.findByText(en["state.failed"])).toBeVisible();
    expect(screen.queryByText(en["analytics.sdrNothingNeedsYou"])).toBeNull();
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
