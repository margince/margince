/** @vitest-environment happy-dom */
import { cleanup, screen, waitFor, within } from "@testing-library/react";
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

// The lane a reader looks at, found by its own heading rather than by position:
// the panel draws four sections and an assertion keyed on order would pass
// while the rows sat under the wrong words.
function lane(heading: string): HTMLElement {
  const section = screen
    .getByRole("heading", { name: heading })
    .closest("section");
  if (!section) {
    throw new Error(`the "${heading}" heading sits in no section`);
  }
  return section;
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
      await within(lane("Done for you")).findByText(
        "A deal moved to its next stage",
      ),
    ).toBeTruthy();
    expect(
      within(lane("Waiting on you")).getByText(
        "A message is waiting for your word",
      ),
    ).toBeTruthy();
    expect(
      within(lane("Could not be finished")).getByText(
        "Nightly follow-up is in trouble: timed out",
      ),
    ).toBeTruthy();
    expect(
      within(lane("Needs restoring")).getByText(
        "Gmail needs to be connected again",
      ),
    ).toBeTruthy();
  });

  it("says a lane is clear rather than drawing a heading over a gap", async () => {
    stub(
      receipt({
        done: [line()],
        totals: { done: 1, needs_you: 0, could_not_complete: 0, watching: 0 },
      }),
    );
    renderMagic();
    expect(
      await within(lane("Waiting on you")).findByText(
        "No decision is waiting on you.",
      ),
    ).toBeTruthy();
    expect(
      within(lane("Needs restoring")).getByText(
        "Every source and rule is healthy.",
      ),
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
      await screen.findByText(
        "Source not available to you: Proposals",
      ),
    ).toBeTruthy();
    // The lane the refusal belongs to is EMPTY, and saying so would report a
    // clear queue over an answer nobody could read.
    expect(screen.queryByText("No decision is waiting on you.")).toBeNull();
    expect(
      within(lane("Waiting on you")).getByText(/may be incomplete/i),
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

  it("gives a waiting decision no verb, because decisions are decided elsewhere", async () => {
    stub(
      receipt({
        needs_you: [
          line({
            lane: "needs_you",
            summary: {
              key: "magic.action.approval_advance_deal",
              values: { target: "Fleet retrofit" },
            },
            consequence: "magic.consequence.awaits_your_decision",
            undo: { undoable: false, reason: "no_completed_change" },
          }),
        ],
        totals: { done: 0, needs_you: 1, could_not_complete: 0, watching: 0 },
      }),
    );
    renderMagic();
    const waiting = lane("Waiting on you");
    expect(
      await within(waiting).findByText("Nothing happens until you decide."),
    ).toBeTruthy();
    expect(within(waiting).queryAllByRole("button")).toEqual([]);
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

  it("points an undoable change at the record whose history owns the way back", async () => {
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
            },
          }),
        ],
        totals: { done: 1, needs_you: 0, could_not_complete: 0, watching: 0 },
      }),
    );
    renderMagic();
    const wayBack = await screen.findByRole("link", {
      name: "Can be put back from the record’s history",
    });
    expect(wayBack.getAttribute("href")).toBe(
      "#/deals/00000000-0000-7000-8000-0000000000aa",
    );
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
    await waitFor(() => {
      expect(screen.getAllByText("This section did not load.")).toHaveLength(4);
    });
    expect(
      screen.queryByText("Nothing was done on your behalf in this window."),
    ).toBeNull();
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
    await screen.findByText("Needs restoring");
    expect(screen.queryByText(/Failing since/)).toBeNull();
  });
});
