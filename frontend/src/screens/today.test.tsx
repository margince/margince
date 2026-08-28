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
  // A meeting leads the day, above every other lane. It is the one thing on the
  // page that happens whether or not the reader acts — a decision waits, an
  // appointment at eleven does not.
  it("leads with today's meetings above everything else waiting", async () => {
    stub({
      ...emptyDay,
      meetings: [
        {
          id: "act-1",
          source: "meeting",
          title: "Vogt — Angebotsbesprechung",
          due_at: "2026-08-25T11:00:00Z",
          subject: { type: "activity", id: "act-1" },
          actions: ["open"],
        },
      ],
      needs_you: [
        {
          id: "ap-1",
          source: "approval",
          kind: "send_email",
          title: "Send the Weber follow-up",
          actions: ["decide"],
        },
      ],
      counts: { this_morning: 0, needs_you: 1, planned: 0, meetings: 1 },
    });
    renderToday();

    await screen.findByText("Vogt — Angebotsbesprechung");
    expect(screen.getByText("1 on the calendar today.")).toBeTruthy();
  });
  // A drifting deal says how long it has been quiet, in the reader's own
  // language. The server sends the ground and the number and never the
  // sentence, so a card that dropped either would say "at risk" and leave the
  // rep to guess which patience produced it.
  it("says how long a deal has been quiet rather than only that it is at risk", async () => {
    stub({
      ...emptyDay,
      at_risk: [
        {
          id: "deal-1",
          source: "deal_at_risk",
          kind: "quiet",
          title: "Fleet retrofit",
          detail: "19",
          subject: { type: "deal", id: "deal-1" },
          actions: ["open"],
        },
      ],
      counts: { this_morning: 0, needs_you: 0, planned: 0, at_risk: 1 },
    });
    renderToday();

    await screen.findByText("Fleet retrofit");
    expect(screen.getByText("No contact for 19 days.")).toBeTruthy();
    expect(screen.queryByText("Your day is clear.")).toBeNull();
  });
  // A drifting deal is reachable from the row that reports it. Naming a deal as
  // going quiet and then making the rep go and find it by hand is a warning
  // they cannot act on, which is what this lane was before.
  it("lets the reader open the deal that has gone quiet", async () => {
    stub({
      ...emptyDay,
      at_risk: [
        {
          id: "deal-1",
          source: "deal_at_risk",
          kind: "quiet",
          title: "Fleet retrofit",
          detail: "19",
          subject: { type: "deal", id: "deal-1" },
          actions: ["open"],
        },
      ],
      counts: { this_morning: 0, needs_you: 0, planned: 0, at_risk: 1 },
    });
    renderToday();

    const row = await screen.findByRole("link", { name: /Fleet retrofit/ });
    expect(row.getAttribute("href")).toBe("#/deals/deal-1");
  });
  // A planned task reaches the lead it was raised for, and keeps its verbs. The
  // row carries both because the title is the link and the buttons are their
  // own targets — a whole-row link would swallow Done and Tomorrow.
  it("links a task to its record without losing its verbs", async () => {
    stub({
      ...emptyDay,
      planned: [
        {
          id: "task-1",
          source: "task",
          title: "Follow up with the new lead",
          due_at: "2026-08-25T09:00:00Z",
          overdue: true,
          subject: { type: "lead", id: "lead-9" },
          actions: ["complete", "snooze"],
        },
      ],
      counts: { this_morning: 0, needs_you: 0, planned: 1 },
    });
    renderToday();

    const link = await screen.findByRole("link", {
      name: "Follow up with the new lead",
    });
    expect(link.getAttribute("href")).toBe("#/leads/lead-9");
    expect(screen.getByRole("button", { name: "Done" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "Tomorrow" })).toBeTruthy();
  });
  // A meeting's subject is the activity behind it, which is a timeline entry
  // and not a record with a page. The row stays text rather than offering a
  // link to a screen that does not exist.
  it("draws a meeting as text, having nowhere to send the reader", async () => {
    stub({
      ...emptyDay,
      meetings: [
        {
          id: "act-1",
          source: "meeting",
          title: "Vogt — Angebotsbesprechung",
          due_at: "2026-08-25T11:00:00Z",
          subject: { type: "activity", id: "act-1" },
          actions: [],
        },
      ],
      counts: { this_morning: 0, needs_you: 0, planned: 0, meetings: 1 },
    });
    renderToday();

    await screen.findByText("Vogt — Angebotsbesprechung");
    expect(screen.queryByRole("link", { name: /Vogt/ })).toBeNull();
  });
  // A deal past its close date reports THAT, not a silence. The two grounds are
  // different facts and the card must not blur them.
  it("names the passed close date rather than reporting silence", async () => {
    stub({
      ...emptyDay,
      at_risk: [
        {
          id: "deal-1",
          source: "deal_at_risk",
          kind: "close_overdue",
          title: "Closing last month",
          detail: "2",
          due_at: "2026-07-26T00:00:00Z",
          overdue: true,
          subject: { type: "deal", id: "deal-1" },
          actions: ["open"],
        },
      ],
      counts: { this_morning: 0, needs_you: 0, planned: 0, at_risk: 1 },
    });
    renderToday();

    await screen.findByText("Closing last month");
    expect(screen.getByText(/Expected to close/)).toBeTruthy();
    expect(screen.queryByText(/No contact for/)).toBeNull();
  });
  // An installation whose feed reads no claims sends NO commitments lane and no
  // count — a different fact from "the rep owes nobody anything". Reading the
  // absent count as zero would answer "your day is clear" for a page that never
  // looked where promises live, which is the reassuring half of a question
  // nobody asked.
  it("does not call a day clear when nothing read the promises", async () => {
    stub(emptyDay);
    renderToday();

    await screen.findByText("Nothing is waiting in the lanes on this page.");
    expect(screen.queryByText("Your day is clear.")).toBeNull();
  });
  // The same defect, one lane later, and caught the same way: the lead line
  // read "Your day is clear" directly above an OVERDUE promise. It repeats
  // because every new lane has to be added to a summary written before it, so
  // this test is the sibling of the briefing one below rather than a copy of it.
  it("never calls a day clear when a promise is still owed", async () => {
    stub({
      ...emptyDay,
      commitments: [
        {
          id: "claim-1",
          source: "conversation_claim",
          title: "Referenzliste an Frau Wagner schicken",
          detail: "Ich schicke Ihnen die Referenzliste bis Dienstag.",
          subject: { type: "person", id: "person-1" },
          due_at: "2026-08-25T16:00:00Z",
          overdue: true,
          actions: ["open"],
        },
      ],
      counts: { this_morning: 0, needs_you: 0, planned: 0, commitments: 1 },
    });
    renderToday();

    await screen.findByText("Referenzliste an Frau Wagner schicken");
    expect(screen.queryByText("Your day is clear.")).toBeNull();
    expect(screen.getByText("You promised 1 — those come first.")).toBeTruthy();
  });
  // A promise outranks a briefing suggestion in the lead, because the reader
  // already agreed to it and only ever suggested the other. Both present, the
  // line must name the promise.
  it("leads with the promise rather than the briefing when both are on the day", async () => {
    stub({
      ...emptyDay,
      commitments: [
        {
          id: "claim-1",
          source: "conversation_claim",
          title: "Referenzliste an Frau Wagner schicken",
          subject: { type: "person", id: "person-1" },
          due_at: "2026-08-25T16:00:00Z",
          actions: ["open"],
        },
      ],
      counts: { this_morning: 2, needs_you: 0, planned: 0, commitments: 1 },
    });
    renderToday();

    await screen.findByText("Referenzliste an Frau Wagner schicken");
    expect(screen.queryByText(/the night picked out/)).toBeNull();
    expect(screen.getByText("You promised 1 — those come first.")).toBeTruthy();
  });
  // Caught in the browser, not by a test: the lead line was written before this
  // lane existed, so it read "Your day is clear" directly above two items the
  // night had picked out. A summary that contradicts the page under it is worse
  // than no summary, because a reader who reads only the line believes it.
  it("never calls a day clear when the night left something on it", async () => {
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
        candidate_count: 1,
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

    await screen.findByTestId("brief-item-bi-1");
    expect(screen.queryByText("Your day is clear.")).toBeNull();
    expect(screen.getByText(/the night picked out/)).toBeTruthy();
  });
  // Found in the browser, not by a test. The lane intersects the feed's item
  // ids with the brief's own items, and the two reads settle independently — so
  // a feed naming two entries against a brief that has not caught up produced an
  // EMPTY intersection, and the lane drew "the night found nothing" over a
  // morning that had work in it. The reader was told the opposite of the truth.
  it("does not call a morning quiet while the items it names are still arriving", async () => {
    stub(
      {
        ...emptyDay,
        this_morning: [
          {
            id: "bi-9",
            source: "brief_item",
            rank: 1,
            subject: { type: "deal", id: "deal-9" },
            actions: ["act", "set_aside", "dismiss"],
          },
        ],
        counts: { this_morning: 1, needs_you: 0, planned: 0 },
      },
      undefined,
      // The brief read answers a run that does not carry the id the feed named.
      {
        id: "run-old",
        generated_at: "2026-08-24T05:02:00Z",
        as_of: "2026-08-24T05:02:00Z",
        local_day: "2026-08-24",
        candidate_count: 0,
        items: [],
      },
    );
    renderToday();

    await screen.findByText("This morning");
    expect(
      screen.queryByText(/found nothing worth your first hour/),
    ).toBeNull();
  });
  // The lane holds unanswered work, and the two reads settle at different
  // speeds: a mark patches the brief cache at once while the feed catches up on
  // its own refetch. Trusting the feed alone left a card the reader had just
  // answered sitting in the lane, drawn in its settled state, for as long as the
  // network took.
  it("drops an item the brief already reports as answered", async () => {
    stub(
      {
        ...emptyDay,
        this_morning: [
          {
            id: "bi-7",
            source: "brief_item",
            rank: 1,
            subject: { type: "deal", id: "deal-7" },
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
        candidate_count: 1,
        items: [
          {
            id: "bi-7",
            deal_id: "deal-7",
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
            // Answered, though the feed has not dropped it yet.
            state: "acted",
            state_at: "2026-08-25T09:01:00Z",
          },
        ],
      },
    );
    renderToday();

    await screen.findByText("This morning");
    expect(screen.queryByTestId("brief-item-bi-7")).toBeNull();
    // And the lane says the quiet thing rather than the still-arriving thing:
    // an answered item is legitimately gone, not late.
    expect(
      await screen.findByText(/found nothing worth your first hour/),
    ).toBeTruthy();
  });
});

describe("a decision lane handed something it cannot decide", () => {
  // The lane branched on duplicates and treated everything else as a staged
  // proposal, reading /approvals/{id} with the item's own id. For an item that
  // is not an approval that read answers 404, so the card rendered as a failed
  // one — on the surface whose whole promise is that it can be finished.
  //
  // A producer putting an undecidable source on this lane is the case that
  // shipped it, so the test drives exactly that and asserts two things: the
  // reader gets the item's own words, and no approval was ever asked for.
  it("draws the item instead of asking the approvals endpoint about it", async () => {
    stub({
      ...emptyDay,
      needs_you: [
        {
          id: "task-1",
          source: "task",
          title: "Call the buyer back",
          actions: [],
        },
      ],
      counts: { this_morning: 0, needs_you: 1, planned: 0 },
    });
    renderToday();

    expect(await screen.findByText("Call the buyer back")).toBeTruthy();
    const asked = vi
      .mocked(fetch)
      .mock.calls.map(([input]) =>
        String(input instanceof Request ? input.url : input),
      );
    expect(asked.some((url) => url.includes("/approvals/"))).toBe(false);
  });

  // A duplicate pair whose entity type the reader has no sentence for, drawn on
  // a reporting lane — the rows that write the headline themselves. Naming the
  // wrong noun is worse than naming none: the pair may be two deals, and calling
  // them contacts sends the reader looking for something not on screen.
  it("names an unfamiliar duplicate generically rather than calling it a contact", async () => {
    stub({
      ...emptyDay,
      done_for_you: [
        {
          id: "dc-1",
          source: "dedupe_candidate",
          kind: "deal",
          actions: [],
        },
      ],
      counts: { this_morning: 0, needs_you: 0, planned: 0 },
    });
    renderToday();

    expect(
      await screen.findByText("Two records look like the same one"),
    ).toBeTruthy();
    expect(screen.queryByText(/Two contacts/)).toBeNull();
  });
});

describe("pushing a decision to the back of the queue", () => {
  // "Later" deferred the head of the LANE rather than the card on screen. The
  // two are the same only until the first deferral, after which the lane's head
  // is already at the back — so the second card's "Later" deferred something
  // already deferred and the queue stopped moving. A reader who cannot pass the
  // second card cannot reach the third.
  it("advances past the second card too", async () => {
    const user = userEvent.setup();
    stub({
      ...emptyDay,
      needs_you: [
        { id: "t-1", source: "task", title: "First promise", actions: [] },
        { id: "t-2", source: "task", title: "Second promise", actions: [] },
        { id: "t-3", source: "task", title: "Third promise", actions: [] },
      ],
      counts: { this_morning: 0, needs_you: 3, planned: 0 },
    });
    renderToday();

    await screen.findByText("First promise");
    await user.click(screen.getByRole("button", { name: "Later" }));
    expect(await screen.findByText("Second promise")).toBeTruthy();

    await user.click(screen.getByRole("button", { name: "Later" }));
    expect(await screen.findByText("Third promise")).toBeTruthy();
  });
});
