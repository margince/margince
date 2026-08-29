/** @vitest-environment jsdom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render as rtlRender,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { meFixture } from "../app/mefixture";
import { MONEY_ABSENT } from "../format/format";
import { LocaleProvider } from "../i18n";
import { en } from "../i18n/en";
import { HomeScreen } from "./home";
import { HomeGlance } from "./home.glance";
import type { Deal, MorningBrief } from "./home.queries";

// Home — the morning handover, held to the three things its shape claims.
//
//   1. The DECK STAGES. A verdict a reader gives here does not leave the
//      browser: the assertion that proves it is a COUNT OF WRITES, and it is
//      zero until the explicit commit. Then exactly the staged verdicts go, a
//      bundle through its own endpoint once rather than per member, and an
//      `edit` sends nothing at all because the deck cannot edit — it hands the
//      reader to the Decisions screen instead.
//   2. The page's ORDER follows the day: decisions lead while any are waiting,
//      the ranked queue leads once the deck is clear.
//   3. Every reading is gated on its OWN read, and a reading still in flight is
//      absent rather than zero. "Nothing is waiting on you" is a claim, and a
//      page that makes it while the queue is still loading is lying.
//
// The clock is never the real one where a test depends on it: the greeting takes
// `now` as a prop and is driven by a fixed instant, and the one case that asks
// what "today" means pins the system clock. Every fixed instant is built with
// the LOCAL Date constructor, so a machine in any zone reads the hour the case
// is named for.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.useRealTimers();
  window.location.hash = "";
});

type Approval = components["schemas"]["Approval"];

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function render(ui: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">{ui}</LocaleProvider>
    </QueryClientProvider>,
  );
}

const emptyPage = { data: [], page: { next_cursor: null, has_more: false } };

/** One request the screen made, as the route it names and what it carried. */
type Call = { method: string; path: string; body: unknown };

type Routes = Record<string, (body: unknown) => Response | Promise<Response>>;

// Every read Home fans out to, answered honestly by default so each case
// declares only the route it is about: a session, no nightly digest, no brief
// run, and a pipeline report with no rows. The report matters — the fallback
// empty PAGE carries no `rows`, which the pipeline reading would read as a
// failure and put a refusal in the rail of every case in this file.
const DEFAULTS: Routes = {
  "GET /me": () => jsonResponse(meFixture()),
  "GET /brief": () => jsonResponse({ title: "Not Found" }, 404),
  "GET /digest": () =>
    jsonResponse({ title: "Not Found", code: "no_digest_yet" }, 404),
  "POST /reports/deals-by-stage": () =>
    jsonResponse({ report: "deals-by-stage", plan: {}, columns: [], rows: [] }),
};

/**
 * Routes the stubbed fetch by method+path and RECORDS every call, because what
 * this screen must not do is as load-bearing as what it must: a staged verdict
 * is only staged if nothing was sent, and no rendering can prove that.
 */
function stubApi(routes: Routes): Call[] {
  const calls: Call[] = [];
  const mock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const request = input instanceof Request ? input : null;
    const url = new URL(
      request ? request.url : String(input),
      "https://test.local",
    );
    const method = request?.method ?? init?.method ?? "GET";
    const path = url.pathname.replace(/^\/v1/, "");
    let body: unknown = null;
    if (method !== "GET") {
      try {
        // `clone()`: the client sends a Request and the handler below may read
        // the same body again.
        body = request
          ? await request.clone().json()
          : JSON.parse(String(init?.body));
      } catch {
        // A write with no body at all (POST /brief) is not a malformed one.
        body = null;
      }
    }
    calls.push({ method, path, body });
    const handler =
      routes[`${method} ${path}`] ?? DEFAULTS[`${method} ${path}`];
    return handler ? handler(body) : jsonResponse(emptyPage);
  });
  vi.stubGlobal("fetch", mock);
  return calls;
}

/**
 * Everything that left the browser as a write, in the order it went.
 *
 * `/reports/{report}` is excluded because it is a READ spelled as a POST — the
 * query plan does not fit a URL — and the rail runs one on every mount. Counting
 * it would make "nothing was sent" untrue of a page that sent nothing.
 */
function writes(calls: readonly Call[]): Call[] {
  return calls.filter(
    (call) => call.method !== "GET" && !call.path.startsWith("/reports/"),
  );
}

/**
 * The `snoozed_until` a write carried, narrowed rather than asserted: a body is
 * `unknown` here because it came off the wire, and casting one into shape hides
 * the case where the field never went at all.
 */
function readSnoozedUntil(body: unknown): string {
  if (
    typeof body !== "object" ||
    body === null ||
    !("snoozed_until" in body) ||
    typeof body.snoozed_until !== "string"
  ) {
    throw new Error(
      `the snooze write carried no instant: ${JSON.stringify(body)}`,
    );
  }
  return body.snoozed_until;
}

/** The routes those writes named, which is what a commit is judged on. */
function writeRoutes(calls: readonly Call[]): string[] {
  return writes(calls).map((call) => `${call.method} ${call.path}`);
}

/** Home's two work sections, in the order the document holds them. */
function workOrder(): string[] {
  return [...document.querySelectorAll("#home-decisions, #home-today")].map(
    (section) => section.id,
  );
}

const fleetDeal: Deal = {
  id: "d-1",
  name: "Fleet retrofit",
  amount_minor: 4_800_000,
  currency: "EUR",
  pipeline_id: "pl",
  stage_id: "s2",
  status: "open",
  stalled: false,
  source: "manual",
  captured_by: "human:u1",
  version: 1,
  created_at: "2026-05-01T08:00:00Z",
  updated_at: "2026-06-01T08:00:00Z",
};

const run: MorningBrief = {
  id: "br-1",
  generated_at: "2026-07-05T05:30:00Z",
  as_of: "2026-07-05T05:00:00Z",
  candidate_count: 1,
  items: [
    {
      id: "bi-1",
      deal_id: "d-1",
      rank: 1,
      composite: 0.74,
      feature_vector: {
        winnability: 0.4,
        revenue: 1,
        timing: 0.75,
        momentum: 1,
        warmth: 0.47,
      },
      evidence_ids: ["ev-1", "ev-2"],
      state: "new",
      state_at: null,
    },
  ],
};

/** One staged proposal, named by the sentence its card leads with. */
function proposal(id: string, summary: string, over: Partial<Approval> = {}) {
  const staged: Approval = {
    id,
    kind: "send_email",
    status: "pending",
    proposed_by: "agent:runner",
    summary,
    proposed_change: { body: "Hi — shall we sync next week?" },
    created_at: "2026-07-05T05:00:00Z",
    ...over,
  };
  return staged;
}

/** The pending queue, minus whatever the case has had decided. */
function pendingPage(queue: readonly Approval[], decided: ReadonlySet<string>) {
  return jsonResponse({
    data: queue.filter((approval) => !decided.has(approval.id)),
    page: { next_cursor: null, has_more: false },
  });
}

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

    await waitFor(() => expect(window.location.hash).toBe("#/today"));
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
    expect(workOrder()).toEqual(["home-decisions", "home-today"]);
  });

  it("leads with the ranked queue once the deck is clear", async () => {
    stubApi({
      "GET /brief": () => jsonResponse(run),
      "GET /deals": () => jsonResponse({ data: [fleetDeal] }),
    });
    render(<HomeScreen />);

    await screen.findByText("Fleet retrofit");
    expect(workOrder()).toEqual(["home-today", "home-decisions"]);
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
          firstName={firstName}
          now={new Date(2026, 6, 5, hour, 30, 0)}
          decisions={null}
          brief={null}
          overnight={null}
          stalled={null}
          onGoToDecisions={() => {}}
          onGoToToday={() => {}}
          onGoToDuplicates={() => {}}
          onGoToWatch={() => {}}
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
          firstName="Ada"
          now={new Date(2026, 6, 5, 9, 0, 0)}
          decisions={null}
          brief={null}
          overnight={null}
          stalled={null}
          onGoToDecisions={() => {}}
          onGoToToday={() => {}}
          onGoToDuplicates={() => {}}
          onGoToWatch={() => {}}
        />
      </LocaleProvider>,
    );
    // Not one of them, and in particular not the "nothing is waiting" claim: an
    // unread queue is not an empty one.
    expect(screen.queryByTestId("glance-decisions")).toBeNull();
    expect(screen.queryByTestId("glance-ranked")).toBeNull();
    expect(screen.queryByTestId("glance-captured")).toBeNull();
    expect(screen.queryByTestId("glance-quiet")).toBeNull();
    expect(screen.queryByText("Nothing is waiting on you.")).toBeNull();
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

    // The other four reads land, so the strip is drawn and its silence about
    // decisions is a choice rather than a page that has not started.
    const strip = await screen.findByTestId("home-readings");
    await waitFor(() =>
      expect(within(strip).getByText("Gone quiet")).toBeTruthy(),
    );
    expect(within(strip).queryByText("Waiting on you")).toBeNull();
    expect(within(strip).queryByText("none expire today")).toBeNull();
    expect(screen.queryByText("Nothing is waiting on you.")).toBeNull();
    expect(screen.queryByTestId("glance-decisions")).toBeNull();
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

    const strip = await screen.findByTestId("home-readings");
    await waitFor(() =>
      expect(within(strip).getByText("Gone quiet")).toBeTruthy(),
    );
    expect(within(strip).queryByText("Waiting on you")).toBeNull();
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
  it("reports a deal reading off a full page as a floor rather than a total", async () => {
    stubApi({
      "GET /deals": () =>
        jsonResponse({
          data: [fleetDeal, { ...fleetDeal, id: "d-2", stalled: true }],
          page: { has_more: true },
        }),
    });
    render(<HomeScreen />);

    const strip = await screen.findByTestId("home-readings");
    await waitFor(() =>
      expect(within(strip).getByText("Open deals")).toBeTruthy(),
    );
    // Two open deals on the page and another page behind them: both the open
    // count and the quiet count say so where the figure is read.
    expect(within(strip).getByText("2+")).toBeTruthy();
    expect(within(strip).getByText("1+")).toBeTruthy();
    // And the panel that LISTS them says the list is part of one, rather than
    // making the "nothing has gone quiet" claim it has no grounds for.
    expect(screen.getByText("Showing part of the list")).toBeTruthy();
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

    const strip = await screen.findByTestId("home-readings");
    await waitFor(() =>
      expect(within(strip).getByText("Waiting on you")).toBeTruthy(),
    );
    expect(within(strip).getByText("2")).toBeTruthy();
    expect(within(strip).getByText("1 expires today")).toBeTruthy();
    expect(screen.getByTestId("glance-expiring").textContent).toContain("1");
  });
});

// ── The ranked queue ──

describe("HomeScreen — the ranked queue", () => {
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

// ── The week just gone ──

describe("HomeScreen — the weekly retrospective", () => {
  const review = {
    id: "01a04000-0000-7000-8000-00000000000a",
    local_week_start: "2026-06-29",
    generated_at: "2026-07-06T06:00:00Z",
    as_of: "2026-07-06T06:00:00Z",
    counts: {
      tasks_due: 5,
      tasks_done: 4,
      tasks_carried_over: 2,
      deals_moved: 3,
      deals_won: 1,
      deals_lost: 1,
      proposals_accepted: 7,
      proposals_rejected: 2,
      brief_items_acted: 6,
      brief_items_dismissed: 3,
    },
    deals: [
      {
        deal_id: "01a04000-0000-7000-8000-00000000000b",
        label: "Weber Rahmenvertrag",
        outcome: "won",
        occurred_at: "2026-07-02T14:00:00Z",
      },
    ],
  };

  it("shows the week's tallies and its frozen deal lines", async () => {
    stubApi({
      "GET /weekly-reviews/latest": () => jsonResponse(review),
      "GET /weekly-reviews": () => jsonResponse({ weeks: ["2026-06-29"] }),
      "GET /deals": () => jsonResponse({ data: [fleetDeal] }),
    });
    render(<HomeScreen />);

    // The label is what the deal was CALLED that week, served from the frozen
    // row rather than looked up — which is why it renders with no deal in the
    // deals payload at all.
    await screen.findByText("Weber Rahmenvertrag");
    expect(screen.getByText(en["home.weekly.promised"])).toBeTruthy();
  });

  it("says there is no review yet rather than drawing a week of zeroes", async () => {
    stubApi({
      "GET /weekly-reviews/latest": () =>
        jsonResponse({ title: "Not Found" }, 404),
      "GET /weekly-reviews": () => jsonResponse({ weeks: [] }),
      "GET /deals": () => jsonResponse({ data: [fleetDeal] }),
    });
    render(<HomeScreen />);

    // A page of zeroes would claim a week that was measured and empty.
    await screen.findByText(en["home.weekly.none"]);
  });

  it("survives a payload that is not a review", async () => {
    stubApi({
      // The shape an unrouted read answers with: a list page, not a review.
      "GET /weekly-reviews/latest": () =>
        jsonResponse({
          data: [],
          page: { next_cursor: null, has_more: false },
        }),
      "GET /brief": () => jsonResponse(run),
      "GET /deals": () => jsonResponse({ data: [fleetDeal] }),
    });
    render(<HomeScreen />);

    // The panel formats local_week_start immediately, so a half-shaped answer
    // used to take Home's whole render down with it — the queue, the deck and
    // everything else — rather than drawing one honest empty section.
    await screen.findByText("Fleet retrofit");
  });
});

describe("HomeScreen — the week's sentence", () => {
  const narrated = {
    id: "01a04000-0000-7000-8000-00000000000a",
    local_week_start: "2026-06-29",
    generated_at: "2026-07-06T06:00:00Z",
    as_of: "2026-07-06T06:00:00Z",
    counts: {
      tasks_due: 5,
      tasks_done: 4,
      tasks_carried_over: 2,
      deals_moved: 3,
      deals_won: 1,
      deals_lost: 1,
      proposals_accepted: 7,
      proposals_rejected: 2,
      brief_items_acted: 6,
      brief_items_dismissed: 3,
    },
    deals: [],
  };

  it("shows the sentence, marked as agent-authored", async () => {
    stubApi({
      "GET /weekly-reviews/latest": () =>
        jsonResponse({
          ...narrated,
          narrative: "Weber signed; two promises slipped to this week.",
          narrated_at: "2026-07-06T06:01:00Z",
        }),
      "GET /weekly-reviews": () => jsonResponse({ weeks: ["2026-06-29"] }),
      "GET /brief": () => jsonResponse(run),
      "GET /deals": () => jsonResponse({ data: [fleetDeal] }),
    });
    render(<HomeScreen />);

    await screen.findByText("Weber signed; two promises slipped to this week.");
    // Model-authored prose sitting beside numbers a deterministic pass
    // computed; nothing else on the panel would tell them apart.
    expect(
      screen.getAllByText(en["trust.agentUnnamed"]).length,
    ).toBeGreaterThan(0);
  });

  it("says no pass ran, rather than showing nothing", async () => {
    stubApi({
      "GET /weekly-reviews/latest": () =>
        jsonResponse({ ...narrated, narrative: null, narrated_at: null }),
      "GET /weekly-reviews": () => jsonResponse({ weeks: ["2026-06-29"] }),
      "GET /brief": () => jsonResponse(run),
      "GET /deals": () => jsonResponse({ data: [fleetDeal] }),
    });
    render(<HomeScreen />);

    // Never a blank week, never a silent one: the counts are still the week's,
    // and a rep reading silence would conclude there was nothing to remark on.
    await screen.findByText(en["home.weekly.noNarrative"]);
  });

  it("stays silent when a pass ran and had nothing to add", async () => {
    stubApi({
      "GET /weekly-reviews/latest": () =>
        jsonResponse({
          ...narrated,
          narrative: null,
          narrated_at: "2026-07-06T06:01:00Z",
        }),
      "GET /weekly-reviews": () => jsonResponse({ weeks: ["2026-06-29"] }),
      "GET /brief": () => jsonResponse(run),
      "GET /deals": () => jsonResponse({ data: [fleetDeal] }),
    });
    render(<HomeScreen />);

    await screen.findByText(en["home.weekly.promised"]);
    // A pass that honestly found nothing is not a pass that never ran, and
    // claiming otherwise would tell the rep their week was never looked at.
    expect(screen.queryByText(en["home.weekly.noNarrative"])).toBeNull();
  });
});
