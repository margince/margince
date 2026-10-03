/** @vitest-environment happy-dom */
import { QueryClient } from "@tanstack/react-query";
import {
  cleanup,
  fireEvent,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { formatDateTime } from "../format/format";
import { viewerZone } from "../format/timezone";
import {
  line,
  receipt,
  renderMagic,
  stub,
  stubPending,
  stubRefusal,
} from "./magic.testkit";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

// The lane a reader looks at, found by its own name rather than by position:
// an assertion keyed on order would pass while the rows sat under the wrong
// words.
// The done lane is folded and so hidden until opened; present is what a case
// asks of it, so a lane is found whether or not it is open.
function lane(name: string): Promise<HTMLElement> {
  return screen.findByRole("list", { name, hidden: true });
}

function summary(): Promise<HTMLElement> {
  return screen.findByRole("list", { name: "Summary" });
}

describe("the receipt draws every lane it promises", () => {
  it("puts each lane's rows under that lane's own heading", async () => {
    stub(
      receipt({
        done: [line({ summary: { key: "magic.action.advance_stage" } })],
        needs_you: [
          line({
            id: "00000000-0000-7000-8000-000000000002",
            lane: "needs_you",
            summary: { key: "magic.action.approval_send_email" },
          }),
        ],
        could_not_complete: [
          line({
            id: "00000000-0000-7000-8000-000000000003",
            lane: "could_not_complete",
            summary: {
              key: "magic.action.automation_troubled",
              values: { name: "Nightly follow-up", outcome: "timed out" },
            },
          }),
        ],
        watching: [
          line({
            id: "00000000-0000-7000-8000-000000000004",
            lane: "watching",
            summary: {
              key: "magic.action.capture_reauth_required",
              values: { provider: "Gmail" },
            },
          }),
        ],
        totals: {
          done: 1,
          needs_you: 1,
          could_not_complete: 1,
          watching: 1,
        },
      }),
    );
    renderMagic();
    expect(
      within(await lane("Done for you")).getByText(
        "A deal moved to its next stage",
      ),
    ).toBeTruthy();
    expect(
      within(await lane("Waiting on you")).getByText(
        "A message is waiting for your word",
      ),
    ).toBeTruthy();
    expect(
      within(await lane("Could not be finished")).getByText(
        "Nightly follow-up is in trouble: timed out",
      ),
    ).toBeTruthy();
    expect(
      within(await lane("Needs restoring")).getByText(
        "Gmail needs to be connected again",
      ),
    ).toBeTruthy();
    // What waits and what broke lead; what already happened comes last.
    expect(
      screen.getAllByRole("heading", { level: 3 }).map((h) => h.textContent),
    ).toEqual([
      "Waiting on you",
      "Could not be finished",
      "Needs restoring",
      "Done for you",
    ]);
  });

  it("answers every lane in the summary and draws a group only for lines", async () => {
    stub(
      receipt({
        done: [line(), line({ id: "00000000-0000-7000-8000-000000000009" })],
        totals: { done: 2, needs_you: 0, could_not_complete: 0, watching: 0 },
      }),
    );
    renderMagic();
    expect(
      Array.from((await summary()).children).map((item) => item.textContent),
    ).toEqual([
      "Nothing waiting on you",
      "Nothing failed",
      "Every source healthy",
      "2 done for you",
    ]);
    // A clear lane is said once, in the summary, rather than as a heading
    // standing over nothing.
    expect(
      screen.queryByRole("heading", { name: "Waiting on you" }),
    ).toBeNull();
    expect(screen.queryByRole("list", { name: "Waiting on you" })).toBeNull();
    expect(screen.getByRole("heading", { name: "Done for you" })).toBeTruthy();
  });

  it("titles the section by its window and says when that window began", async () => {
    stub(receipt());
    renderMagic();
    expect(
      await screen.findByRole("heading", { name: "Since your last brief" }),
    ).toBeTruthy();
    expect(
      await screen.findByText(
        `What Margince did since ${formatDateTime("2026-09-12T08:00:00Z", "en", viewerZone())}.`,
      ),
    ).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Last 30 days" }));
    expect(
      screen.getByRole("heading", { name: "The last 30 days" }),
    ).toBeTruthy();
  });

  it("names a withheld source and claims nothing clear while one is named", async () => {
    stub(
      receipt({
        sources_unavailable: [{ source: "approval", reason: "withheld" }],
      }),
    );
    renderMagic();
    expect(
      await screen.findByText("Source not available to you: Proposals"),
    ).toBeTruthy();
    // The lane the refusal belongs to is EMPTY, and saying so would report a
    // clear queue over an answer nobody could read.
    expect(screen.queryByText("Nothing waiting on you")).toBeNull();
    expect(
      within(await summary()).getByText("Waiting on you: may be incomplete"),
    ).toBeTruthy();
  });

  it("says how much it left out, so five lines never imply five things happened", async () => {
    stub(
      receipt({
        done: [line()],
        totals: { done: 1, needs_you: 0, could_not_complete: 0, watching: 0 },
        not_shown: [{ reason: "out_of_scope", count: 12 }],
      }),
    );
    renderMagic();
    expect(
      await screen.findByText(
        "12 changes are not shown: outside your own records",
      ),
    ).toBeTruthy();
  });

  it("drops a sentence this build has no key for rather than printing the key", async () => {
    stub(
      receipt({
        done: [
          line({
            summary: { key: "magic.action.invented_by_a_newer_server" },
            entity: {
              type: "deal",
              id: "00000000-0000-7000-8000-0000000000aa",
              label: "Fleet retrofit",
            },
          }),
        ],
        totals: { done: 1, needs_you: 0, could_not_complete: 0, watching: 0 },
      }),
    );
    renderMagic();
    // The row still says what it was about, so the reader loses a sentence
    // rather than the line.
    expect(await screen.findByText("Fleet retrofit")).toBeTruthy();
    expect(screen.queryByText(/magic\.action\./)).toBeNull();
  });

  it("names a proposal's kind and subject in words, never its code", async () => {
    stub(
      receipt({
        needs_you: [
          line({
            lane: "needs_you",
            summary: {
              key: "magic.action.approval_pending",
              values: { kind: "capture_counterparty" },
            },
          }),
          line({
            lane: "needs_you",
            summary: {
              key: "magic.action.approval_capture_counterparty",
              values: {
                kind: "capture_counterparty",
                target: "Boris <boris@customer.example>",
              },
            },
          }),
        ],
        totals: { done: 0, needs_you: 2, could_not_complete: 0, watching: 0 },
      }),
    );
    renderMagic();
    const waiting = await lane("Waiting on you");
    expect(within(waiting).queryByText(/capture_counterparty/)).toBeNull();
    expect(
      within(waiting).getByText(/Boris <boris@customer\.example> wrote to you/),
    ).toBeTruthy();
  });

  it("points a waiting decision at that decision, where it is decided", async () => {
    stub(
      receipt({
        needs_you: [
          line({
            id: "0198a0de-0000-7000-8000-00000000d0c1",
            lane: "needs_you",
            summary: {
              key: "magic.action.approval_advance_deal",
              values: { target: "Fleet retrofit" },
            },
            consequence: "magic.consequence.awaits_your_decision",
            // An older server still sends a refusal here; the line must not
            // repeat it as "cannot be undone" about a change nobody made.
            undo: { undoable: false, reason: "no_completed_change" },
          }),
        ],
        totals: { done: 0, needs_you: 1, could_not_complete: 0, watching: 0 },
      }),
    );
    renderMagic();
    const waiting = await lane("Waiting on you");
    expect(
      within(waiting).getByText("Nothing happens until you decide."),
    ).toBeTruthy();
    const decide = within(waiting).getByRole("link", { name: "Decide" });
    expect(decide.getAttribute("href")).toBe(
      "#/home?approval=0198a0de-0000-7000-8000-00000000d0c1",
    );
    expect(within(waiting).queryAllByRole("button")).toEqual([]);
    expect(within(waiting).queryByText("Undo")).toBeNull();
    expect(
      within(waiting).queryByText(
        "Nothing changed, so there is nothing to put back.",
      ),
    ).toBeNull();
  });

  it("says why a change cannot be taken back instead of greying a control", async () => {
    stub(
      receipt({
        done: [line({ undo: { undoable: false, reason: "already_undone" } })],
        totals: { done: 1, needs_you: 0, could_not_complete: 0, watching: 0 },
      }),
    );
    renderMagic();
    expect(
      await screen.findByText("This change was already undone."),
    ).toBeTruthy();
  });

  it("says what a job did, why, who it was, and how many records it touched", async () => {
    stub(
      receipt({
        done: [
          line({
            summary: { key: "magic.action.mail_filed" },
            reason: { key: "magic.why.mail_filed" },
            actor: {
              type: "system",
              id: "link-reconcile",
              label: { key: "magic.by.mail_filing" },
            },
            entity: {
              type: "contact",
              id: "00000000-0000-7000-8000-0000000000cc",
              label: "Anna Keller",
            },
            count: 1200,
          }),
        ],
        totals: { done: 1, needs_you: 0, could_not_complete: 0, watching: 0 },
      }),
    );
    renderMagic();
    expect(
      await screen.findByText("Filed captured email under this contact"),
    ).toBeTruthy();
    expect(
      screen.getByText("The sender’s address belongs to this contact."),
    ).toBeTruthy();
    const filed = await lane("Done for you");
    expect(within(filed).getByText("Mail filing")).toBeTruthy();
    expect(within(filed).getByText("Anna Keller and 1,199 more")).toBeTruthy();
  });

  it("says a count is a floor when the read behind it was cut short", async () => {
    stub(
      receipt({
        done: [
          line({
            summary: { key: "magic.action.mail_filed" },
            reason: { key: "magic.why.mail_filed" },
            entity: {
              type: "contact",
              id: "00000000-0000-7000-8000-0000000000cc",
              label: "Anna Keller",
            },
            count: 5000,
            count_is_floor: true,
          }),
        ],
        totals: { done: 1, needs_you: 0, could_not_complete: 0, watching: 0 },
      }),
    );
    renderMagic();
    expect(
      await screen.findByText("Anna Keller and at least 4,999 more"),
    ).toBeTruthy();
  });

  it("names the one record a cut read saw without claiming it was the only one", async () => {
    stub(
      receipt({
        done: [
          line({
            summary: { key: "magic.action.mail_filed" },
            reason: { key: "magic.why.mail_filed" },
            entity: {
              type: "contact",
              id: "00000000-0000-7000-8000-0000000000cc",
              label: "Anna Keller",
            },
            count_is_floor: true,
          }),
        ],
        totals: { done: 1, needs_you: 0, could_not_complete: 0, watching: 0 },
      }),
    );
    renderMagic();
    expect(
      await screen.findByText("Anna Keller, and possibly others"),
    ).toBeTruthy();
  });

  it("counts a retention action without naming any record it touched", async () => {
    stub(
      receipt({
        done: [
          line({
            summary: { key: "magic.action.retention_lead_anonymize" },
            reason: { key: "magic.why.retention", values: { days: "365" } },
            actor: {
              type: "system",
              id: "system",
              label: { key: "magic.by.retention" },
            },
            entity: undefined,
            count: 1200,
            undo: undefined,
          }),
        ],
        totals: { done: 1, needs_you: 0, could_not_complete: 0, watching: 0 },
      }),
    );
    renderMagic();
    expect(
      await screen.findByText(
        "Anonymized unconverted leads past their retention period",
      ),
    ).toBeTruthy();
    expect(screen.getByText("Retention rule: after 365 days.")).toBeTruthy();
    expect(screen.getByText("1,200 records")).toBeTruthy();
  });

  it("puts an undoable change back with one press, sending the record's version", async () => {
    stub(
      receipt({
        done: [
          line({
            entity: {
              type: "deal",
              id: "00000000-0000-7000-8000-0000000000aa",
              label: "Fleet retrofit",
            },
            undo: {
              undoable: true,
              audit_id: "00000000-0000-7000-8000-0000000000bb",
              version: 7,
            },
          }),
        ],
        totals: { done: 1, needs_you: 0, could_not_complete: 0, watching: 0 },
      }),
    );
    renderMagic();
    await userEvent.click(await screen.findByRole("button", { name: "Undo" }));
    await waitFor(() => {
      const restore = vi
        .mocked(fetch)
        .mock.calls.map(([input]) => input as Request)
        .find((request) =>
          request.url.endsWith(
            "/records/deal/00000000-0000-7000-8000-0000000000aa/history/00000000-0000-7000-8000-0000000000bb/restore",
          ),
        );
      expect(restore?.method).toBe("POST");
      expect(restore?.headers.get("If-Match")).toContain("7");
    });
  });

  it("says it is reading while the read is in flight", () => {
    stubPending();
    renderMagic();
    expect(
      screen.getAllByText("Reading what the machinery did").length,
    ).toBeGreaterThan(0);
  });

  it("reports a refusal as a refusal, never as a quiet morning", async () => {
    stubRefusal();
    renderMagic();
    expect(await screen.findByText("This section did not load.")).toBeTruthy();
    expect(screen.queryByRole("list", { name: "Summary" })).toBeNull();
    expect(screen.queryByText("Nothing done for you")).toBeNull();
  });

  it("offers no undo from a receipt whose refresh failed", async () => {
    stub(
      receipt({
        done: [
          line({
            entity: {
              type: "deal",
              id: "00000000-0000-7000-8000-0000000000aa",
              label: "Fleet retrofit",
            },
            undo: {
              undoable: true,
              audit_id: "00000000-0000-7000-8000-0000000000bb",
              version: 7,
            },
          }),
        ],
        totals: { done: 1, needs_you: 0, could_not_complete: 0, watching: 0 },
      }),
    );
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    renderMagic("en", client);
    expect(await screen.findByRole("button", { name: "Undo" })).toBeTruthy();

    stubRefusal();
    await client.refetchQueries();
    // The cached answer is still in the query, and an undo drawn from it
    // would sit under a section that says it did not load.
    expect(await screen.findByText("This section did not load.")).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Undo" })).toBeNull();
    // Nor does it date a window it could not read.
    expect(screen.queryByText(/What Margince did since/)).toBeNull();
    expect(screen.queryByRole("list", { name: "Done for you" })).toBeNull();
  });

  // A watching line's occurred_at is when the condition was seen, so a source
  // that broke weeks ago would otherwise read as having broken on page load.
  it("dates a failing source from when it started failing", async () => {
    stub(
      receipt({
        watching: [
          line({
            lane: "watching",
            occurred_at: "2026-09-13T07:30:00Z",
            summary: {
              key: "magic.action.capture_sync_failing",
              values: {
                provider: "google",
                failing_since: "2026-08-20T06:00:00Z",
              },
            },
          }),
        ],
      }),
    );
    renderMagic();
    const when = await screen.findByText(/Failing since/);
    // The outage's own date, not the read's: a client showing occurred_at here
    // would date every watched source to this page load.
    expect(when.textContent).toContain(
      formatDateTime("2026-08-20T06:00:00Z", "en", viewerZone()),
    );
  });

  // A source that is off rather than failing has no beginning to report, and
  // inventing one from the read would be the same lie in the other direction.
  it("reports no beginning for a condition that never started failing", async () => {
    stub(
      receipt({
        watching: [
          line({
            lane: "watching",
            summary: {
              key: "magic.action.capture_reauth_required",
              values: { provider: "google" },
            },
          }),
        ],
      }),
    );
    renderMagic();
    within(await lane("Needs restoring")).getByText(
      "google needs to be connected again",
    );
    expect(screen.queryByText(/Failing since/)).toBeNull();
  });

  // A RESPONSE THIS CLIENT DID NOT EXPECT MUST NOT TAKE THE PAGE DOWN.
  //
  // The panel sits on Home beside every other section, so a field read off a
  // shape the server did not send throws inside the shell's render and costs a
  // reader the whole screen, not one card. Version skew is the ordinary way
  // that happens.
  it("draws without a count rather than throwing on a receipt it cannot read", async () => {
    stub({ data: [] } as unknown as ReturnType<typeof receipt>);
    renderMagic();
    // Every lane says it cannot be counted: absent is version skew, and only a
    // list the server sent can say there is nothing in it.
    expect(
      Array.from((await summary()).children).map((item) => item.textContent),
    ).toEqual([
      "Waiting on you: may be incomplete",
      "Could not be finished: may be incomplete",
      "Needs restoring: may be incomplete",
      "Done for you: may be incomplete",
    ]);
  });
});

describe("the reader chooses how far back the page looks", () => {
  // The default window is the server's own: since the last brief. A reader
  // back from a weekend, or looking at what an import set off, needs more,
  // and without a choice the page reported "nothing" about work it had done.
  it("leaves since to the server by default and asks for 7 days when chosen", async () => {
    stub(receipt());
    renderMagic();
    const fetched = vi.mocked(fetch);
    const magicReads = () =>
      fetched.mock.calls
        .map(([input]) => String(input instanceof Request ? input.url : input))
        .filter((url) => url.split("?")[0].endsWith("/magic"));
    await waitFor(() => expect(magicReads().length).toBe(1));
    expect(magicReads()[0]).not.toContain("since=");

    fireEvent.click(screen.getByRole("button", { name: "Last 7 days" }));
    await waitFor(() => expect(magicReads().length).toBe(2));
    const since = new URL(magicReads()[1], "http://x").searchParams.get(
      "since",
    );
    expect(since).toBeTruthy();
    const days = (Date.now() - Date.parse(since ?? "")) / 86_400_000;
    expect(Math.round(days)).toBe(7);
  });
});
