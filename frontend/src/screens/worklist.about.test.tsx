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

// The server names the contact behind a thread even when the thread is filed
// under a deal, and says when each side last wrote: a rep reads both before
// choosing how to answer, on the row, with nothing to open.
it("names the sender behind a deal-filed thread, and which side wrote last", async () => {
  stub(
    day({
      queue: [
        row({
          id: "w2",
          source: "customer_waiting",
          category: "customer_waiting",
          title: "Re: pricing for the retrofit",
          subject: {
            type: "deal",
            id: "01a05500-0000-7000-8000-00000000000d",
            label: "Retrofit",
          },
          contact: {
            id: "01a05500-0000-7000-8000-000000000009",
            label: "Sonya Beck",
            touch: {
              last_inbound_at: "2026-09-03T16:46:00Z",
              last_outbound_at: null,
            },
          },
          email_summary: {
            activity_id: "mail-2",
            subject: "Re: pricing for the retrofit",
            preview: "Can you send the revised figures?",
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

  await screen.findAllByRole("link", { name: "Sonya Beck" });
  expect(
    document.querySelector(".worklist-row-about a")?.getAttribute("href"),
  ).toBe("#/contacts/01a05500-0000-7000-8000-000000000009");
  const touch = document.querySelector(".worklist-row-touch");
  expect(touch?.textContent).toContain("Last inbound");
  expect(touch?.textContent).toContain("03/09/2026");
  expect(touch?.textContent).toContain("Last outbound Never");
});

// A row whose contact the reader may see but whose activity they may not:
// the name stands, and no date is claimed either way.
it("claims no moments when the server withheld them", async () => {
  stub(
    day({
      queue: [
        row({
          id: "t1",
          source: "task",
          category: "tasks",
          title: "Confirm the workshop date",
          contact: {
            id: "01a05500-0000-7000-8000-000000000009",
            label: "Sonya Beck",
          },
        }),
      ],
      summary: { urgent: 0, due: 1, lower_priority: 0, total: 1 },
    }),
  );
  renderWorklist();

  await screen.findByText("Confirm the workshop date");
  expect(document.querySelector(".worklist-row-touch")).toBeNull();
});
