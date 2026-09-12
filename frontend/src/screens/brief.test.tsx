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
import { formatTimeOfDay } from "../format/format";
import { viewerZone } from "../format/timezone";
import { LocaleProvider } from "../i18n";
import { en } from "../i18n/en";
import { BriefScreen } from "./brief";
import { overnightRow, readingsDay } from "./brief.fixtures";
import { BriefGlance } from "./brief.glance";
import { leadOf } from "./brief.sentence";
import {
  fleetDeal,
  jsonResponse,
  pendingPage,
  proposal,
  render,
  run,
  stubApi,
  workOrder,
  writeRoutes,
  writes,
} from "./brief.testkit";
import type { BriefView } from "./brief.view";
import { rowHref } from "./worklist.copy";
import type { Worklist } from "./worklist.queries";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.useRealTimers();
  window.location.hash = "";
});

// ── The deck: staging is local, the commit is the only thing that sends ──

describe("BriefScreen — the deck stages, and only the commit sends", () => {
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
    render(<BriefScreen />);
    // The list is what the Brief opens on now (decisiondeck.tsx says why);
    // this case is about the DECK, so it opens the deck as a reader would.
    await user.click(await screen.findByRole("button", { name: "Deck" }));

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
    render(<BriefScreen />);

    // Three proposals, ONE card. The card is headlined by the member it
    // represents, says how much saying yes decides, and keeps the other two
    // behind an expander rather than drawing three questions.
    expect(await screen.findByText("One decision · 3 items")).toBeTruthy();
    expect(document.querySelectorAll(".dcard").length).toBe(1);
    expect(document.querySelector(".approval-headline")?.textContent).toBe(
      "Publish the acme.example facts",
    );
    // Brief draws a LINE per decision, so what is being proposed is behind one
    // control rather than on the row: the page opens with the decisions and
    // goes on to the day's own work, and a reader is passing through.
    await user.click(
      screen.getByRole("button", { name: en["brief.deck.rowDetail"] }),
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
    render(<BriefScreen />);

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
    render(<BriefScreen />);

    await screen.findByText("Send the Weber follow-up");
    // Accept and reject are on the line; the rarer verdicts are folded into a
    // menu, because a dense row has no space to spell four of them.
    await user.click(
      screen.getByRole("button", { name: en["brief.deck.rowMore"] }),
    );
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
    render(<BriefScreen />);
    // The list is what the Brief opens on now (decisiondeck.tsx says why);
    // this case is about the DECK, so it opens the deck as a reader would.
    await user.click(await screen.findByRole("button", { name: "Deck" }));

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
          approval_token: "example-brief-token",
        });
      },
    });
    const user = userEvent.setup();
    render(<BriefScreen />);

    await screen.findByText("Send the Weber follow-up");
    await user.click(screen.getByRole("button", { name: "Accept" }));
    await user.click(
      screen.getByRole("button", { name: "Send staged decisions" }),
    );

    await waitFor(() =>
      expect(screen.queryByText("Send the Weber follow-up")).toBeNull(),
    );
    expect(screen.queryByText("example-brief-token")).toBeNull();
  });
});

// ── The page's order follows the day ──

describe("BriefScreen — the order of the page follows the day", () => {
  it("leads with the decisions while any are waiting", async () => {
    stubApi({
      "GET /approvals": () =>
        pendingPage([proposal("ap-1", "Send the Weber follow-up")], new Set()),
      "GET /brief": () => jsonResponse(run),
      "GET /deals": () => jsonResponse({ data: [fleetDeal] }),
    });
    render(<BriefScreen />);

    await screen.findByText("Send the Weber follow-up");
    expect(workOrder()).toEqual(["brief-decisions", "brief-today"]);
  });

  it("leads with the ranked queue once the deck is clear", async () => {
    stubApi({
      "GET /brief": () => jsonResponse(run),
      "GET /deals": () => jsonResponse({ data: [fleetDeal] }),
      // A row in the one feed, so the assertion waits on the surface it is
      // about rather than on a panel that no longer exists.
      "GET /worklist": () =>
        jsonResponse(readingsDay({}, [overnightRow("bi-1", "d-1")])),
    });
    render(<BriefScreen />);

    await screen.findByRole("region", { name: en["brief.feed.title"] });
    expect(workOrder()).toEqual(["brief-today", "brief-decisions"]);
  });
});

// ── The greeting, on a clock the test owns ──

describe("BriefGlance — the greeting follows the reader's own hour", () => {
  // A local-time instant, so the hour the case names is the hour the reader's
  // own zone reports whatever machine this runs on.
  function greetingAt(hour: number, firstName: string | null): string {
    const view = rtlRender(
      <LocaleProvider initial="en">
        <BriefGlance
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
        <BriefGlance
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

describe("BriefGlance — the weekly's sentence comes from the closed week", () => {
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
  } as unknown as Parameters<typeof BriefGlance>[0]["week"];

  function sentenceOf(
    view: BriefView,
    week: Parameters<typeof BriefGlance>[0]["week"],
  ): string | null {
    const rendered = rtlRender(
      <LocaleProvider initial="en">
        <BriefGlance
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

// ── The opening block is two lines, and one of them is a control ──

describe("BriefGlance — the greeting and the sentence, and nothing between", () => {
  // Two rows that each carry a record of their own, so the sentence has a lead
  // to link and a tail to count. The strip's default day is two meetings with
  // no subject between them, which is a sentence with nothing to open.
  function linkedDay(): Worklist {
    return readingsDay({}, [
      overnightRow("o1", "d-aster"),
      overnightRow("o2", "d-fleet"),
    ]);
  }

  function drawGlance(view: BriefView, day: Worklist | undefined) {
    return rtlRender(
      <LocaleProvider initial="en">
        <BriefGlance
          view={view}
          firstName="Ada"
          now={new Date(2026, 6, 5, 9, 0, 0)}
          day={day}
          week={undefined}
        />
      </LocaleProvider>,
    );
  }

  // WHAT THIS BLOCK NO LONGER DRAWS, and the three things are one rule: a
  // reader already knew them. The view is the dial's own state, drawn beside
  // this block; the date is the shell's; and an as-of that ticked every sixty
  // seconds re-rendered the page's opening for a digit nobody was reading.
  //
  // Asserted on the block's SHAPE rather than on the absence of three strings:
  // an absence check passes on any rewording of the same three lines, while a
  // heading plus one paragraph is the whole claim.
  it("draws the greeting and one line under it, and nothing else", () => {
    const day = readingsDay({});
    drawGlance("morning", day);

    const glance = screen.getByTestId("brief-glance");
    expect(glance.children).toHaveLength(2);
    expect(glance.firstElementChild?.tagName).toBe("H1");
    // No uppercase kicker above the greeting, and no clock anywhere: the moment
    // the queue was read was the one thing here that moved on its own.
    expect(glance.querySelector(".t-eyebrow")).toBeNull();
    expect(glance.textContent).not.toContain(
      formatTimeOfDay(day.as_of, "en", viewerZone()),
    );
    // And no date line. Today's date under a greeting that already says which
    // part of today it is tells a reader what their own machine tells them.
    expect(glance.textContent).not.toContain("July");
  });

  // The weekly draws the SAME two lines. Two views drawn alike need no kicker
  // to tell them apart, and one of them wearing one would be the odd page.
  it("draws the same two lines over the weekly", () => {
    drawGlance("weekly", readingsDay({}));

    expect(screen.getByTestId("brief-glance").children).toHaveLength(2);
  });

  // THE LEAD IS THE WAY INTO THE ROW IT DESCRIBES. The sentence names one row
  // and the section below draws it; before this the reader had to find it again
  // down the page. The href is the row's OWN, through the same helper the feed's
  // rows are linked by, so the two cannot send a reader to two places.
  it("opens the named lead's own record from the sentence", () => {
    const day = linkedDay();
    drawGlance("morning", day);

    const lead = leadOf(day);
    if (lead === undefined) {
      throw new Error("the fixture has no lead for the sentence to name");
    }
    const link = screen.getByTestId("glance-sentence").querySelector("a[href]");
    if (!(link instanceof HTMLAnchorElement)) {
      throw new Error("the sentence names no lead to open");
    }
    expect(link.getAttribute("href")).toBe(rowHref(lead));
  });

  // AND THE TAIL REACHES THE DAY'S OWN ORDER. "Then 4 more" is where a reader
  // goes next, and the whole clause is the control — a bare numeral is a
  // two-character press target that reads as a figure rather than as a way
  // anywhere.
  //
  // A button rather than an anchor, and the address bar is the reason: every
  // href in this product is a route, so a fragment link would replace `#/brief`
  // and send the reader off the page the tail is pointing at.
  // ONE SENTENCE, not a sentence with a control dropped into it.
  //
  // Two halves, and only one of them is a test. The DOM half: no text node
  // between the control and the stop, so the sentence is not assembled with a
  // separator somebody could widen. The CSS half — `.link-button` is an
  // inline-flex box with side padding, which printed "Then 24 more ." with a
  // space before the stop and raised the line the clamp then reserved two of —
  // is held by `glance-rest` being ON the control, because jsdom computes
  // nothing from a stylesheet and an assertion about the rendered gap here
  // would pass whatever the sheet said. `brief.css` carries the rule and says
  // why; this is the hook it keys on.
  it("closes the sentence with no gap before the stop", () => {
    drawGlance("morning", linkedDay());

    const sentence = screen.getByTestId("glance-sentence");
    const tail = within(sentence).getByRole("button");
    expect(tail.className).toContain("glance-rest");
    expect(tail.nextSibling?.textContent).toBe(".");
    expect(sentence.textContent).toMatch(/\d+ more\.$/);
  });

  it("reaches the day's own order from the tail of the sentence", async () => {
    const day = linkedDay();
    const section = document.createElement("section");
    section.id = "brief-today";
    section.scrollIntoView = vi.fn();
    document.body.append(section);
    try {
      drawGlance("morning", day);
      const tail = within(screen.getByTestId("glance-sentence")).getByRole(
        "button",
      );

      await userEvent.setup().click(tail);

      expect(section.scrollIntoView).toHaveBeenCalled();
    } finally {
      section.remove();
    }
  });
});

// ── A reading is absent until it is read. Never zero. ──

// ── A bundle's chips are claims about the ACT, not about the drawn member ──

describe("BriefScreen — a bundle says only what its members agree on", () => {
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
    render(<BriefScreen />);

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
    render(<BriefScreen />);

    expect(await screen.findByText("One decision · 2 items")).toBeTruthy();
    expect(screen.getByText("Automated by runner")).toBeTruthy();
  });
});

describe("BriefScreen — a reading in flight is absent, not zero", () => {
  /** The deck's own section, which is where a failed decisions read belongs. */
  function deckSection(): HTMLElement {
    const section = document.getElementById("brief-decisions");
    if (!section) {
      throw new Error("Brief rendered no decisions section");
    }
    return section;
  }

  it("draws no decisions tile while the queue is still loading, and keeps the ranked queue", async () => {
    stubApi({
      // Never settles: the queue is in flight for the whole case.
      "GET /approvals": () => new Promise<Response>(() => {}),
      "GET /brief": () => jsonResponse(run),
      "GET /deals": () => jsonResponse({ data: [fleetDeal] }),
      "GET /worklist": () =>
        jsonResponse(readingsDay({}, [overnightRow("bi-1", "d-1")])),
    });
    render(<BriefScreen />);

    // The other reads land, so the page is drawn and its silence about decisions
    // is a choice rather than a page that has not started. The strip is the
    // witness that the page rendered: it is drawn from the worklist answer, not
    // from the queue this case leaves in flight.
    await screen.findByTestId("brief-readings");
    expect(screen.queryByText("Nothing is waiting on you.")).toBeNull();
    // The wait belongs to the deck alone. Five independent reads exist so that
    // one of them being slow cannot blank the other four.
    expect(deckSection().querySelector("[aria-busy='true']")).toBeTruthy();
    // The day's own feed is drawn and populated: a slow decisions read blanks
    // the deck and nothing else.
    expect(
      screen.getByRole("region", { name: en["brief.feed.title"] }),
    ).toBeTruthy();
    expect(document.querySelector(".brief-feed-list")).toBeTruthy();
  });

  it("shows a failed decisions read in the deck alone, with the ranked queue intact", async () => {
    stubApi({
      "GET /approvals": () => jsonResponse({ title: "Server Error" }, 500),
      "GET /brief": () => jsonResponse(run),
      "GET /deals": () => jsonResponse({ data: [fleetDeal] }),
      "GET /worklist": () =>
        jsonResponse(readingsDay({}, [overnightRow("bi-1", "d-1")])),
    });
    render(<BriefScreen />);

    await screen.findByTestId("brief-readings");
    // Not "nothing is waiting": a queue that could not be read is not an empty
    // one, and the deck says which of the two this is.
    expect(screen.queryByText("Nothing is waiting on you.")).toBeNull();
    expect(
      within(deckSection()).getByText("This section did not load."),
    ).toBeTruthy();
    // A healthy ranked queue is not hidden behind a failed decisions read.
    expect(
      screen.getByRole("region", { name: en["brief.feed.title"] }),
    ).toBeTruthy();
    expect(document.querySelector(".brief-feed-list")).toBeTruthy();
  });

  // Brief reads ONE page of deals. Past it every reading taken from those rows is
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
    render(<BriefScreen />);

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
    const user = userEvent.setup();
    render(<BriefScreen />);
    // The list is what the Brief opens on now (decisiondeck.tsx says why);
    // this case is about the DECK, so it opens the deck as a reader would.
    await user.click(await screen.findByRole("button", { name: "Deck" }));

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
    // (brief.tsx expiringToday) went with it. Whether the deck should say that
    // across the whole queue is a product question, filed rather than guessed.
    expect(within(deckSection()).getByText(/expires/i)).toBeTruthy();
  });
});
