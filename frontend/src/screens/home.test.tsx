/** @vitest-environment jsdom */
import {
  cleanup,
  render as rtlRender,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { formatTimeOfDay, MONEY_ABSENT } from "../format/format";
import { viewerZone } from "../format/timezone";
import { LocaleProvider } from "../i18n";
import { en } from "../i18n/en";
import type { BriefView } from "./brief.view";
import { HomeScreen } from "./home";
import { overnightRow, readingsDay } from "./home.fixtures";
import { HomeGlance } from "./home.glance";
import {
  fleetDeal,
  jsonResponse,
  pendingPage,
  proposal,
  readSnoozedUntil,
  render,
  run,
  stubApi,
  threeRanked,
  workOrder,
  writeRoutes,
  writes,
} from "./home.testkit";
import type { Worklist } from "./worklist.queries";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.useRealTimers();
  window.location.hash = "";
});

// ── The deck: staging is local, the commit is the only thing that sends ──

describe("HomeScreen — the deck stages, and only the commit sends", () => {
  it("stages three verdicts without a single write, then sends exactly the two that are verdicts", async () => {
    const queue = [
      proposal("ap-1", "Send the Weber follow-up"),
      proposal("ap-2", "Advance the PIM deal"),
      proposal("ap-3", "Promote Kilian Wenzel"),
    ];
    const decided = new Set<string>();
    const calls = stubApi({
      "GET /approvals": () => pendingPage(queue, decided),
      "POST /approvals/ap-1/approve": () => {
        decided.add("ap-1");
        return jsonResponse({ ...queue[0], status: "approved" });
      },
      "POST /approvals/ap-2/reject": () => {
        decided.add("ap-2");
        return jsonResponse({ ...queue[1], status: "rejected" });
      },
    });
    const user = userEvent.setup();
    render(<HomeScreen />);

    // One card at a time: staging the live one brings the next forward.
    await screen.findByText("Send the Weber follow-up");
    await user.click(screen.getByRole("button", { name: "Accept" }));
    await screen.findByText("Advance the PIM deal");
    await user.click(screen.getByRole("button", { name: "Reject" }));
    await screen.findByText("Promote Kilian Wenzel");
    await user.click(screen.getByRole("button", { name: "Later" }));

    // The whole point of the tray: three answers given, nothing sent. A
    // committed decision cannot be undone, so this is where the undo lives.
    expect(writes(calls)).toEqual([]);
    expect(screen.getByText("3 decisions staged")).toBeTruthy();

    await user.click(
      screen.getByRole("button", { name: "Send staged decisions" }),
    );
    await waitFor(() =>
      expect(writeRoutes(calls)).toEqual([
        "POST /approvals/ap-1/approve",
        "POST /approvals/ap-2/reject",
      ]),
    );
    // Later means later: ap-3 was never sent, and the deck says two went.
    expect(await screen.findByText("2 decisions sent")).toBeTruthy();
    expect(writes(calls)[1].body).toEqual({ reason: "" });
  });

  // The API decides a bundle as a unit, so a reader answering it card by card
  // would be answering a question that no longer exists after the first one.
  it("commits a bundle through the bundle endpoint once, not once per member", async () => {
    const bundleId = "bn-1";
    const queue = [
      proposal("apb-1", "Publish the acme.example facts", {
        bundle_id: bundleId,
      }),
      proposal("apb-2", "Lead: Anna Weber", { bundle_id: bundleId }),
      proposal("apb-3", "Lead: Mira Osei", { bundle_id: bundleId }),
    ];
    const decided = new Set<string>();
    const calls = stubApi({
      "GET /approvals": () => pendingPage(queue, decided),
      "POST /approval-bundles/bn-1/approve": () => {
        for (const member of queue) {
          decided.add(member.id);
        }
        return jsonResponse({
          bundle_id: bundleId,
          data: queue.map((approval) => ({
            approval: { ...approval, status: "approved" },
            outcome: "decided",
          })),
        });
      },
    });
    const user = userEvent.setup();
    render(<HomeScreen />);

    // Three proposals, ONE card. The card is headlined by the member it
    // represents, says how much saying yes decides, and keeps the other two
    // behind an expander rather than drawing three questions.
    expect(await screen.findByText("One decision · 3 items")).toBeTruthy();
    expect(document.querySelectorAll(".dcard").length).toBe(1);
    expect(document.querySelector(".approval-headline")?.textContent).toBe(
      "Publish the acme.example facts",
    );
    expect(screen.getByText("Show the 3 items")).toBeTruthy();

    await user.click(screen.getByRole("button", { name: "Accept" }));
    expect(writes(calls)).toEqual([]);
    await user.click(
      screen.getByRole("button", { name: "Send staged decisions" }),
    );
    await waitFor(() =>
      expect(writeRoutes(calls)).toEqual([
        "POST /approval-bundles/bn-1/approve",
      ]),
    );
    // One call decided all three members, so the card is gone from the queue.
    await waitFor(() =>
      expect(screen.queryByText("One decision · 3 items")).toBeNull(),
    );
    // And the plate the commit earned is actually drawn. Emptying the deck also
    // flips the column's order, and while the two sections were positional
    // children of a fragment that flip REMOUNTED the deck — losing the tally
    // behind this plate, so clearing the queue could never show the one state
    // clearing it exists to reach. The sections carry keys for this reason.
    expect(await screen.findByText("Deck clear")).toBeTruthy();
    expect(screen.getByText("1 decision sent")).toBeTruthy();
  });

  // A bundle whose members somebody else answered first. Deciding a bundle is
  // not all-or-nothing — the response reports each member — and reading that
  // report as "nothing was already decided" told this reader their commit had
  // landed on work that had in fact been settled elsewhere.
  it("says so when a bundle's members were already decided", async () => {
    const bundleId = "bn-2";
    const queue = [
      proposal("apc-1", "Publish the nordwind.example facts", {
        bundle_id: bundleId,
      }),
      proposal("apc-2", "Lead: Jonas Brandt", { bundle_id: bundleId }),
    ];
    const decided = new Set<string>();
    stubApi({
      "GET /approvals": () => pendingPage(queue, decided),
      "POST /approval-bundles/bn-2/approve": () => {
        for (const member of queue) {
          decided.add(member.id);
        }
        return jsonResponse({
          bundle_id: bundleId,
          data: queue.map((approval) => ({
            approval: { ...approval, status: "approved" },
            outcome: "already_decided",
          })),
        });
      },
    });
    const user = userEvent.setup();
    render(<HomeScreen />);

    await user.click(await screen.findByRole("button", { name: "Accept" }));
    await user.click(
      screen.getByRole("button", { name: "Send staged decisions" }),
    );
    expect(await screen.findByText(/already/i)).toBeTruthy();
  });

  // An edit re-enters the admission gate with a new payload, which is a form
  // rather than a swipe — so the deck sends nothing and hands the reader over.
  it("sends nothing for a staged edit and lands the reader on the Decisions screen", async () => {
    const queue = [proposal("ap-1", "Send the Weber follow-up")];
    const calls = stubApi({
      "GET /approvals": () => pendingPage(queue, new Set()),
    });
    const user = userEvent.setup();
    render(<HomeScreen />);

    await screen.findByText("Send the Weber follow-up");
    await user.click(screen.getByRole("button", { name: "Edit" }));
    await user.click(
      screen.getByRole("button", { name: "Send staged decisions" }),
    );

    await waitFor(() => expect(window.location.hash).toBe("#/worklist"));
    expect(writes(calls)).toEqual([]);
  });

  // Somebody else answered one of them first. That is news, not a failed
  // commit: the rest of the tray still deserves to go.
  it("surfaces an already-decided 409 without abandoning the rest of the tray", async () => {
    const queue = [
      proposal("ap-1", "Send the Weber follow-up"),
      proposal("ap-2", "Advance the PIM deal"),
    ];
    const decided = new Set<string>();
    const calls = stubApi({
      "GET /approvals": () => pendingPage(queue, decided),
      "POST /approvals/ap-1/approve": () => {
        decided.add("ap-1");
        return jsonResponse(
          { title: "Conflict", code: "already_decided" },
          409,
        );
      },
      "POST /approvals/ap-2/approve": () => {
        decided.add("ap-2");
        return jsonResponse({ ...queue[1], status: "approved" });
      },
    });
    const user = userEvent.setup();
    render(<HomeScreen />);

    await screen.findByText("Send the Weber follow-up");
    await user.click(screen.getByRole("button", { name: "Accept" }));
    await screen.findByText("Advance the PIM deal");
    await user.click(screen.getByRole("button", { name: "Accept" }));
    await user.click(
      screen.getByRole("button", { name: "Send staged decisions" }),
    );

    expect(
      await screen.findByText("Already decided — nothing left to do here."),
    ).toBeTruthy();
    // The refusal on the first item did not swallow the second one.
    expect(writeRoutes(calls)).toEqual([
      "POST /approvals/ap-1/approve",
      "POST /approvals/ap-2/approve",
    ]);
  });

  // The approve still mints a token — an agent redeems its own staging with
  // one — and the surface it used to be shown on is exactly where a reader
  // would still look, so the absence is asserted at SCREEN level.
  it("shows no approval token after the commit re-reads the queue", async () => {
    const queue = [proposal("ap-1", "Send the Weber follow-up")];
    const decided = new Set<string>();
    stubApi({
      "GET /approvals": () => pendingPage(queue, decided),
      "POST /approvals/ap-1/approve": () => {
        decided.add("ap-1");
        return jsonResponse({
          ...queue[0],
          status: "approved",
          approval_token: "example-home-token",
        });
      },
    });
    const user = userEvent.setup();
    render(<HomeScreen />);

    await screen.findByText("Send the Weber follow-up");
    await user.click(screen.getByRole("button", { name: "Accept" }));
    await user.click(
      screen.getByRole("button", { name: "Send staged decisions" }),
    );

    await waitFor(() =>
      expect(screen.queryByText("Send the Weber follow-up")).toBeNull(),
    );
    expect(screen.queryByText("example-home-token")).toBeNull();
  });
});

// ── The page's order follows the day ──

describe("HomeScreen — the order of the page follows the day", () => {
  it("leads with the decisions while any are waiting", async () => {
    stubApi({
      "GET /approvals": () =>
        pendingPage([proposal("ap-1", "Send the Weber follow-up")], new Set()),
      "GET /brief": () => jsonResponse(run),
      "GET /deals": () => jsonResponse({ data: [fleetDeal] }),
    });
    render(<HomeScreen />);

    await screen.findByText("Send the Weber follow-up");
    expect(workOrder()).toEqual(["home-decisions", "home-focus"]);
  });

  it("leads with the ranked queue once the deck is clear", async () => {
    stubApi({
      "GET /brief": () => jsonResponse(run),
      "GET /deals": () => jsonResponse({ data: [fleetDeal] }),
    });
    render(<HomeScreen />);

    await screen.findByText("Fleet retrofit");
    expect(workOrder()).toEqual(["home-focus", "home-decisions"]);
  });
});

// ── The greeting, on a clock the test owns ──

describe("HomeGlance — the greeting follows the reader's own hour", () => {
  // A local-time instant, so the hour the case names is the hour the reader's
  // own zone reports whatever machine this runs on.
  function greetingAt(hour: number, firstName: string | null): string {
    const view = rtlRender(
      <LocaleProvider initial="en">
        <HomeGlance
          view="morning"
          firstName={firstName}
          now={new Date(2026, 6, 5, hour, 30, 0)}
          // These cases are about the GREETING's hour. An unread day is the
          // right value for them: the sentence is then absent, and the greeting
          // is what the assertion reads.
          day={undefined}
          week={undefined}
        />
      </LocaleProvider>,
    );
    const greeting =
      screen.getByRole("heading", { level: 1 }).textContent ?? "";
    view.unmount();
    return greeting;
  }

  it("names four bands, at the boundaries of each", () => {
    expect(greetingAt(5, "Ada")).toBe("Good morning, Ada.");
    expect(greetingAt(11, "Ada")).toBe("Good morning, Ada.");
    expect(greetingAt(12, "Ada")).toBe("Good afternoon, Ada.");
    expect(greetingAt(17, "Ada")).toBe("Good afternoon, Ada.");
    expect(greetingAt(18, "Ada")).toBe("Good evening, Ada.");
    expect(greetingAt(21, "Ada")).toBe("Good evening, Ada.");
    expect(greetingAt(22, "Ada")).toBe("Still at it, Ada.");
    expect(greetingAt(4, "Ada")).toBe("Still at it, Ada.");
  });

  // The hour is known before the name is. Greeting nobody until /me answers
  // would move the heading under the reader a moment after they read it.
  it("greets the hour with no name at all while the session is in flight", () => {
    expect(greetingAt(9, null)).toBe("Good morning.");
    expect(greetingAt(2, null)).toBe("Still at it.");
  });

  it("draws no line for a reading it was not given", () => {
    rtlRender(
      <LocaleProvider initial="en">
        <HomeGlance
          view="morning"
          firstName="Ada"
          now={new Date(2026, 6, 5, 9, 0, 0)}
          day={undefined}
          week={undefined}
        />
      </LocaleProvider>,
    );
    // Not one of them, and in particular not the "nothing is waiting" claim: an
    // unread queue is not an empty one.
    // The header never states a count. Every fact those lines carried is drawn
    // by the section that owns it — the deck, the readings strip, the rail's
    // panels — so an unread queue leaves the header saying only the greeting.
    expect(screen.queryByTestId("glance-sentence")).toBeNull();
    expect(screen.queryByText("Nothing is waiting on you.")).toBeNull();
  });
});

// ── The weekly speaks about its own week ──

describe("HomeGlance — the weekly's sentence comes from the closed week", () => {
  // Only what the sentence reads. A fuller review would let this suite pass
  // over a composer reaching for a figure the weekly does not actually carry.
  const CLOSED_WEEK = {
    local_week_start: "2026-06-29",
    counts: {
      tasks_due: 0,
      tasks_done: 0,
      tasks_carried_over: 0,
      deals_moved: 0,
      deals_won: 2,
      deals_lost: 0,
      proposals_accepted: 0,
      proposals_rejected: 0,
      brief_items_acted: 0,
      brief_items_dismissed: 0,
      commitments_due: 3,
      commitments_kept: 1,
      leads_routed: 0,
      leads_answered_in_target: 0,
      leads_breached: 0,
      meetings_held: 0,
      meetings_with_next_step: 0,
    },
  } as unknown as Parameters<typeof HomeGlance>[0]["week"];

  function sentenceOf(
    view: BriefView,
    week: Parameters<typeof HomeGlance>[0]["week"],
  ): string | null {
    const rendered = rtlRender(
      <LocaleProvider initial="en">
        <HomeGlance
          view={view}
          firstName="Ada"
          now={new Date(2026, 6, 5, 9, 0, 0)}
          // A read day, so the MORNING case has a sentence of its own to draw.
          // Without it both views would fall silent and the assertion that they
          // say different things would pass over a component saying neither.
          day={readingsDay({})}
          week={week}
        />
      </LocaleProvider>,
    );
    const text = screen.queryByTestId("glance-sentence")?.textContent ?? null;
    rendered.unmount();
    return text;
  }

  it("states the week's result and what it left behind", () => {
    const said = sentenceOf("weekly", CLOSED_WEEK);
    expect(said).toContain("closed 2");
    expect(said).toContain("2 promises");
  });

  // The two views compose from different reads. Over the weekly the morning's
  // sentence would be describing today under a heading about a closed week.
  it("does not put the morning's sentence under the weekly's heading", () => {
    expect(sentenceOf("weekly", CLOSED_WEEK)).not.toBe(
      sentenceOf("morning", CLOSED_WEEK),
    );
  });

  // A week still in flight, or one that failed to read. The heading names the
  // view and claims nothing about it.
  it("draws no sentence at all over a week it has not read", () => {
    expect(sentenceOf("weekly", undefined)).toBeNull();
  });
});

// ── The eyebrow dates the morning's reading, and only the morning's ──

describe("HomeGlance — the eyebrow says when the queue was read", () => {
  function eyebrowOf(view: BriefView, day: Worklist | undefined): string {
    const rendered = rtlRender(
      <LocaleProvider initial="en">
        <HomeGlance
          view={view}
          firstName="Ada"
          now={new Date(2026, 6, 5, 9, 0, 0)}
          day={day}
          week={undefined}
        />
      </LocaleProvider>,
    );
    const text =
      screen.getByTestId("home-glance").firstChild?.textContent ?? "";
    rendered.unmount();
    return text;
  }

  // Derived from the fixture and the runner's own zone. A literal time here
  // would pin the test to whichever machine wrote it.
  it("names the moment the morning's queue was read", () => {
    const day = readingsDay({});
    expect(eyebrowOf("morning", day)).toBe(
      `Your morning · as of ${formatTimeOfDay(day.as_of, "en", viewerZone())}`,
    );
  });

  // The weekly's numbers were frozen when the week closed. A time of day
  // against them dates the reading rather than the week.
  it("gives the weekly no as-of at all", () => {
    expect(eyebrowOf("weekly", readingsDay({}))).toBe("Your week");
  });

  // A queue still in flight has no moment to name, and inventing one would
  // date a reading that has not happened.
  it("says the scope alone while the morning's queue is unread", () => {
    expect(eyebrowOf("morning", undefined)).toBe("Your morning");
  });
});

// ── A reading is absent until it is read. Never zero. ──

// ── A bundle's chips are claims about the ACT, not about the drawn member ──

describe("HomeScreen — a bundle says only what its members agree on", () => {
  // A bundle is drawn from one member, which is right for what the card decides
  // and wrong for what it claims. Two site reads of the same company stage under
  // one bundle — the second joins the first's still-pending rows and moves them
  // onto its own bundle — so the members really do name different agents.
  it("draws no provenance tag where the members name different agents", async () => {
    const queue = [
      proposal("apx-1", "Lead: Anna Weber", {
        bundle_id: "bn-2",
        proposed_by: "agent:deepread",
      }),
      proposal("apx-2", "Lead: Mira Osei", {
        bundle_id: "bn-2",
        proposed_by: "agent:site-read",
      }),
    ];
    stubApi({ "GET /approvals": () => pendingPage(queue, new Set<string>()) });
    render(<HomeScreen />);

    expect(await screen.findByText("One decision · 2 items")).toBeTruthy();
    // Neither agent's name — and not the unnamed tag either, which would still
    // claim one agent produced the whole act.
    expect(screen.queryByText("Automated by deepread")).toBeNull();
    expect(screen.queryByText("Automated by site-read")).toBeNull();
    expect(screen.queryByText("Automated by an agent")).toBeNull();
    // The KIND is every member's, so the chip that says what this act is stays:
    // the rule drops the fact that diverged, not the card's meta line.
    expect(screen.getByText("Send an email")).toBeTruthy();
  });

  // The other end. Without this, blanking the tag unconditionally would pass the
  // case above, and every single-agent bundle would lose a true reading.
  it("keeps the tag where every member names the same agent", async () => {
    const queue = [
      proposal("apy-1", "Lead: Anna Weber", { bundle_id: "bn-3" }),
      proposal("apy-2", "Lead: Mira Osei", { bundle_id: "bn-3" }),
    ];
    stubApi({ "GET /approvals": () => pendingPage(queue, new Set<string>()) });
    render(<HomeScreen />);

    expect(await screen.findByText("One decision · 2 items")).toBeTruthy();
    expect(screen.getByText("Automated by runner")).toBeTruthy();
  });
});

describe("HomeScreen — a reading in flight is absent, not zero", () => {
  /** The deck's own section, which is where a failed decisions read belongs. */
  function deckSection(): HTMLElement {
    const section = document.getElementById("home-decisions");
    if (!section) {
      throw new Error("Home rendered no decisions section");
    }
    return section;
  }

  it("draws no decisions tile while the queue is still loading, and keeps the ranked queue", async () => {
    stubApi({
      // Never settles: the queue is in flight for the whole case.
      "GET /approvals": () => new Promise<Response>(() => {}),
      "GET /brief": () => jsonResponse(run),
      "GET /deals": () => jsonResponse({ data: [fleetDeal] }),
    });
    render(<HomeScreen />);

    // The other reads land, so the page is drawn and its silence about decisions
    // is a choice rather than a page that has not started. The strip is the
    // witness that the page rendered: it is drawn from the worklist answer, not
    // from the queue this case leaves in flight.
    await screen.findByTestId("home-readings");
    expect(screen.queryByText("Nothing is waiting on you.")).toBeNull();
    // The wait belongs to the deck alone. Five independent reads exist so that
    // one of them being slow cannot blank the other four.
    expect(deckSection().querySelector("[aria-busy='true']")).toBeTruthy();
    expect(screen.getByText("Fleet retrofit")).toBeTruthy();
  });

  it("shows a failed decisions read in the deck alone, with the ranked queue intact", async () => {
    stubApi({
      "GET /approvals": () => jsonResponse({ title: "Server Error" }, 500),
      "GET /brief": () => jsonResponse(run),
      "GET /deals": () => jsonResponse({ data: [fleetDeal] }),
    });
    render(<HomeScreen />);

    await screen.findByTestId("home-readings");
    // Not "nothing is waiting": a queue that could not be read is not an empty
    // one, and the deck says which of the two this is.
    expect(screen.queryByText("Nothing is waiting on you.")).toBeNull();
    expect(
      within(deckSection()).getByText("This section did not load."),
    ).toBeTruthy();
    // A healthy ranked queue is not hidden behind a failed decisions read.
    expect(screen.getByText("Fleet retrofit")).toBeTruthy();
    expect(screen.getByText("2 evidence rows")).toBeTruthy();
  });

  // Home reads ONE page of deals. Past it every reading taken from those rows is
  // a floor, and the failure this guards is the quiet one: the same words, a
  // smaller number, and nothing failing.
  //
  // The readings strip no longer counts deals — it is drawn from the worklist
  // answer, which carries its own bound. What still reads a deals PAGE is the
  // panel that lists the quiet ones, and that is where the floor has to be said.
  it("reports a deal reading off a full page as a floor rather than a total", async () => {
    stubApi({
      "GET /deals": () =>
        jsonResponse({
          data: [fleetDeal, { ...fleetDeal, id: "d-2", stalled: true }],
          page: { has_more: true },
        }),
    });
    render(<HomeScreen />);

    // The panel that LISTS them says the list is part of one, rather than making
    // the "nothing has gone quiet" claim it has no grounds for.
    expect(await screen.findByText("Showing part of the list")).toBeTruthy();
  });

  // What "today" means is the reader's own calendar day, so this is the one case
  // that pins the clock rather than reading it.
  it("counts what stops waiting today in the reader's own day", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    vi.setSystemTime(new Date(2026, 6, 5, 12, 0, 0));
    stubApi({
      "GET /approvals": () =>
        pendingPage(
          [
            proposal("ap-1", "Send the Weber follow-up", {
              expires_at: new Date(2026, 6, 5, 18, 0, 0).toISOString(),
            }),
            proposal("ap-2", "Advance the PIM deal", {
              expires_at: new Date(2026, 6, 6, 9, 0, 0).toISOString(),
            }),
          ],
          new Set(),
        ),
    });
    render(<HomeScreen />);

    // The DECK is where decisions are said now, and it draws the proposal
    // itself rather than a count of them — a stronger claim than the line that
    // used to sit above it, which could be right about a queue the section
    // failed to render.
    //
    // ONE card, because a deck shows one: the second proposal is behind it and
    // reached by deciding this one. What this case can still hold is that the
    // queue arrived and the deck is drawing from it.
    await waitFor(() =>
      expect(
        within(deckSection()).getByText("Send the Weber follow-up"),
      ).toBeTruthy(),
    );
    // The expiry the card states is its own countdown, in the reader's zone.
    // The "how many stop waiting today" FIGURE has no surface any more: it
    // existed only for the removed briefing line, and its same-day arithmetic
    // (home.tsx expiringToday) went with it. Whether the deck should say that
    // across the whole queue is a product question, filed rather than guessed.
    expect(within(deckSection()).getByText(/expires/i)).toBeTruthy();
  });
});

// ── The ranked queue ──

describe("HomeScreen — the ranked queue", () => {
  // ONE brief run reaches this page through TWO endpoints: `GET /worklist`
  // ranks each suggestion into the one order as a `brief_item` row, and
  // `GET /brief` serves the same records with the factors behind them, under the
  // same ids. So a suggestion that ranks into "Do next" is on the page twice
  // unless "Focus when time opens" leaves out what the lead already drew — once
  // as a worklist row and once as a card, each offering its own controls over
  // the same deal.
  it("draws an overnight suggestion once, even when it also leads the page", async () => {
    stubApi({
      "GET /brief": () => jsonResponse(threeRanked),
      "GET /deals": () => jsonResponse({ data: [fleetDeal] }),
      "GET /worklist": () =>
        jsonResponse(readingsDay({}, [overnightRow("bi-1", "d-1")])),
    });
    render(<HomeScreen />);

    // The suggestions that did not lead are drawn as cards, so the section is
    // skipping ONE record rather than going quiet. Awaited first: it is the
    // slowest of the reads this assertion depends on.
    expect(await screen.findByTestId("brief-item-bi-2")).toBeTruthy();
    expect(screen.getByTestId("brief-item-bi-3")).toBeTruthy();
    // The one that leads is drawn ONCE — as a worklist row, not again as a card.
    const lead = screen.getByRole("region", {
      name: en["brief.donext.title"],
    });
    expect(within(lead).getAllByRole("listitem")).toHaveLength(1);
    expect(screen.queryByTestId("brief-item-bi-1")).toBeNull();
  });

  // The other half of the same rule. Nothing led the page, so the section below
  // owns every suggestion — a filter that dropped one here would hide work.
  it("draws every suggestion when none of them leads the page", async () => {
    stubApi({
      "GET /brief": () => jsonResponse(threeRanked),
      "GET /deals": () => jsonResponse({ data: [fleetDeal] }),
    });
    render(<HomeScreen />);

    expect(await screen.findByTestId("brief-item-bi-1")).toBeTruthy();
    expect(screen.getByTestId("brief-item-bi-2")).toBeTruthy();
    expect(screen.getByTestId("brief-item-bi-3")).toBeTruthy();
  });

  // A morning whose every suggestion already leads. "Nothing cleared the bar"
  // would contradict the rows the reader can see directly above.
  it("says the work is above rather than that the night found nothing", async () => {
    stubApi({
      "GET /brief": () => jsonResponse(threeRanked),
      "GET /deals": () => jsonResponse({ data: [fleetDeal] }),
      "GET /worklist": () =>
        jsonResponse(
          readingsDay({}, [
            overnightRow("bi-1", "d-1"),
            overnightRow("bi-2", "d-2"),
            overnightRow("bi-3", "d-3"),
          ]),
        ),
    });
    render(<HomeScreen />);

    expect(await screen.findByText(en["home.focus.allAbove"])).toBeTruthy();
    expect(screen.queryByText(en["home.quietRun"])).toBeNull();
  });

  it("renders the run: the deal, its money, the decomposition, the evidence and the honest-short line", async () => {
    stubApi({
      "GET /brief": () => jsonResponse(run),
      "GET /deals": () => jsonResponse({ data: [fleetDeal] }),
    });
    render(<HomeScreen />);

    const card = await screen.findByTestId("brief-item-bi-1");
    expect(within(card).getByText("Fleet retrofit")).toBeTruthy();
    expect(card.textContent).toContain("#1");
    expect(within(card).getByText("Score")).toBeTruthy();
    expect(within(card).getByText("74%")).toBeTruthy();
    expect(within(card).getByText("Winnability")).toBeTruthy();
    expect(within(card).getByText("Warmth")).toBeTruthy();
    expect(within(card).getByText("2 evidence rows")).toBeTruthy();
    expect(within(card).getByText("€48,000.00")).toBeTruthy();
    expect(
      screen.getByText(
        "Only 1 deals cleared the bar — the queue is never padded.",
      ),
    ).toBeTruthy();
  });

  // A money figure is an amount AND its currency. The amount on its own is not
  // one: naming a currency for it puts a euro sign on money that might be dong.
  it("states no figure for a brief deal with an amount and no currency", async () => {
    stubApi({
      "GET /brief": () => jsonResponse(run),
      "GET /deals": () =>
        jsonResponse({ data: [{ ...fleetDeal, currency: null }] }),
    });
    render(<HomeScreen />);

    const card = await screen.findByTestId("brief-item-bi-1");
    expect(
      within(card).getByText(MONEY_ABSENT, {
        selector: ".brief-item-amount",
      }),
    ).toBeTruthy();
    expect(within(card).queryByText(/48,000/)).toBeNull();
  });

  it("fetches today's brief on demand when the night has not left one, and renders it", async () => {
    let generated = false;
    stubApi({
      "GET /brief": () =>
        generated
          ? jsonResponse(run)
          : jsonResponse({ title: "Not Found" }, 404),
      "POST /brief": () => {
        generated = true;
        return jsonResponse(run, 201);
      },
      "GET /deals": () => jsonResponse({ data: [fleetDeal] }),
    });
    const user = userEvent.setup();
    render(<HomeScreen />);

    await screen.findByText(/ranks the deals worth your first hour/);
    await user.click(
      screen.getByRole("button", { name: /Get today's brief now/ }),
    );
    expect(await screen.findByText("Fleet retrofit")).toBeTruthy();
  });

  it("says the quiet out loud when a run ranked nothing — no invented urgency", async () => {
    stubApi({
      "GET /brief": () =>
        jsonResponse({ ...run, candidate_count: 0, items: [] }),
    });
    render(<HomeScreen />);

    expect(
      await screen.findByText(
        "Nothing cleared the bar this morning. No invented urgency — enjoy the quiet.",
      ),
    ).toBeTruthy();
  });

  it("marks an item acted in place, still visible and receded", async () => {
    stubApi({
      "GET /brief": () => jsonResponse(run),
      "GET /deals": () => jsonResponse({ data: [fleetDeal] }),
      "POST /brief/items/bi-1/act": () =>
        jsonResponse({
          ...run.items[0],
          state: "acted",
          state_at: "2026-07-05T06:00:00Z",
        }),
    });
    const user = userEvent.setup();
    render(<HomeScreen />);

    await screen.findByText("Fleet retrofit");
    await user.click(screen.getByRole("button", { name: /Done/ }));
    expect(await screen.findByText("acted")).toBeTruthy();
    expect(screen.getByText("Fleet retrofit")).toBeTruthy();
  });

  // The contract requires a FUTURE instant, and the product has no picker yet:
  // Home's promise is "back tomorrow morning", in the reader's own zone.
  it("snoozes an item until tomorrow morning in the reader's own zone", async () => {
    // "Tomorrow" is a claim about the reader's calendar, so the clock is pinned:
    // a case that derives its own expectation from the live clock agrees with the
    // component even when both are wrong, and it changes verdict overnight.
    vi.useFakeTimers({ shouldAdvanceTime: true });
    vi.setSystemTime(new Date(2026, 6, 5, 12, 0, 0));
    const calls = stubApi({
      "GET /brief": () => jsonResponse(run),
      "GET /deals": () => jsonResponse({ data: [fleetDeal] }),
      "POST /brief/items/bi-1/snooze": () =>
        jsonResponse({
          ...run.items[0],
          state: "snoozed",
          state_at: "2026-07-05T06:00:00Z",
          snoozed_until: "2026-07-06T06:00:00Z",
        }),
    });
    const user = userEvent.setup();
    render(<HomeScreen />);

    await screen.findByText("Fleet retrofit");
    await user.click(screen.getByRole("button", { name: /Snooze/ }));
    await waitFor(() =>
      expect(writeRoutes(calls)).toEqual(["POST /brief/items/bi-1/snooze"]),
    );

    const sent = writes(calls)[0].body;
    const until = new Date(readSnoozedUntil(sent));
    expect(until.getTime()).toBeGreaterThan(Date.now());
    expect(until).toEqual(new Date(2026, 6, 6, 8, 0, 0));
    expect(await screen.findByText("snoozed")).toBeTruthy();
  });
});

// ── What the night said, and whether it spoke at all ──
//
// Three states, and the third is the one worth the tests. A brief with no
// narrative means either "the pass ran and had nothing to say" or "no pass ran
// at all" — and those read identically as silence. `annotated_at` is what
// separates them, so a screen that showed nothing in both cases would tell a
// rep the product had nothing to explain when in fact nobody looked.

describe("HomeScreen — the sentence about the night", () => {
  it("shows the narrative, marked as agent-authored", async () => {
    stubApi({
      "GET /brief": () =>
        jsonResponse({
          ...run,
          narrative: "Two replies overnight, one deal went quiet.",
          annotated_at: "2026-07-05T05:35:00Z",
        }),
      "GET /deals": () => jsonResponse({ data: [fleetDeal] }),
    });
    render(<HomeScreen />);

    await screen.findByText("Two replies overnight, one deal went quiet.");
    // The prose is model-authored and sits beside numbers a deterministic
    // engine computed; nothing else on the panel would tell them apart.
    expect(
      screen.getAllByText(en["trust.agentUnnamed"]).length,
    ).toBeGreaterThan(0);
  });

  it("says a pass did not run, rather than showing nothing", async () => {
    stubApi({
      "GET /brief": () =>
        jsonResponse({ ...run, narrative: null, annotated_at: null }),
      "GET /deals": () => jsonResponse({ data: [fleetDeal] }),
    });
    render(<HomeScreen />);

    // The honest degrade the plan asks for: never a blank morning, never a
    // silent one. A rep reading silence would conclude there was nothing to
    // explain.
    await screen.findByText(en["home.narrativeNoPass"]);
  });

  it("stays silent when a pass ran and had nothing to say", async () => {
    stubApi({
      "GET /brief": () =>
        jsonResponse({
          ...run,
          narrative: null,
          annotated_at: "2026-07-05T05:35:00Z",
        }),
      "GET /deals": () => jsonResponse({ data: [fleetDeal] }),
    });
    render(<HomeScreen />);

    await screen.findByText("Fleet retrofit");
    // A quiet night honestly has no sentence, and inventing one — or claiming
    // no pass ran — would both be false.
    expect(screen.queryByText(en["home.narrativeNoPass"])).toBeNull();
  });

  it("shows a per-item finding above the factor meters", async () => {
    stubApi({
      "GET /brief": () =>
        jsonResponse({
          ...run,
          annotated_at: "2026-07-05T05:35:00Z",
          items: [
            {
              ...run.items[0],
              finding: "He asked about the delivery date yesterday.",
            },
          ],
        }),
      "GET /deals": () => jsonResponse({ data: [fleetDeal] }),
    });
    render(<HomeScreen />);

    await screen.findByText("He asked about the delivery date yesterday.");
  });
});

// ── The one control that promised what the click could not do ──

describe("HomeScreen — the brief is generated, never re-ranked", () => {
  // POST /brief answers "today's run already existed; this is it, unchanged".
  // It assembles a brief where none exists and re-ranks nothing — so a control
  // labelled "Refresh brief" beside an existing run promised a re-rank the
  // click could not perform, and a rep who pressed it and saw the same order
  // concluded the ranking was stuck.
  it("offers no refresh once today's run exists", async () => {
    stubApi({
      "GET /brief": () => jsonResponse(run),
      "GET /deals": () => jsonResponse({ data: [fleetDeal] }),
    });
    render(<HomeScreen />);

    await screen.findByText("Fleet retrofit");
    expect(screen.queryByTestId("brief-refresh")).toBeNull();
  });

  // And the affordance that DOES something stays: a rep whose overnight pass
  // has not run has a real button to press, and it is the only one.
  it("offers to generate one when there is none", async () => {
    stubApi({
      "GET /brief": () => new Response(null, { status: 404 }),
      "GET /deals": () => jsonResponse({ data: [fleetDeal] }),
    });
    render(<HomeScreen />);

    const generate = await screen.findByTestId("brief-refresh");
    expect(generate.textContent).toContain(en["home.generate"]);
  });

  // And what it says WHILE it works names the same act. The button assembles a
  // first run; a pending label reading "Ranking…" describes re-ordering one
  // that already exists, which is the confusion the button's own wording was
  // changed to avoid. Nothing asserted this label, so the two drifted.
  it("names assembling, not ranking, while the run is being built", async () => {
    let releasePost: (() => void) | undefined;
    const posted = new Promise<void>((resolve) => {
      releasePost = resolve;
    });
    stubApi({
      "GET /brief": () => jsonResponse({ title: "Not Found" }, 404),
      "GET /deals": () => jsonResponse({ data: [fleetDeal] }),
      "POST /brief": async () => {
        await posted;
        return jsonResponse(run, 201);
      },
    });
    const user = userEvent.setup();
    render(<HomeScreen />);

    // Deliberately NOT awaited: the click's promise settles only once the write
    // does, and the pending label is what the button says in between.
    void user.click(await screen.findByTestId("brief-refresh"));
    expect(
      (await screen.findByText(en["home.generating"])).textContent,
    ).toContain(en["home.generating"]);

    releasePost?.();
  });
});
