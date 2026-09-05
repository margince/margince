// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment jsdom */
import { cleanup, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { en } from "../i18n/en";
import {
  day,
  renderWorklist,
  row,
  stub,
  type WorklistItem,
} from "./worklist.testkit";

// The ranked queue, and the ways it can mislead the person reading it.
//
// Every case here is one promise the page makes: that the order is readable,
// that a figure describes the rows beneath it, that nothing is drawn to report
// a zero, and that the database's own words never reach the screen.
//
// What the pane opens (and refuses to open) lives in worklist.pane.test.tsx.

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

  // The reasons past the third go behind a tap — NOT away.
  //
  // A cap that dropped them was tried and abandoned: the reasons that matter
  // most are appended LAST, because they are applied late. `pinned` drives the
  // row to the top and is appended by applyPins after everything else;
  // `expected_revenue` is absorbed onto a waiting row last and is a ranking
  // comparator. A head-of-list cut takes exactly the facts that decided where
  // the row sits.
  it("folds the reasons past the third rather than dropping them", async () => {
    stub(
      day({
        queue: [
          row({
            title: "A buyer is waiting on a funded deal",
            source: "customer_waiting",
            because: [
              { kind: "buyer_wrote_last" },
              { kind: "stale" },
              { kind: "no_reply_history" },
              { kind: "asks_nothing" },
              // Absorbed from the deal this wait suppressed, appended last.
              { kind: "expected_revenue" },
            ],
          }),
        ],
        summary: { urgent: 0, due: 0, lower_priority: 1, total: 1 },
      }),
    );
    const { container } = renderWorklist();

    await screen.findByText("A buyer is waiting on a funded deal");
    const fold = container.querySelector("details.worklist-row-because-fold");
    expect(fold).not.toBeNull();
    // Read the two halves SEPARATELY. Asserting over the whole page cannot
    // tell "in the summary" from "in the fold", so it passes just as well on a
    // row that put the strongest reasons behind the tap — which is the exact
    // inversion this component exists to prevent.
    const summary = fold?.querySelector("summary")?.textContent ?? "";
    const hidden = fold?.querySelector("p")?.textContent ?? "";

    // THE SERVER'S ORDER, kept. `because` arrives "in the order they were
    // weighed", so the head is the strongest three and the fold falls in the
    // right place by construction. Nothing held that until now: reversing the
    // list before the slice passed every test in this file, and a reversed
    // fold buries the weak reasons and promotes the ones a reader already has.
    // Positions, checked for PRESENCE first. indexOf answers -1 for a phrase
    // that is not there, and -1 is less than every real index — so an ordering
    // assertion alone passes when the reason it names has vanished entirely.
    const at = (kind: "buyer_wrote_last" | "stale" | "no_reply_history") => {
      const found = summary.indexOf(en[`worklist.because.${kind}`]);
      expect(found, `${kind} is missing from the summary`).toBeGreaterThan(-1);
      return found;
    };
    expect(at("buyer_wrote_last")).toBeLessThan(at("stale"));
    expect(at("stale")).toBeLessThan(at("no_reply_history"));

    // The rest are PRESENT — folded, never discarded — and specifically NOT in
    // the summary. The one that decided the rank is in here, which is the whole
    // reason a cap was wrong.
    for (const kind of ["asks_nothing", "expected_revenue"] as const) {
      expect(hidden).toContain(en[`worklist.because.${kind}`]);
      expect(summary).not.toContain(en[`worklist.because.${kind}`]);
    }

    // The count names how many are ACTUALLY hidden, so a reader can decide
    // whether to spend the tap. Asserted as the whole rendered phrase against
    // the summary alone: `toContain("2")` matched any digit 2 anywhere on the
    // page, so a summary promising "+22 more reasons" over two hidden facts
    // passed it.
    expect(summary).toContain(
      en["worklist.because.more_other"].replace("{count}", "2"),
    );
  });

  // Three reasons still fit on one line at 390px, so folding them would cost a
  // tap and save nothing. Measured 2026-09-05: two and three are both 19px; the
  // fourth wraps to 37px.
  it("draws no fold when every reason fits", async () => {
    stub(
      day({
        queue: [
          row({
            title: "A task",
            because: [
              { kind: "overdue" },
              { kind: "due_today" },
              { kind: "unassigned" },
            ],
          }),
        ],
        summary: { urgent: 0, due: 0, lower_priority: 1, total: 1 },
      }),
    );
    const { container } = renderWorklist();

    await screen.findByText("A task");
    expect(container.textContent).toContain(en["worklist.because.unassigned"]);
    expect(
      container.querySelector("details.worklist-row-because-fold"),
    ).toBeNull();
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

// The lead lane, on the screen.
//
// The lane, its dedupe and its bounds are proved in Go. What none of that can
// see is the half of the same invariant that lives here: a source, a category
// and three reason kinds each have to be REGISTERED in worklist.copy.ts before
// a word of them reaches a reader. An unregistered one does not error — it
// renders a fallback, or silently drops the figure it was carrying — so the
// backend can be entirely correct while the row says nothing.
describe("a lead still owed its first reply", () => {
  function leadRow(over: Partial<WorklistItem> = {}): WorklistItem {
    return row({
      id: "lead-row",
      source: "lead_response",
      category: "leads",
      level: 1,
      title: "Weber GmbH",
      subject: { type: "lead", id: "11111111-2222-4333-8444-555555555555" },
      consequence: "buyer_waits",
      ...over,
    });
  }

  it("says why it is here, in the reader's own words", async () => {
    stub(
      day({
        queue: [
          leadRow({
            because: [
              { kind: "response_overdue" },
              { kind: "waiting_days", value: { kind: "days", days: 2 } },
            ],
          }),
        ],
      }),
    );
    renderWorklist();

    await screen.findByText("Weber GmbH");
    // The category badge, and the reason — both from the catalog, so an
    // unregistered kind renders its raw key here and fails visibly.
    expect(screen.getByText(en["worklist.category.leads"])).toBeTruthy();
    expect(
      screen.getByText(new RegExp(en["worklist.because.response_overdue"])),
    ).toBeTruthy();
  });

  // A lead with no name of its own still gets a sentence rather than a blank.
  it("names the kind of thing it is when the lead has no name", async () => {
    stub(day({ queue: [leadRow({ title: undefined })] }));
    renderWorklist();

    expect(
      await screen.findByText(en["worklist.untitled.lead_response"]),
    ).toBeTruthy();
  });

  // The row's way out is the LEAD's own record, through the shared entity
  // registry — never a path spelled a second time in this file.
  it("opens the lead record it is about", async () => {
    stub(day({ queue: [leadRow()] }));
    renderWorklist();

    const link = await screen.findByRole("link", { name: "Weber GmbH" });
    expect(link.getAttribute("href")).toBe(
      "#/leads/11111111-2222-4333-8444-555555555555",
    );
  });

  // And the queue can be narrowed to them, which is the strip card's
  // destination as well as a pill of its own.
  it("can be narrowed to on its own", async () => {
    const user = userEvent.setup();
    stub(day({ queue: [leadRow()] }));
    renderWorklist();

    await screen.findByText("Weber GmbH");
    await user.click(
      screen.getByRole("button", { name: en["worklist.filter.leads"] }),
    );

    await waitFor(() => {
      const calls = (globalThis.fetch as ReturnType<typeof vi.fn>).mock.calls;
      const urls = calls.map((call) => {
        const target = call[0];
        return target instanceof Request ? target.url : String(target);
      });
      expect(urls.some((url) => url.includes("filter=leads"))).toBe(true);
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
    await screen.findByText(/0 urgent/);
    // A "Decide" LINK beside an inline decision card would ask the reader to
    // choose between answering here and going somewhere to answer.
    expect(screen.queryByRole("link", { name: "Decide" })).toBeNull();
  });
});

describe("one obligation is drawn once", () => {
  // The top row IS the one thing to do next, and the page no longer says so a
  // second time. A focus card above the queue drew that row whole and a Next-up
  // list drew the three after it as titles, so a reader counting their morning
  // off this screen counted the first four rows twice.
  //
  // What replaced it: the first row is selected on arrival, which puts it in
  // the context pane without drawing it again.
  it("draws the top row once, not once on a card and once in the queue", async () => {
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

    expect(await screen.findAllByText(/Northstar renewal/)).toHaveLength(1);
  });

  // Review work is judgement the queue collects, not a rep's next action. It
  // still belongs on the page — what it must not do is claim the top of it.
  it("keeps review work on the page without promoting it", async () => {
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

    expect(await screen.findAllByText(/Two Acme records/)).toHaveLength(1);
  });

  // A row the server named no verb for is still real work and still drawn. It
  // simply has nothing to press, which the row says by drawing no control.
  it("draws a row the server named no verb for", async () => {
    stub(
      day({
        queue: [
          row({ id: "verbless", title: "Something happened", band: "now" }),
        ],
        summary: { urgent: 1, due: 0, lower_priority: 0, total: 1 },
      }),
    );
    renderWorklist();

    expect(await screen.findAllByText(/Something happened/)).toHaveLength(1);
  });
});

describe("the address opens a queue", () => {
  function requestedUrls(): string[] {
    const mock = globalThis.fetch as unknown as {
      mock: { calls: readonly (readonly unknown[])[] };
    };
    return mock.mock.calls
      .map(([input]) =>
        String(input instanceof Request ? input.url : (input as string)),
      )
      .filter((url) => url.includes("/worklist"));
  }

  it("asks for a named colleague's day when the segment is a user id", async () => {
    stub(day({ scope_options: ["mine", "team"] }));
    renderWorklist("en", "11111111-1111-4111-8111-111111111111");

    await waitFor(() => expect(requestedUrls().length).toBeGreaterThan(0));
    expect(
      requestedUrls().some((url) =>
        url.includes("owner=11111111-1111-4111-8111-111111111111"),
      ),
    ).toBe(true);
  });

  // The scope word is not an owner. Passing it as one would ask the server for
  // a person whose id is the string "unassigned", which is a 403 or a 404 where
  // the reader asked a perfectly ordinary question.
  it("asks for the unowned pile when the segment is the scope word", async () => {
    stub(day({ scope: "unassigned", scope_options: ["mine", "team"] }));
    renderWorklist("en", "unassigned");

    await waitFor(() => expect(requestedUrls().length).toBeGreaterThan(0));
    expect(
      requestedUrls().some((url) => url.includes("scope=unassigned")),
    ).toBe(true);
    expect(requestedUrls().some((url) => url.includes("owner="))).toBe(false);
  });

  // No segment is the reader's own day, which is what every seat sees.
  it("asks for the reader's own day when the address names nothing", async () => {
    stub(day());
    renderWorklist();

    await waitFor(() => expect(requestedUrls().length).toBeGreaterThan(0));
    expect(requestedUrls().some((url) => url.includes("owner="))).toBe(false);
    expect(requestedUrls().some((url) => url.includes("scope=mine"))).toBe(
      true,
    );
  });
});

// The label moves with the route.
//
// The row's own comment refused to say "Draft the reply" while the click only
// navigated: "a link labelled 'Draft the reply' would promise something the
// click does not do… the label moves back when it lands". This is the assertion
// that the two halves stay together — mutate either and one of these fails.
describe("the draft_reply verb says what the click does", () => {
  function replyRow(subjectType: string, id: string): WorklistItem {
    return row({
      id: `m-${id}`,
      source: "waiting_customer",
      category: "customer_waiting",
      title: "Aster Handel",
      subject: { type: subjectType, id },
      move: { action: "draft_reply", activity_id: "a-1" },
    } as unknown as Partial<WorklistItem>);
  }

  it("names the ACT where the address opens the composer", async () => {
    stub(day({ queue: [replyRow("person", "p-1")] }));
    renderWorklist();

    const link = await screen.findByRole("link", {
      name: en["worklist.verb.draft_reply_now"],
    });
    expect(link.getAttribute("href")).toContain("compose=reply");
  });

  it("names where it GOES where there is no composer to open", async () => {
    stub(day({ queue: [replyRow("deal", "d-1")] }));
    renderWorklist();

    const link = await screen.findByRole("link", {
      name: en["worklist.verb.draft_reply"],
    });
    expect(link.getAttribute("href")).not.toContain("compose=");
    expect(
      screen.queryByRole("link", {
        name: en["worklist.verb.draft_reply_now"],
      }),
    ).toBeNull();
  });
});

// One obligation, drawn once.
//
// The page used to render its first item up to three times — as a focus card,
// as the head of a "next up" strip, and again as a row in the queue. Three
// drawings of one thing is not emphasis; it is a reader counting their morning
// wrong, and a rep who answers the focus card then meets the same customer
// twice on the way down.
//
// Counted over every `.worklist-row-title` ON THE PAGE, not within its list
// items. The duplication this guards against renders OUTSIDE the queue's <ol>
// — a card above it drawing the top row again — so a walk over <li> elements
// filters out the one shape of the defect the test is named for. It passed for
// exactly that reason while the screen showed the duplication.
//
// Titles rather than identities, and the fixture is built so that is sound:
// three rows, three distinct titles. Two rows may honestly share a title in
// production — two tasks called "Follow up" are two obligations — so this
// fixture must keep them distinct for the count to mean what it says, which is
// what `seen.size` being asserted at 3 holds.
describe("every obligation is drawn once", () => {
  it("renders no row twice, including the one at the top", async () => {
    stub(
      day({
        queue: [
          row({
            id: "a",
            source: "customer_waiting",
            title: "Kirsten replied",
          }),
          row({ id: "b", source: "task", title: "Send the quote" }),
          row({ id: "c", source: "meeting", title: "Quarterly review" }),
        ],
        summary: { urgent: 1, due: 1, lower_priority: 1, total: 3 },
      }),
    );

    const { container } = renderWorklist();
    await screen.findByText("Kirsten replied");

    // Over the WHOLE page, not over its list items. The focus card this test
    // exists to keep away drew its copy OUTSIDE any <li>, so walking list items
    // filtered the duplicate out and the test passed while the screen showed
    // exactly the duplication it names.
    const drawn = [...container.querySelectorAll(".worklist-row-title")].map(
      (title) => title.textContent ?? "",
    );
    const seen = new Map<string, number>();
    for (const title of drawn.filter(Boolean)) {
      seen.set(title, (seen.get(title) ?? 0) + 1);
    }

    const twice = [...seen.entries()].filter(([, times]) => times > 1);
    expect(twice).toEqual([]);
    expect(seen.size).toBe(3);
  });
});

// Done, and the way back from it.
//
