// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment jsdom */
import { cleanup, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { day, renderWorklist, stub } from "./worklist.testkit";

// Whose hidden work the guardrail is about.
//
// The panel says what the queue is NOT showing, which makes it the one surface
// where attributing a figure to the wrong person does the most damage: a reader
// checking whether their day is honest is told about somebody else's.
//
// The endpoint takes no owner and no scope. It derives its subject from the
// authenticated principal, so wherever the queue beside it is about somebody
// else, the panel is still answering about the reader — and there are TWO ways
// to leave your own day, reached by different controls.
//
// Apart from worklist.test.tsx because that file is over the 1000-line ceiling
// (frontend/AGENTS.md) and these cases are one question of their own.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("the hidden-backlog panel is about the reader", () => {
  // Every request for the hidden-backlog figures, told apart from the day's own
  // read: `/worklist/hidden` starts with `/worklist`, so a filter on the prefix
  // alone counts the queue itself and can never report zero.
  function hiddenRequests(): string[] {
    const mock = globalThis.fetch as unknown as {
      mock: { calls: readonly (readonly unknown[])[] };
    };
    return mock.mock.calls
      .map(([input]) =>
        String(input instanceof Request ? input.url : (input as string)),
      )
      .filter((url) => url.includes("/worklist/hidden"));
  }

  // The hidden-backlog panel answers about the READER, always.
  //
  // The endpoint takes no owner and derives its subject from the authenticated
  // principal, so under a drill-down the panel stood on a page headed with a
  // colleague's name and reported the MANAGER's own hidden work: "412 hidden
  // from you" read as Lena's backlog. On the one surface whose whole job is to
  // say what a queue is not showing, that is the worst place in the product to
  // attribute a figure to the wrong person.
  it("AC-WORKLIST-MGR-02: draws no hidden-backlog panel on a colleague's queue", async () => {
    stub(day({ scope_options: ["mine", "team"] }));
    renderWorklist("en", "11111111-1111-4111-8111-111111111111");

    // Waited on the panel's own NEIGHBOUR rather than on a stopwatch. The
    // coach control renders on this same drill-down and sits immediately before
    // the panel in the tree, so seeing it proves React reached the point where
    // the panel would have rendered and asked. A pause of N milliseconds proves
    // the same thing only on a machine fast enough, which is a test that passes
    // for a reason unrelated to what it checks.
    //
    // The wait is load-bearing: asserting straight after the day's read reports
    // silence from a component that has not run yet, and this case passed with
    // the guard removed before the anchor was added.
    await screen.findByRole("button", { name: "Leave a note" });
    // And a line drawn after the panel, so the wait covers the panel's own
    // position rather than only its neighbour's.
    await screen.findByText("Nothing is waiting on you.");

    // The panel is absent, and — the half that actually holds — it never ASKS.
    // The endpoint answers about the authenticated principal, so a request made
    // from a colleague's page is already the wrong question; the drawn panel is
    // only where the wrong answer would have shown up.
    expect(screen.queryByText("What the queue is not showing")).toBeNull();
    expect(hiddenRequests()).toEqual([]);
  });

  // The OTHER way of leaving your own day, and the one the first guard missed.
  //
  // `owner` is the drill-down into a named colleague. The scope picker beside
  // it — and the team board's own "show me the unowned pile" — moves the queue
  // to somebody else's work while leaving the owner empty, so a guard on the
  // drill-down alone left the reader's own figure standing under the unassigned
  // and team queues. Two controls, one wrong answer.
  it("draws no hidden-backlog panel on a queue that is not the reader's", async () => {
    stub(
      day({
        scope: "unassigned",
        scope_options: ["mine", "unassigned", "team"],
      }),
    );
    renderWorklist("en", "unassigned");

    // Anchored on a line drawn AFTER the panel in the same tree, so seeing it
    // proves React reached and passed the point where the panel would have
    // rendered and asked.
    //
    // `waitFor` on the absence itself would prove nothing: "this text is not
    // here" is already true before the day has landed, so the wait returns on
    // its first tick and the case passes against a page that has not rendered.
    // That is the defect this anchor replaces, and it is the same one the
    // sibling case above hit.
    await screen.findByText("Nothing is waiting on you.");

    expect(screen.queryByText("What the queue is not showing")).toBeNull();
    expect(hiddenRequests()).toEqual([]);
  });

  // And it IS drawn on the reader's own day, where the figure is about them.
  // Without this the assertion above passes on a panel deleted outright.
  it("draws the hidden-backlog panel on the reader's own day", async () => {
    stub(day({ scope_options: ["mine", "team"] }));
    renderWorklist("en");

    expect(
      await screen.findByText("What the queue is not showing"),
    ).toBeTruthy();
    // It asked, which is what makes the silence above mean something.
    await waitFor(() => expect(hiddenRequests().length).toBeGreaterThan(0));
  });
});
