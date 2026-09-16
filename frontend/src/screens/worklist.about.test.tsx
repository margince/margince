/** @vitest-environment happy-dom */
import { cleanup, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { day, renderWorklist, row, stub } from "./worklist.testkit";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

// Every other row names and links its record on its title (`itemTitle`,
// `rowHref`); a message names its sender as text and draws no title line, so
// it is the one row that links the record it is about in its facts line.
it("links the record a message row is about", async () => {
  stub(
    day({
      queue: [
        row({
          id: "w1",
          source: "customer_waiting",
          category: "customer_waiting",
          title: "Meet next Tues?",
          subject: {
            type: "contact",
            id: "01a05500-0000-7000-8000-000000000009",
            label: "Sonya Beck",
          },
          email_summary: {
            activity_id: "mail-1",
            subject: "Meet next Tues?",
            preview: "Let's meet next Tues instead of Monday.",
            occurred_at: "2026-09-03T16:46:00Z",
            direction: "inbound",
            counterparty: "Sonya Beck",
            attachment_count: 0,
            move: "needs_reply",
            display_status: "team",
            version: 1,
          },
        }),
      ],
      summary: { urgent: 1, due: 0, lower_priority: 0, total: 1 },
    }),
  );
  renderWorklist();

  // The pane beside the queue names her too; the row's own link is the one
  // in its facts line.
  await screen.findAllByRole("link", { name: "Sonya Beck" });
  expect(
    document.querySelector(".worklist-row-about a")?.getAttribute("href"),
  ).toBe("#/contacts/01a05500-0000-7000-8000-000000000009");
});
