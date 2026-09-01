/** @vitest-environment jsdom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { type Locale, LocaleProvider } from "../i18n";
import { WorklistScreen } from "./worklist";

// The ranked queue, and the ways it can mislead the person reading it.
//
// Every case here is one promise the page makes: that the order is readable,
// that a figure describes the rows beneath it, that nothing is drawn to report
// a zero, and that the database's own words never reach the screen.

type Worklist = components["schemas"]["Worklist"];
type WorklistItem = components["schemas"]["WorklistItem"];

function jsonResponse(body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "content-type": "application/json" },
  });
}

// The queue, plus the one approval a decision row fetches whole. A row sends a
// sentence; deciding needs the payload, the stager and the evidence, so the row
// being decided reads the approval it is showing.
function stub(day: Worklist, approval?: unknown) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input instanceof Request ? input.url : input);
      if (url.includes("/worklist")) {
        return jsonResponse(day);
      }
      if (approval && /\/approvals\/[^/]+$/.test(url.split("?")[0])) {
        return jsonResponse(approval);
      }
      return jsonResponse({ data: [] });
    }),
  );
}

function renderWorklist(locale: Locale = "en") {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial={locale}>
        <WorklistScreen />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

function day(over: Partial<Worklist> = {}): Worklist {
  return {
    as_of: "2026-08-31T09:00:00Z",
    scope: "mine",
    scope_options: ["mine"],
    queue: [],
    summary: { urgent: 0, due: 0, lower_priority: 0, total: 0 },
    sources_unavailable: [],
    reach: [],
    counts: [],
    ...over,
  };
}

function row(over: Partial<WorklistItem> = {}): WorklistItem {
  return {
    id: "row-1",
    source: "task",
    category: "tasks",
    level: 4,
    consequence: "task_slips",
    because: [],
    actions: [],
    ...over,
  };
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("what the ranked queue tells a reader", () => {
  it("draws no panel to report a zero", async () => {
    stub(day());
    const { container } = renderWorklist();

    await screen.findByText("Nothing is waiting on you.");

    // Topology, not headings: a panel drawn to say "none" is the thing the
    // concept asks us to remove, whatever words it carries.
    expect(container.querySelectorAll(".panel")).toHaveLength(0);
  });

  it("names the record when the title alone cannot be told apart", async () => {
    stub(
      day({
        queue: [
          row({
            id: "t1",
            title: "Follow up with the new lead",
            subject: {
              type: "lead",
              id: "01a05500-0000-7000-8000-000000000001",
              label: "Katrin Seibert",
            },
          }),
          row({
            id: "t2",
            title: "Follow up with the new lead",
            subject: {
              type: "lead",
              id: "01a05500-0000-7000-8000-000000000002",
              label: "Philipp Hartwig",
            },
          }),
        ],
        summary: { urgent: 0, due: 2, lower_priority: 0, total: 2 },
      }),
    );
    renderWorklist();

    // Eight rows reading the same sentence cannot be ordered or chosen between.
    // The name is already on the row; only the title discarded it.
    expect(await screen.findByText(/Katrin Seibert/)).toBeTruthy();
    expect(screen.getByText(/Philipp Hartwig/)).toBeTruthy();
  });

  // The server sends the deal's name in `detail` — the title has nowhere to
  // put it, since a notice's subject is never set.
  it("renders the server's one supporting line, even when the title has no subject to name", async () => {
    stub(
      day({
        queue: [
          row({
            id: "n1",
            source: "notice",
            title: "A deal you own changed stage",
            detail: "Acme Renewal moved to a new pipeline stage.",
          }),
        ],
        summary: { urgent: 0, due: 1, lower_priority: 0, total: 1 },
      }),
    );
    renderWorklist();

    expect(
      await screen.findByText("A deal you own changed stage"),
    ).toBeTruthy();
    expect(
      screen.getByText("Acme Renewal moved to a new pipeline stage."),
    ).toBeTruthy();
  });

  it("does not repeat a name the title already carries", async () => {
    stub(
      day({
        queue: [
          row({
            title: "Call Anna Weber about the renewal",
            subject: {
              type: "person",
              id: "01a05500-0000-7000-8000-000000000003",
              label: "Anna Weber",
            },
          }),
        ],
        summary: { urgent: 0, due: 1, lower_priority: 0, total: 1 },
      }),
    );
    renderWorklist();

    const title = await screen.findByText(/Call Anna Weber/);
    expect(title.textContent).not.toContain("· Anna Weber");
  });

  it("draws one control per destination, not one per verb", async () => {
    stub(
      day({
        queue: [
          row({
            title: "Send the proposal",
            subject: {
              type: "person",
              id: "01a05500-0000-7000-8000-000000000009",
              label: "Anna Weber",
            },
            // Both verbs open the same record; the row drew two "Open"s.
            actions: ["complete", "snooze"],
          }),
        ],
        summary: { urgent: 0, due: 1, lower_priority: 0, total: 1 },
      }),
    );
    renderWorklist();

    await screen.findByText(/Send the proposal/);
    expect(screen.getAllByRole("link", { name: "Open" })).toHaveLength(1);
  });

  it("draws a pile of routine decisions as one row that says how many", async () => {
    stub(
      day({
        queue: [
          row({
            id: "likely_automated",
            source: "batch",
            category: "decisions",
            level: 6,
            consequence: "data_drifts",
            batch: {
              key: "likely_automated",
              count: 43,
              sample: [
                "Is noreply@x.com a contact?",
                "Is bot@y.com a contact?",
              ],
            },
          }),
        ],
        summary: { urgent: 0, due: 0, lower_priority: 1, total: 1 },
      }),
    );
    renderWorklist();

    // The row IS the sentence: a reader decides whether to open it from this
    // alone, so the count is part of the name rather than a badge beside it.
    expect(await screen.findByText("43 likely automated senders")).toBeTruthy();
    // And it names some of what it holds, because a group nobody can see into
    // is a group nobody trusts.
    expect(screen.getByText(/Is noreply@x.com a contact\?/)).toBeTruthy();
  });

  it("says a bounded count is a floor, not a total", async () => {
    stub(
      day({
        queue: [
          row({
            id: "held_draft",
            source: "batch",
            category: "decisions",
            level: 6,
            consequence: "data_drifts",
            batch: { key: "held_draft", count: 200, at_least: true },
          }),
        ],
        summary: { urgent: 0, due: 0, lower_priority: 1, total: 1 },
      }),
    );
    renderWorklist();

    // The read stopped at its own bound with members left over. "200" would be
    // a wrong number; "200+" is a bounded one.
    expect(
      await screen.findByText("200+ drafts waiting to be sent"),
    ).toBeTruthy();
  });

  it("opens a group into the work it stands for", async () => {
    const user = userEvent.setup();
    stub(
      day({
        queue: [
          row({
            id: "likely_automated",
            source: "batch",
            category: "decisions",
            level: 6,
            consequence: "data_drifts",
            batch: { key: "likely_automated", count: 43 },
          }),
        ],
        summary: { urgent: 0, due: 0, lower_priority: 1, total: 1 },
      }),
    );
    renderWorklist();

    await screen.findByText("43 likely automated senders");
    await user.click(screen.getByRole("button", { name: "Review" }));

    // A row whose verb led nowhere would be worse than the pile it replaced,
    // so this asserts the queue actually narrows rather than that a control
    // exists.
    await waitFor(() => {
      const calls = (globalThis.fetch as ReturnType<typeof vi.fn>).mock.calls;
      const urls = calls.map((call) => {
        const target = call[0];
        return target instanceof Request ? target.url : String(target);
      });
      expect(urls.some((url) => url.includes("filter=decisions"))).toBe(true);
    });
  });

  it("states the money and offers the reply the product already worked out", async () => {
    stub(
      day({
        queue: [
          row({
            title: "Re: pricing for the retrofit",
            source: "customer_waiting",
            category: "customer_waiting",
            level: 1,
            consequence: "buyer_waits",
            deal: { amount_minor: 16010000, currency: "EUR" },
            subject: {
              type: "deal",
              id: "01a05500-0000-7000-8000-00000000bbbb",
              label: "Acme Expansion",
            },
            move: {
              action: "draft_reply",
              activity_id: "01a05500-0000-7000-8000-00000000aaaa",
            },
          }),
        ],
        summary: { urgent: 1, due: 0, lower_priority: 0, total: 1 },
      }),
    );
    renderWorklist();

    // The concept's sharpest example: a €160,100 deal reduced to "no contact
    // for 83 days". The money was on the wire the whole time.
    expect(await screen.findByText(/160,100/)).toBeTruthy();
    const reply = screen.getByRole("link", { name: "Open to reply" });
    // The verb says where it GOES, because that is what pressing it does: the
    // composer lives on the record behind its own button, and a label promising
    // a draft would overstate the click.
    expect(reply.getAttribute("href")).toBe(
      "#/deals/01a05500-0000-7000-8000-00000000bbbb",
    );
  });

  it("sends a privacy request to the screen it is worked on", async () => {
    stub(
      day({
        queue: [
          row({
            id: "01a05500-0000-7000-8000-00000000dddd",
            source: "dsr",
            category: "system",
            consequence: "legal_deadline_missed",
          }),
        ],
        summary: { urgent: 1, due: 0, lower_priority: 0, total: 1 },
      }),
    );
    renderWorklist();

    // The row names no record — a request is worked on the privacy screen and
    // nowhere else — so without this it is a legal clock a reader is told
    // about and cannot follow.
    const request = await screen.findByRole("link", {
      name: "An open privacy request",
    });
    expect(request.getAttribute("href")).toBe("#/settings/admin/privacy");
  });

  it("says what happens if the reader does nothing", async () => {
    stub(
      day({
        queue: [row({ title: "Send the proposal" })],
        summary: { urgent: 0, due: 1, lower_priority: 0, total: 1 },
      }),
    );
    renderWorklist();

    expect(
      await screen.findByText("If you do nothing, it slips."),
    ).toBeTruthy();
  });

  it("says why a row sits above the one below it", async () => {
    stub(
      day({
        queue: [
          row({
            id: "a",
            title: "Closing tomorrow",
            above_next: {
              comparator: "deadline",
              mine: { kind: "date", date: "2026-09-01T09:00:00Z" },
              theirs: { kind: "date", date: "2027-05-01T09:00:00Z" },
            },
          }),
          row({ id: "b", title: "Closing later" }),
        ],
        summary: { urgent: 0, due: 0, lower_priority: 0, total: 2 },
      }),
    );
    const { container } = renderWorklist();

    expect(await screen.findByText(/Above the next:/)).toBeTruthy();
    // The server's order is the page's order. Rendering the rows sorted or
    // reversed would still show a reason line, so the DOM order is what this
    // asserts.
    const titles = Array.from(
      container.querySelectorAll(".worklist-row-title"),
    ).map((node) => node.textContent ?? "");
    expect(titles[0]).toContain("Closing tomorrow");
    expect(titles[1]).toContain("Closing later");
  });

  it("never prints the database's own words at a reader", async () => {
    stub(
      day({
        queue: [
          row({
            id: "raw",
            source: "approval",
            category: "decisions",
            level: 6,
            consequence: "data_drifts",
            kind: "capture_counterparty_verdict",
            because: [{ kind: "routine" }],
          }),
        ],
        summary: { urgent: 0, due: 0, lower_priority: 1, total: 1 },
      }),
    );
    const { container } = renderWorklist();

    await screen.findByText("A decision is waiting");
    // Named values, not a blanket ban on underscores: a real title may carry
    // one ("ACME_Q3"), and the raw words that actually leak — a kind, a source,
    // a reason, an i18n key — mostly carry none.
    const text = container.textContent ?? "";
    for (const raw of [
      "capture_counterparty_verdict",
      "data_drifts",
      "worklist.",
      "approval",
      "decisions",
    ]) {
      expect(text).not.toContain(raw);
    }
  });

  it("says nothing rather than printing a word it does not know", async () => {
    stub(
      day({
        queue: [
          row({
            title: "A task",
            // A reason from a newer server than this build.
            because: [{ kind: "customer_escalated" as never }],
          }),
        ],
        summary: { urgent: 0, due: 0, lower_priority: 0, total: 1 },
      }),
    );
    const { container } = renderWorklist();

    await screen.findByText("A task");
    expect(container.textContent).not.toContain("customer_escalated");
    expect(container.textContent).not.toContain("worklist.because");
  });

  it("offers a verb that goes somewhere, and none that does not", async () => {
    stub(
      day({
        queue: [
          row({
            id: "with-record",
            title: "Anna Weber",
            source: "customer_waiting",
            category: "customer_waiting",
            consequence: "buyer_waits",
            subject: {
              type: "person",
              id: "01a05500-0000-7000-8000-000000000001",
            },
            actions: ["open"],
          }),
          row({
            id: "no-destination",
            title: "Add someone from your mail",
            source: "approval",
            category: "decisions",
            consequence: "data_drifts",
            // `decide` is answered on this page, so it has nowhere to link.
            actions: ["decide"],
          }),
        ],
        summary: { urgent: 1, due: 0, lower_priority: 1, total: 2 },
      }),
    );
    renderWorklist();

    // The row that names a record gets its verb.
    expect(await screen.findByRole("link", { name: "Open" })).toBeTruthy();
  });

  it("lets a staged decision be answered on the page", async () => {
    stub(
      day({
        queue: [
          row({
            id: "ap-1",
            title: "Send the follow-up to Anna Weber",
            source: "approval",
            category: "decisions",
            level: 5,
            consequence: "work_blocked",
            actions: ["decide"],
          }),
        ],
        summary: { urgent: 0, due: 0, lower_priority: 1, total: 1 },
      }),
      {
        id: "ap-1",
        workspace_id: "w",
        kind: "send_email",
        status: "pending",
        proposed_by: "agent:runner",
        summary: "Send the follow-up to Anna Weber",
        proposed_change: { subject: "Follow-up", body: "Hi Anna" },
        confidence: 0.62,
        created_at: "2026-08-31T08:00:00Z",
      },
    );
    const { container } = renderWorklist();

    // The row arrives first; the decision it carries is a second read, so the
    // card appears after it.
    await screen.findByText(/Send the follow-up/);
    // A queue that can rank a decision and not answer it sends the reader to a
    // second screen to do what the row already described. The card is the same
    // one the record page draws, posting to the same endpoint.
    await waitFor(() => {
      expect(container.querySelector(".worklist-row-decision")).toBeTruthy();
    });
    expect(await screen.findByRole("button", { name: "Accept" })).toBeTruthy();
  });

  it("names the source it could not read rather than counting it", async () => {
    stub(
      day({
        sources_unavailable: [{ source: "capture_health", reason: "failed" }],
      }),
    );
    renderWorklist();

    expect(
      await screen.findByText(/mailbox connection needs attention/i),
    ).toBeTruthy();
  });

  it("writes the day's figures in the reader's own notation", async () => {
    stub(
      day({
        queue: [row()],
        summary: { urgent: 1234, due: 0, lower_priority: 0, total: 1 },
      }),
    );
    renderWorklist("de");

    expect(await screen.findByText(/1\.234/)).toBeTruthy();
  });

  it("offers no scope control when the reader has one scope", async () => {
    stub(day({ queue: [row()] }));
    renderWorklist();

    await screen.findByText("A task");
    // SegmentedControl draws a fieldset of pressed buttons, so the absence is
    // asserted on what it actually renders — a role it never uses would make
    // this pass over a control drawn unconditionally.
    expect(screen.queryByRole("group", { name: "Whose work" })).toBeNull();
    expect(screen.queryByRole("button", { name: "My team" })).toBeNull();
  });

  it("offers the scope control when the reader may ask for more", async () => {
    stub(day({ queue: [row()], scope_options: ["mine", "team", "all"] }));
    renderWorklist();

    await screen.findByText("A task");
    expect(screen.getAllByText("My team").length).toBeGreaterThan(0);
  });

  it("warns rather than claiming a clear day it could not read", async () => {
    stub(
      day({
        sources_unavailable: [{ source: "capture_health", reason: "failed" }],
      }),
    );
    renderWorklist();

    expect(
      await screen.findByText(
        "Nothing is waiting among the sources that answered.",
      ),
    ).toBeTruthy();
    expect(screen.queryByText("Nothing is waiting on you.")).toBeNull();
  });

  it("narrows the queue when the reader picks a kind of work", async () => {
    const user = userEvent.setup();
    stub(day({ queue: [row({ title: "A waiting customer" })] }));
    renderWorklist();

    await screen.findByText("A waiting customer");
    await user.click(screen.getByRole("button", { name: /Deals at risk/ }));

    await waitFor(() => {
      const calls = (globalThis.fetch as ReturnType<typeof vi.fn>).mock.calls;
      // The client passes a Request, so the URL is read off it rather than
      // stringified — String(request) yields "[object Request]".
      const urls = calls.map((call) => {
        const target = call[0];
        return target instanceof Request ? target.url : String(target);
      });
      expect(urls.some((url) => url.includes("filter=deals_at_risk"))).toBe(
        true,
      );
    });
  });
});

// An introduction ask is the one row whose `decide` leads somewhere.
//
// It has no inline card — the four answers are the colleague's own, given on
// the contact's Network tab — so without a routed verb the row names somebody
// waiting on them and offers nothing to press. An approval's `decide` still
// routes nowhere, because that decision IS this page.
describe("an introduction ask on the queue", () => {
  it("sends the colleague to the tab where the ask is answered", async () => {
    stub(
      day({
        queue: [
          row({
            id: "ask-1",
            source: "introduction_request",
            category: "decisions",
            level: 5,
            consequence: "work_blocked",
            detail: "Dana reopened the retrofit conversation.",
            actions: ["decide"],
            subject: {
              type: "person",
              id: "018f3a1b-0000-7000-8000-000000000010",
              label: "Dana Buyer",
            },
          }),
        ],
        summary: { urgent: 0, due: 0, lower_priority: 1, total: 1 },
      }),
    );
    renderWorklist();

    const verb = await screen.findByRole("link", { name: "Decide" });
    // The NETWORK tab, not the contact's default page: the ask is not
    // mentioned anywhere else, and landing a colleague on the overview leaves
    // them to go and find it.
    expect(verb.getAttribute("href")).toBe(
      "#/contacts/018f3a1b-0000-7000-8000-000000000010/network",
    );
  });

  it("leaves an approval's decide unrouted, because that decision is this page", async () => {
    stub(
      day({
        queue: [
          row({
            id: "appr-1",
            source: "approval",
            category: "decisions",
            level: 5,
            consequence: "work_blocked",
            title: "Send the retrofit quote",
            actions: ["decide"],
            subject: {
              type: "person",
              id: "018f3a1b-0000-7000-8000-000000000011",
              label: "Someone Else",
            },
          }),
        ],
        summary: { urgent: 0, due: 0, lower_priority: 1, total: 1 },
      }),
      { id: "appr-1", kind: "send_email", status: "pending" },
    );
    renderWorklist();

    // Wait for the queue itself to render before asserting an absence: a
    // findBy on a link that must NOT exist would pass against a page that had
    // not drawn anything yet.
    await screen.findByText("0 urgent · 0 due · 1 lower-priority");
    // A "Decide" LINK beside an inline decision card would ask the reader to
    // choose between answering here and going somewhere to answer.
    expect(screen.queryByRole("link", { name: "Decide" })).toBeNull();
  });
});

describe("the one thing to do next", () => {
  it("says what the top row is, rather than leaving a reader to work it out", async () => {
    stub(
      day({
        queue: [
          row({
            id: "focus-me",
            title: "Northstar renewal",
            band: "now",
            primary_action: "act",
            source: "deal_at_risk",
            category: "deals_at_risk",
            level: 3,
            // The record the verb goes to. Without it the row has no address
            // at all, and the card is correctly withheld — which is a
            // different case, tested below.
            subject: {
              type: "deal",
              id: "01a05500-0000-7000-8000-0000000000cc",
              label: "Northstar",
            },
          }),
        ],
        summary: { urgent: 1, due: 0, lower_priority: 0, total: 1 },
      }),
    );
    renderWorklist();

    // The card names the row and offers ONE verb. Both are drawn twice — once
    // on the card, once on the row it was lifted from — because the row stays
    // in the queue so the ranks and counts keep agreeing with the page.
    await screen.findByText("Do this next");
    expect(await screen.findAllByText("Northstar renewal")).toHaveLength(2);
  });

  it("promotes no card when the day's top row is review work", async () => {
    stub(
      day({
        queue: [
          row({
            id: "dupe",
            title: "Two Acme records",
            band: "review",
            primary_action: "merge",
            source: "dedupe_candidate",
            category: "decisions",
            level: 6,
          }),
        ],
        summary: { urgent: 0, due: 0, lower_priority: 1, total: 1 },
      }),
    );
    renderWorklist();

    // The row is there; the card is not. A page headed "do this next" over a
    // duplicate-merge suggestion tells a rep something false about their day.
    await screen.findByText("Two Acme records");
    expect(screen.queryByText("Do this next")).toBeNull();
  });

  it("promotes no card when the server named no verb for the top row", async () => {
    stub(
      day({
        queue: [
          row({ id: "verbless", title: "Something happened", band: "now" }),
        ],
        summary: { urgent: 1, due: 0, lower_priority: 0, total: 1 },
      }),
    );
    renderWorklist();

    await screen.findByText("Something happened");
    expect(screen.queryByText("Do this next")).toBeNull();
  });
  it("promotes no card when the row has nowhere for its verb to go", async () => {
    stub(
      day({
        // A task filed under nothing: no subject, so rowHref falls through
        // SOURCE_QUEUE and finds no address. The server named a verb, and the
        // page has nowhere to send it.
        queue: [
          row({
            id: "unfiled",
            title: "Something to do",
            band: "keep_momentum",
            primary_action: "complete",
          }),
        ],
        summary: { urgent: 0, due: 0, lower_priority: 0, total: 1 },
      }),
    );
    renderWorklist();

    await screen.findByText("Something to do");
    // A card with a headline and no way to act is worse than no card: it
    // occupies the place a reader looks for their next step.
    expect(screen.queryByText("Do this next")).toBeNull();
  });
});

describe("what the selected row is about", () => {
  it("draws no pane until the reader picks a row", async () => {
    stub(
      day({
        queue: [
          row({
            id: "one",
            title: "A task",
            subject: {
              type: "person",
              id: "01a05500-0000-7000-8000-0000000000aa",
              label: "Kirsten Vogel",
            },
          }),
        ],
        summary: { urgent: 0, due: 0, lower_priority: 0, total: 1 },
      }),
    );
    renderWorklist();

    // The full-width list a reader had before selection existed. An empty
    // column standing ready reads as a pane that failed to load.
    await screen.findByText("A task · Kirsten Vogel");
    expect(screen.queryByText("They last wrote")).toBeNull();
  });

  it("opens the record beside the queue, and closes it on a second press", async () => {
    stub(
      day({
        queue: [
          row({
            id: "one",
            title: "A task",
            subject: {
              type: "person",
              id: "01a05500-0000-7000-8000-0000000000aa",
              label: "Kirsten Vogel",
            },
          }),
        ],
        summary: { urgent: 0, due: 0, lower_priority: 0, total: 1 },
      }),
    );
    renderWorklist();

    const open = await screen.findByRole("button", {
      name: /^Show what/,
    });
    await userEvent.click(open);
    // The pane names the record and answers the question the row cannot: how
    // long the silence has run, in both directions.
    await screen.findByText("They last wrote");
    expect(screen.getByText("We last wrote")).toBeTruthy();

    await userEvent.click(screen.getByRole("button", { name: /^Show what/ }));
    await waitFor(() => {
      expect(screen.queryByText("They last wrote")).toBeNull();
    });
  });

  it("draws no pane for a row about a deal, whose figures are already on it", async () => {
    stub(
      day({
        queue: [
          row({
            id: "one",
            title: "Northstar renewal",
            source: "deal_at_risk",
            category: "deals_at_risk",
            subject: {
              type: "deal",
              id: "01a05500-0000-7000-8000-0000000000bb",
              label: "Northstar",
            },
          }),
        ],
        summary: { urgent: 0, due: 0, lower_priority: 0, total: 1 },
      }),
    );
    renderWorklist();

    const open = await screen.findByRole("button", {
      name: /^Show what/,
    });
    await userEvent.click(open);

    // Selected, and deliberately nothing beside it: a deal row carries its own
    // amount, close date and owner, so a pane would be a second spelling.
    await waitFor(() => {
      expect(screen.getByRole("button", { name: /^Show what/ })).toBeTruthy();
    });
    expect(screen.queryByText("They last wrote")).toBeNull();
  });

  it("closes the pane when the selected row leaves the queue", async () => {
    const withRow = day({
      queue: [
        row({
          id: "one",
          title: "A task",
          subject: {
            type: "person",
            id: "01a05500-0000-7000-8000-0000000000aa",
            label: "Kirsten Vogel",
          },
        }),
      ],
      summary: { urgent: 0, due: 0, lower_priority: 0, total: 1 },
    });
    // The same day with the row gone — a disposition, a filter, a refetch that
    // found it answered. The pane must not go on describing a record whose row
    // is no longer on the page.
    let current: Worklist = withRow;
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL) => {
        const url = String(input instanceof Request ? input.url : input);
        if (url.includes("/worklist")) {
          return jsonResponse(current);
        }
        return jsonResponse({ data: [] });
      }),
    );
    renderWorklist();

    await userEvent.click(
      await screen.findByRole("button", { name: /^Show what/ }),
    );
    await screen.findByText("They last wrote");

    current = day({
      queue: [],
      summary: { urgent: 0, due: 0, lower_priority: 0, total: 0 },
    });
    // Re-render through the filter, which refetches: the row is gone, and the
    // pane goes with it rather than outliving the row it describes.
    await userEvent.click(screen.getByRole("button", { name: /Decisions/ }));
    await waitFor(() => {
      expect(screen.queryByText("They last wrote")).toBeNull();
    });
  });
  it("draws no aside landmark for a row that has no pane", async () => {
    stub(
      day({
        queue: [
          row({
            id: "one",
            title: "Northstar renewal",
            source: "deal_at_risk",
            category: "deals_at_risk",
            subject: {
              type: "deal",
              id: "01a05500-0000-7000-8000-0000000000bb",
              label: "Northstar",
            },
          }),
        ],
        summary: { urgent: 0, due: 0, lower_priority: 0, total: 1 },
      }),
    );
    const { container } = renderWorklist();

    await userEvent.click(
      await screen.findByRole("button", { name: /^Show what/ }),
    );
    // A deal row has no pane. An empty <aside> would still be announced as a
    // landmark and still take its third of the grid, which reads as a pane
    // that failed rather than as one that was never meant to be there.
    await waitFor(() => {
      expect(container.querySelectorAll("aside")).toHaveLength(0);
    });
  });
});
