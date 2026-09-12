// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Answering a waiting buyer from the row that named the wait.
//
// The row a rep meets most often is somebody who wrote and nobody answered, and
// it used to send them to the record to press Reply there.

/** @vitest-environment jsdom */
import { cleanup, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { day, renderWorklist, row, stub } from "./worklist.testkit";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function aWaitingBuyer(over = {}) {
  return row({
    id: "01a05500-0000-7000-8000-0000000000c1",
    source: "customer_waiting",
    category: "customer_waiting",
    title: "can you resend the quote?",
    band: "now",
    destination: "today",
    actions: ["open", "reply"],
    subject: { type: "contact", id: "01a05500-0000-7000-8000-0000000000c2" },
    ...over,
  });
}

function aDayWith(item: ReturnType<typeof aWaitingBuyer>) {
  return day({
    queue: [item],
    summary: { urgent: 0, due: 1, lower_priority: 0, total: 1 },
  });
}

describe("a buyer waiting on a reply", () => {
  it("can be answered from the row", async () => {
    stub(aDayWith(aWaitingBuyer()));
    renderWorklist();

    await screen.findByText(/can you resend the quote/);
    expect(
      await screen.findByRole("button", { name: /^reply$/i }),
    ).not.toBeNull();
  });

  it("offers no reply when the server sent none", async () => {
    // The server withholds the verb for a channel message and for a wait that
    // names no record. Either way the row must draw nothing rather than a
    // composer that would answer in the wrong place or file nowhere.
    stub(aDayWith(aWaitingBuyer({ actions: ["open"] })));
    renderWorklist();

    await screen.findByText(/can you resend the quote/);
    expect(screen.queryByRole("button", { name: /^reply$/i })).toBeNull();
  });

  it("offers no reply when the row names no record to file against", async () => {
    // A message from a stranger. The verb alone is not enough: the composer
    // links the sent message to a record, and the row's subject is what says
    // which. Without it there is nothing to open the composer on.
    stub(aDayWith(aWaitingBuyer({ subject: undefined })));
    renderWorklist();

    await screen.findByText(/can you resend the quote/);
    expect(screen.queryByRole("button", { name: /^reply$/i })).toBeNull();
  });
});
