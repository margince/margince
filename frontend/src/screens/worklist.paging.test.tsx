// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment jsdom */
import { cleanup, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { day, renderWorklist, row, stub, stubWalk } from "./worklist.testkit";

// Reaching the whole backlog.
//
// The page used to stop at its first read with no route to what was behind it:
// the header said work existed and the queue offered no way to it. These cases
// are about the two halves of fixing that — the rows accumulate, and the day's
// own figures do not move while they do.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("walking to the rest of the queue", () => {
  it("loads the next page and keeps what was already read", async () => {
    stubWalk([
      day({
        queue: [row({ id: "a", title: "First thing" })],
        summary: { urgent: 0, due: 2, lower_priority: 0, total: 2 },
        next_cursor: "page-2",
      }),
      day({
        queue: [row({ id: "b", title: "Second thing" })],
        summary: { urgent: 0, due: 2, lower_priority: 0, total: 2 },
      }),
    ]);
    renderWorklist();

    await screen.findByText("First thing");
    await userEvent.click(screen.getByRole("button", { name: "Show more" }));

    // BOTH, not just the newest page: a walk that replaced the list would lose
    // the rows the reader already worked through.
    await screen.findByText("Second thing");
    expect(screen.getByText("First thing")).toBeTruthy();
  });

  it("offers no way on when the walk is finished", async () => {
    stub(
      day({
        queue: [row({ id: "a", title: "Only thing" })],
        summary: { urgent: 0, due: 1, lower_priority: 0, total: 1 },
      }),
    );
    renderWorklist();

    await screen.findByText("Only thing");
    // A final page carries no cursor. A control offered here would ask the
    // server for a page it already said does not exist.
    expect(screen.queryByRole("button", { name: "Show more" })).toBeNull();
  });

  // THE case the contract warns about. A walk is not a snapshot: the day is
  // re-assembled and re-ranked on every read, so a row crossing the page
  // boundary between two reads is served twice. React would render duplicate
  // keys for it, and the reader would see the same work listed twice.
  it("draws a row served on two pages once", async () => {
    const straddler = row({ id: "a", title: "Served twice" });
    stubWalk([
      day({
        queue: [straddler],
        summary: { urgent: 0, due: 2, lower_priority: 0, total: 2 },
        next_cursor: "page-2",
      }),
      day({
        queue: [straddler, row({ id: "b", title: "Genuinely new" })],
        summary: { urgent: 0, due: 2, lower_priority: 0, total: 2 },
      }),
    ]);
    renderWorklist();

    await screen.findByText("Served twice");
    await userEvent.click(screen.getByRole("button", { name: "Show more" }));

    await screen.findByText("Genuinely new");
    expect(screen.getAllByText("Served twice")).toHaveLength(1);
  });

  // Two lanes mint ids independently, so the same id in two sources is two
  // different pieces of work. Deduping on the id alone would silently drop the
  // second one.
  it("keeps two rows that share an id across different sources", async () => {
    stubWalk([
      day({
        queue: [row({ id: "same", source: "task", title: "A task" })],
        summary: { urgent: 0, due: 2, lower_priority: 0, total: 2 },
        next_cursor: "page-2",
      }),
      day({
        queue: [
          row({
            id: "same",
            source: "notice",
            category: "system",
            title: "A notice",
          }),
        ],
        summary: { urgent: 0, due: 2, lower_priority: 0, total: 2 },
      }),
    ]);
    renderWorklist();

    await screen.findByText("A task");
    await userEvent.click(screen.getByRole("button", { name: "Show more" }));

    await screen.findByText("A notice");
    expect(screen.getByText("A task")).toBeTruthy();
  });

  // The header describes the assembled DAY, the queue describes what is
  // loaded. Reading the figures off the latest page instead would make the
  // summary shrink as the reader pages further in.
  it("keeps the day's figures still while the rows grow", async () => {
    stubWalk([
      day({
        queue: [row({ id: "a", title: "First thing" })],
        summary: { urgent: 3, due: 9, lower_priority: 6, total: 48 },
        next_cursor: "page-2",
      }),
      day({
        queue: [row({ id: "b", title: "Second thing" })],
        // A later page's own figures, which must NOT reach the header.
        summary: { urgent: 0, due: 0, lower_priority: 0, total: 48 },
      }),
    ]);
    renderWorklist();

    await screen.findByText(/3 urgent/);
    await userEvent.click(screen.getByRole("button", { name: "Show more" }));

    await screen.findByText("Second thing");
    await waitFor(() => {
      expect(screen.getByText(/3 urgent/)).toBeTruthy();
    });
  });
});
