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
