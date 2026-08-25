/** @vitest-environment jsdom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { type Locale, LocaleProvider } from "../i18n";
import { TodayScreen } from "./today";

// The day's surface, and the ways it can mislead the person reading it.

type Attention = components["schemas"]["Attention"];

function jsonResponse(body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "content-type": "application/json" },
  });
}

function stub(day: Attention) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input instanceof Request ? input.url : input);
      if (url.includes("/attention")) {
        return jsonResponse(day);
      }
      return jsonResponse({ data: [] });
    }),
  );
}

function renderToday(locale: Locale = "en") {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial={locale}>
        <TodayScreen />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

const emptyDay: Attention = {
  as_of: "2026-08-25T09:00:00Z",
  needs_you: [],
  planned: [],
  done_for_you: [],
  counts: { needs_you: 0, planned: 0 },
};

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("what the day's surface offers", () => {
  // The defect this catches shipped once: every row that was not a task got a
  // "Decide" button by elimination, so a RECEIPT — a thing already done —
  // offered to be decided again. The server was sending the right actions the
  // whole time; only the rendering was wrong, which is why asserting the
  // payload would not have caught it.
  it("offers no decision on something already done", async () => {
    stub({
      ...emptyDay,
      done_for_you: [
        {
          id: "ap-1",
          source: "approval",
          kind: "close_date_correction",
          title: "Moved the Acme close date to 27 Sep",
          occurred_at: "2026-08-25T08:00:00Z",
          actions: ["open"],
        },
      ],
    });
    renderToday();
    await screen.findByText("Moved the Acme close date to 27 Sep");
    expect(screen.queryByRole("button", { name: "Decide" })).toBeNull();
    expect(screen.getByRole("button", { name: "View" })).toBeTruthy();
  });

  // A queue whose only verbs are "done" and nothing teaches a reader to leave
  // it open. The backend advertises `snooze` on every task; the first version
  // of this screen advertised it and rendered nothing.
  it("offers a task the day after, not only done", async () => {
    stub({
      ...emptyDay,
      planned: [
        {
          id: "t-1",
          source: "task",
          title: "Call Anna about the renewal",
          due_at: "2026-08-25T10:00:00Z",
          actions: ["complete", "snooze"],
        },
      ],
      counts: { needs_you: 0, planned: 1 },
    });
    renderToday();
    await screen.findByText("Call Anna about the renewal");
    expect(screen.getByRole("button", { name: "Tomorrow" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "Done" })).toBeTruthy();
  });

  it("asks for a decision on something staged", async () => {
    stub({
      ...emptyDay,
      needs_you: [
        {
          id: "ap-2",
          source: "approval",
          kind: "send_email",
          title: "Send the follow-up to Anna Weber",
          actions: ["decide"],
        },
      ],
      counts: { needs_you: 1, planned: 0 },
    });
    renderToday();
    await screen.findByText("Send the follow-up to Anna Weber");
    expect(screen.getByRole("button", { name: "Decide" })).toBeTruthy();
  });

  // A duplicate pair carries no server-written sentence, because that sentence
  // has no language on the server. The client writes it — and must, or the row
  // is a blank line with a button beside it.
  it("writes the line for a duplicate pair itself", async () => {
    stub({
      ...emptyDay,
      needs_you: [
        {
          id: "dc-1",
          source: "dedupe_candidate",
          kind: "organization",
          confidence: 0.92,
          actions: ["merge"],
        },
      ],
      counts: { needs_you: 1, planned: 0, duplicates_open: 1 },
    });
    renderToday();
    await screen.findByText("Two companies look like the same one");
    expect(screen.getByText("92% match")).toBeTruthy();
    expect(screen.getByRole("button", { name: "Review" })).toBeTruthy();
  });

  // A lane the reader may not see must SAY so, and the page must stop claiming
  // otherwise.
  //
  // The first version drew a warning banner and then carried on saying "Your
  // day is clear" above a lane reading "Nothing needs a decision" — two
  // reassurances and one warning, and a reader believes the reassuring ones.
  // Asserting only that the warning EXISTS is what let that through.
  it("never claims a clear day it cannot see", async () => {
    stub({ ...emptyDay, lanes_omitted: ["needs_you"] });
    renderToday();
    await screen.findByText("Part of your day is hidden from your account.");
    // The lead no longer claims a clear day, and the withheld lane says why it
    // is empty instead of asserting that it is.
    expect(screen.queryByText("Your day is clear.")).toBeNull();
    expect(screen.getByText("Hidden from your account.")).toBeTruthy();
  });

  // Every figure on this page is a MAGNITUDE — a count of decisions, a count
  // of duplicate pairs, a match percentage — so each one is written in the
  // reader's own notation rather than coerced. A German reader reads "1.234";
  // the coerced form says "1234", which they read as a different number.
  //
  // The compiler holds that these are strings. It cannot hold that they are
  // FORMATTED ones: `String(n)` typechecks and is the right answer for a
  // position — a page ordinal, a step, a version. It is the wrong answer here,
  // and en-GB renders both spellings identically, so only a reader whose
  // notation differs can tell the two apart. Hence German.
  it("writes the day's figures in the reader's own notation", async () => {
    stub({
      ...emptyDay,
      counts: { needs_you: 1234, planned: 0, duplicates_open: 5678 },
      needs_you: [
        {
          id: "dc-1",
          source: "dedupe_candidate",
          kind: "organization",
          confidence: 0.92,
          actions: ["merge"],
        },
      ],
    });
    renderToday("de");
    await screen.findByText("1.234 Entscheidungen warten auf dich.");
    expect(
      screen.getByText("5.678 Dubletten-Paare insgesamt offen"),
    ).toBeTruthy();
    // A percentage is a magnitude too, and below the grouping threshold it
    // reads the same in both notations — asserted so the site is covered by
    // name rather than by being too small to disagree.
    expect(screen.getByText("92% Übereinstimmung")).toBeTruthy();
  });

  // The planned lead is a separate arm of the same function, reachable only
  // when nothing needs deciding — so the test above cannot enter it, and a
  // fourth site would otherwise be ruled by the compiler and by nobody else.
  it("writes the planned figure in the reader's notation too", async () => {
    stub({ ...emptyDay, counts: { needs_you: 0, planned: 4321 } });
    renderToday("de");
    await screen.findByText("Nichts zu entscheiden — 4.321 für heute geplant.");
  });

  it("leads with the count of what actually needs deciding", async () => {
    stub({
      ...emptyDay,
      needs_you: [
        { id: "a", source: "approval", title: "One", actions: ["decide"] },
        { id: "b", source: "approval", title: "Two", actions: ["decide"] },
      ],
      counts: { needs_you: 2, planned: 0 },
    });
    renderToday();
    await screen.findByText("2 decisions are waiting on you.");
  });
});
