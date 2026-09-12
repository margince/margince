// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment jsdom */
import { cleanup, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { en } from "../i18n/en";
import { day, renderWorklist, row, stub } from "./worklist.testkit";

// A system row says WHICH thing is broken.
//
// Six of the queue's seven categories name a kind of work a reader recognises
// — a lead, a meeting, a decision. The seventh named the software: `system` is
// the catch-all for the product reporting on itself, so a mailbox that stopped,
// a failed automation and a bounced message all wore the word "System" and a
// reader running down the column learned nothing about any of them.
//
// `cause_label` is the server's own words for the condition, minted by the lane
// that knows which record IS the condition. It was on the row already and
// nothing drew it.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("the word above a system row", () => {
  it("names the broken thing rather than the software", async () => {
    stub(
      day({
        queue: [
          row({
            id: "run-1",
            // A failed automation, which is one of the three producers that
            // actually mints a label today (renderhealthlanes.go): the rule's
            // own name, beside a cause_ref that is its id.
            source: "automation_run",
            category: "system",
            level: 4,
            title: "Notify sales on a new lead",
            cause_label: "Notify sales on a new lead",
          }),
        ],
        summary: { urgent: 0, due: 0, lower_priority: 1, total: 1 },
      }),
    );
    renderWorklist();

    const eyebrow = (
      await screen.findAllByText("Notify sales on a new lead")
    )[0];
    // The category word is GONE from the row, not merely joined: two eyebrows
    // on one row would be the column saying the same thing twice. Scoped to the
    // ROW, because "System" is also a filter pill on this screen and a bare
    // query would find that instead and pass whatever the row drew.
    const kind = eyebrow.closest(".worklist-row-kind");
    expect(kind).not.toBeNull();
    expect(kind?.textContent).toBe("Notify sales on a new lead");
  });

  // Absent is a real answer. A lane whose candidate is a vocabulary rather than
  // a record mints no label, and an empty eyebrow is worse than a vague one.
  it("keeps the category word where the row named no cause", async () => {
    stub(
      day({
        queue: [
          row({
            // Sync health mints NO label, which is the common case rather than
            // an edge one: six of the nine system sources are like this.
            id: "sync-1",
            source: "sync_health",
            category: "system",
            level: 4,
            title: "Reconnect the mailbox",
          }),
        ],
        summary: { urgent: 0, due: 0, lower_priority: 1, total: 1 },
      }),
    );
    renderWorklist();

    await screen.findByText("Reconnect the mailbox");
    const kind = document.querySelector(".worklist-row-kind");
    expect(kind?.textContent).toBe(en["worklist.category.system"]);
  });
});
