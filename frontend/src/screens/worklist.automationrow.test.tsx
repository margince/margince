// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment jsdom */
import { cleanup, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { day, renderWorklist, row, stub } from "./worklist.testkit";

// A failed rule says where to go and look at it.
//
// The row named a broken automation — "Notify sales on a new lead", failed —
// and carried no address at all. Not the run, not the rule, not the screen
// either lives on. A reader was told something was wrong and left to go and
// find it, which on a queue whose whole promise is "here is your work" is the
// row doing the opposite of its job.
//
// It is a DESTINATION and not a verb. Fixing a rule is the automations page's
// job; the queue performs nothing. Retry is deliberately not offered: a re-run
// would have to replay the event that fired the rule, and `workflow_run` keeps
// only a pointer to a bus event the bus drops after about three days — so the
// button would silently do nothing on day four, which is worse than no button.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function aFailedRule(over = {}) {
  return row({
    id: "01a05500-0000-7000-8000-0000000000a1",
    source: "automation_run",
    category: "system",
    title: "Notify sales on a new lead",
    band: "keep_momentum",
    destination: "system_health",
    actions: [],
    ...over,
  });
}

function linksIn(container: HTMLElement): (string | null)[] {
  return [...container.querySelectorAll(".worklist-list li a")].map((link) =>
    link.getAttribute("href"),
  );
}

describe("a failed automation reaches the page that owns it", () => {
  it("links the row to the automations page", async () => {
    stub(
      day({
        queue: [aFailedRule()],
        summary: { urgent: 0, due: 0, lower_priority: 1, total: 1 },
      }),
    );
    const { container } = renderWorklist();

    await screen.findByText(/Notify sales on a new lead/);
    // The ADDRESS, not merely that a link exists: a row pointing at the wrong
    // screen renders exactly like one pointing at the right screen.
    expect(linksIn(container)).toContain("#/settings/models");
  });

  it("gives the same address to the AI work a rule set off", async () => {
    stub(
      day({
        queue: [
          aFailedRule({
            id: "01a05500-0000-7000-8000-0000000000a2",
            source: "ai_work_health",
            title: "A drafting run did not finish",
          }),
        ],
        summary: { urgent: 0, due: 0, lower_priority: 1, total: 1 },
      }),
    );
    const { container } = renderWorklist();

    await screen.findByText(/A drafting run did not finish/);
    expect(linksIn(container)).toContain("#/settings/models");
  });

  // The queue can send a reader somewhere; it cannot fix a rule. A button here
  // would promise a repair this surface does not perform — and Retry
  // specifically would promise one the SERVER cannot perform either.
  it("offers no verb, because the queue performs none", async () => {
    stub(
      day({
        queue: [aFailedRule()],
        summary: { urgent: 0, due: 0, lower_priority: 1, total: 1 },
      }),
    );
    renderWorklist();

    await screen.findByText(/Notify sales on a new lead/);
    expect(screen.queryByRole("button", { name: /Retry/i })).toBeNull();
    expect(screen.queryByRole("button", { name: /Investigate/i })).toBeNull();
  });
});
