// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment jsdom */

// A bounced message names a customer who did not receive something.
//
// It is the one health row with a person on the other end of it, and the
// reader's move is to fix the address and send again. Neither happens here —
// both live on the person's page — so what this row owes the reader is a way
// THERE, and the defect it must not have is a row that reports the failure and
// offers nothing.

import { cleanup, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { day, renderWorklist, row, stub } from "./worklist.testkit";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("a bounced message is reachable", () => {
  it("routes the reader to the person whose address failed", async () => {
    stub(
      day({
        queue: [
          row({
            id: "bounce-1",
            source: "bounce",
            title: "Retrofit quote",
            detail: "The address does not exist.",
            actions: ["open"],
            subject: { type: "person", id: "p-1", label: "Kirsten Bauer" },
          }),
        ],
        summary: { urgent: 0, due: 0, lower_priority: 1, total: 1 },
      }),
    );

    renderWorklist();

    // The TITLE is the route: the copy layer names the send and the person it
    // failed for, and links the pair at the record where the address is fixed.
    // A row that reported the bounce and offered nothing would carry the same
    // words as plain text, so the assertion is the href rather than the name.
    const link = await screen.findByRole("link", {
      name: /Retrofit quote/,
    });
    expect(link.getAttribute("href")).toContain("p-1");
  });

  it("still draws the row when no person is named, without a verb it cannot honour", async () => {
    stub(
      day({
        queue: [
          row({
            id: "bounce-2",
            source: "bounce",
            title: "Retrofit quote",
            detail: "The address does not exist.",
            actions: [],
          }),
        ],
        summary: { urgent: 0, due: 0, lower_priority: 1, total: 1 },
      }),
    );

    renderWorklist();

    // The failure is still reported — a bounce nobody could file under a person
    // is still a customer who did not receive something, and dropping the row
    // would hide it. What it must NOT do is offer a way somewhere it cannot go.
    expect(await screen.findByText("The address does not exist.")).toBeTruthy();
    expect(screen.queryByRole("link", { name: /Retrofit quote/ })).toBeNull();
  });
});
