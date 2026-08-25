/** @vitest-environment jsdom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
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

// The day, plus the one approval the decision lane fetches whole.
//
// The feed sends a lane's worth of each item; deciding a staged proposal needs
// the rest of it, so the lane reads the single approval it is showing. A test
// that served only the feed would render a card stuck loading forever.
function stub(day: Attention, approval?: unknown, brief?: unknown) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input instanceof Request ? input.url : input);
      if (url.includes("/attention")) {
        return jsonResponse(day);
      }
      // The briefing lane takes its ids from the feed and each item's CONTENT
      // from the brief's own read, so a test that shows the lane serves both.
      if (brief && /\/brief$/.test(url.split("?")[0])) {
        return jsonResponse(brief);
      }
      if (approval && /\/approvals\/[^/]+$/.test(url.split("?")[0])) {
        return jsonResponse(approval);
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
  this_morning: [],
  needs_you: [],
  planned: [],
  done_for_you: [],
  counts: { this_morning: 0, needs_you: 0, planned: 0 },
};

afterEach(() => {
  // Unmount as well as unstub. Vitest does not clear the DOM between tests
  // unless it is told to, so a second render puts TWO of the same card on the
  // page and every query by role finds both — which reads as "the component
  // rendered twice" rather than "the last test is still on screen".
  cleanup();
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
    // A receipt reports a finished act, so it carries no verb at all: there is
    // nothing to decide, and nothing to complete.
    expect(screen.queryByRole("button", { name: "Decide" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Merge them" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Done" })).toBeNull();
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
      counts: { this_morning: 0, needs_you: 0, planned: 1 },
    });
    renderToday();
    await screen.findByText("Call Anna about the renewal");
    expect(screen.getByRole("button", { name: "Tomorrow" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "Done" })).toBeTruthy();
  });

  // The decision lane shows ONE decision at a time and draws it whole, so a
  // staged proposal arrives as the same row the record surfaces draw — with the
  // verbs that answer it, not a link to somewhere it can be answered.
  it("asks for a decision on something staged", async () => {
    stub(
      {
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
        counts: { this_morning: 0, needs_you: 1, planned: 0 },
      },
      // The whole approval, as `GET /approvals/{id}` answers it. Every field
      // the contract marks required is here on purpose: a stub that omits one
      // is testing a payload the server cannot send.
      {
        id: "ap-2",
        kind: "send_email",
        status: "pending",
        summary: "Send the follow-up to Anna Weber",
        proposed_by: "agent:runner",
        created_at: "2026-08-25T08:00:00Z",
        proposed_change: {},
        target_entity_type: "person",
        target_entity_id: "11111111-1111-4111-8111-111111111111",
      },
    );
    renderToday();
    // The verb that ANSWERS it, on the page the reader is already on.
    expect(await screen.findByRole("button", { name: "Accept" })).toBeTruthy();
  });

  // A duplicate is DECIDED here, not described here.
  //
  // The surface this replaced said "two companies look like the same one, 92%
  // match" and sent the reader to another screen to find out which two — so the
  // percentage was the whole of what they were given, and a percentage is not
  // something a person can answer. Both records are named, and the verb that
  // merges them is on the card.
  it("names both records and merges them in place", async () => {
    stub({
      ...emptyDay,
      needs_you: [
        {
          id: "dc-1",
          source: "dedupe_candidate",
          kind: "organization",
          confidence: 0.92,
          actions: ["merge"],
          pair: {
            left: {
              id: "org-1",
              label: "Acme Logistik GmbH",
              detail: "acme.example",
            },
            right: {
              id: "org-2",
              label: "Acme Logistik",
              detail: "acme-log.example",
            },
            evidence: [
              {
                field: "display_name",
                signal: "collide",
                left_value: "Acme Logistik GmbH",
                right_value: "Acme Logistik",
              },
            ],
          },
        },
      ],
      counts: { this_morning: 0, needs_you: 1, planned: 0, duplicates_open: 1 },
    });
    renderToday();
    // The two records, by name — the thing the old row could not say. Each
    // name appears twice on purpose: once as the side a reader picks, once in
    // the evidence row that shows what collided, so these count the side tiles.
    const sides = await screen.findAllByText(/^Acme Logistik( GmbH)?$/, {
      selector: ".merge-side-name",
    });
    expect(sides.map((node) => node.textContent)).toEqual([
      "Acme Logistik GmbH",
      "Acme Logistik",
    ]);
    // The comparison in the reader's words, never the database's column name.
    expect(screen.getByText("Company name")).toBeTruthy();
    expect(screen.queryByText("display_name")).toBeNull();
    // And the decision itself, here.
    expect(screen.getByRole("button", { name: "Merge them" })).toBeTruthy();
    expect(
      screen.getByRole("button", { name: "Different records" }),
    ).toBeTruthy();
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
      counts: {
        this_morning: 0,
        needs_you: 1234,
        planned: 0,
        duplicates_open: 5678,
      },
      needs_you: [
        {
          id: "dc-1",
          source: "dedupe_candidate",
          kind: "organization",
          confidence: 0.92,
          actions: ["merge"],
          pair: {
            left: { id: "org-1", label: "Acme Logistik GmbH" },
            right: { id: "org-2", label: "Acme Logistik" },
            evidence: [],
          },
        },
      ],
    });
    renderToday("de");
    await screen.findByText("1.234 Entscheidungen warten auf dich.");
    expect(
      screen.getByText("5.678 Dubletten-Paare insgesamt offen"),
    ).toBeTruthy();
    // The queue position is a magnitude too, and it is the newest one on the
    // page — the focus lane counts a reader through a queue of any size.
    expect(screen.getByText("Entscheidung 1 von 1.234")).toBeTruthy();
    // A percentage is a magnitude too, and below the grouping threshold it
    // reads the same in both notations — asserted so the site is covered by
    // name rather than by being too small to disagree.
    expect(screen.getByText("92% Übereinstimmung")).toBeTruthy();
  });

  // The planned lead is a separate arm of the same function, reachable only
  // when nothing needs deciding — so the test above cannot enter it, and a
  // fourth site would otherwise be ruled by the compiler and by nobody else.
  it("writes the planned figure in the reader's notation too", async () => {
    stub({
      ...emptyDay,
      counts: { this_morning: 0, needs_you: 0, planned: 4321 },
    });
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
      counts: { this_morning: 0, needs_you: 2, planned: 0 },
    });
    renderToday();
    await screen.findByText("2 decisions are waiting on you.");
  });
});

describe("what the day's surface does when the server says no", () => {
  // Found by clicking, not by a test: the server refuses some merges for
  // reasons a reader can act on — the workspace's own company cannot be merged
  // into another — and the first version swallowed every one of them. What a
  // person saw was a button that did nothing, which is the exact failure this
  // surface was built to end.
  it("says why a refused merge was refused", async () => {
    const user = userEvent.setup();
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        const url = String(input instanceof Request ? input.url : input);
        if (url.includes("/disposition") || init?.method === "POST") {
          return new Response(
            JSON.stringify({
              type: "https://errors.gradion.com/validation_error",
              title: "Unprocessable Entity",
              status: 422,
              detail: "this is the workspace's own company",
            }),
            {
              status: 422,
              headers: { "content-type": "application/problem+json" },
            },
          );
        }
        if (url.includes("/attention")) {
          return jsonResponse({
            ...emptyDay,
            counts: { this_morning: 0, needs_you: 1, planned: 0 },
            needs_you: [
              {
                id: "dc-9",
                source: "dedupe_candidate",
                kind: "organization",
                confidence: 1,
                actions: ["merge"],
                pair: {
                  left: { id: "org-a", label: "Gradion" },
                  right: { id: "org-b", label: "Gradion GmbH" },
                  evidence: [],
                },
              },
            ],
          });
        }
        return jsonResponse({ data: [] });
      }),
    );
    renderToday();
    // A winner first: merge is refused without one, so the click that reaches
    // the server is the one this test is about.
    await user.click((await screen.findAllByRole("radio"))[0]);
    await user.click(screen.getByRole("button", { name: "Merge them" }));
    // The server's own words, not a generic apology.
    expect(await screen.findByText(/workspace's own company/)).toBeTruthy();
  });
});

describe("what the night left on the worklist", () => {
  // The partition rule, from the reader's side. The focus lane fetches every
  // non-dedupe decision as an approval, so a brief item routed there renders a
  // card stuck on a failed fetch. It has to be its own lane, and this is the
  // test that would catch it moving.
  it("shows the overnight brief in its own lane, not as a decision", async () => {
    stub(
      {
        ...emptyDay,
        this_morning: [
          {
            id: "bi-1",
            source: "brief_item",
            rank: 1,
            subject: { type: "deal", id: "deal-1" },
            actions: ["act", "set_aside", "dismiss"],
          },
        ],
        counts: { this_morning: 1, needs_you: 0, planned: 0 },
      },
      undefined,
      {
        id: "run-1",
        generated_at: "2026-08-25T05:02:00Z",
        as_of: "2026-08-25T05:02:00Z",
        local_day: "2026-08-25",
        candidate_count: 3,
        items: [
          {
            id: "bi-1",
            deal_id: "deal-1",
            rank: 1,
            composite: 0.72,
            feature_vector: {
              winnability: 0.8,
              revenue: 0.61,
              timing: 0.9,
              momentum: 0.55,
              warmth: 0.74,
            },
            evidence_ids: ["ev-1"],
            state: "new",
          },
        ],
      },
    );
    renderToday();

    // The lane is drawn, and the card inside it is the brief's own card rather
    // than a decision row that would have gone looking for an approval.
    await screen.findByText("This morning");
    expect(await screen.findByTestId("brief-item-bi-1")).toBeTruthy();
    // The decision lane says the day is clear, because it is: a briefing item
    // is a suggestion about where to start and never a decision.
    expect(screen.queryByText("Decide")).toBeNull();
  });

  it("says a quiet morning out loud rather than drawing an empty box", async () => {
    stub(emptyDay);
    renderToday();

    await screen.findByText("This morning");
    expect(
      await screen.findByText(/found nothing worth your first hour/),
    ).toBeTruthy();
  });

  it("never reports a withheld morning as a quiet one", async () => {
    stub({ ...emptyDay, lanes_omitted: ["this_morning"] });
    renderToday();

    await screen.findByText("This morning");
    // The withheld line, not the quiet one: "you may not see this" and "there
    // is none" are different answers and the reader must be told which.
    expect(
      screen.queryByText(/found nothing worth your first hour/),
    ).toBeNull();
  });
});
