// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment jsdom */
import { waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import {
  memoryStorage,
  renderApp,
  sessionOnlyFetch,
} from "../testing/appharness";

// Why the worklist keeps WHOLE_ADDRESS in app/router.tsx's IDENTITY_DEPTH.
//
// Lowering it looks free. Selecting a row on this screen is local state and
// never touches the address, so the obvious reading is that the screen has one
// address and remounting it is waste — which is what a plan to lower it said.
//
// It is not one address. `#/worklist/<owner>` and `#/worklist/unassigned` are
// real addresses that the home team board navigates to, and App.tsx hands that
// segment to the screen as `opensOn`. The screen reads it in useState
// INITIALISERS, so it is used once at mount and never again.
//
// So the remount IS the mechanism: at WHOLE_ADDRESS the two addresses have
// different identities, the screen remounts, and the initialisers re-run with
// the new owner. At depth 1 they share one identity, nothing remounts, the
// initialisers never re-run — and a manager who clicks a colleague's name goes
// on reading their own queue under that colleague's address, with no error and
// nothing on screen to say so.
//
// Held here rather than as a comment because a comment cannot fail. Lower the
// depth and this test says what breaks.

const READS: string[] = [];

beforeEach(() => {
  READS.length = 0;
  vi.stubGlobal("localStorage", memoryStorage());
  globalThis.localStorage.setItem("margince.workspaceSlug", "acme");
  const session = sessionOnlyFetch();
  // Every worklist read this navigation causes, in order. The page 503s into
  // its own error state, which is fine: this is about WHICH question the screen
  // asks the server, not about what comes back.
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: Request | string | URL) => {
      const url = String(input instanceof Request ? input.url : input);
      if (url.includes("/worklist")) {
        READS.push(url.replace(/^.*\/v1/, ""));
      }
      return session(input);
    }),
  );
});

describe("the worklist's routed identity", () => {
  it("re-reads for the named queue when the address names one", async () => {
    window.location.hash = "#/worklist";
    renderApp();
    await waitFor(() => expect(READS.length).toBeGreaterThan(0), {
      timeout: 2000,
    });
    expect(READS[0]).toContain("scope=mine");
    const before = READS.length;

    // What the home team board's "unassigned" arm does.
    window.location.hash = "#/worklist/unassigned";

    // The read for the NEW question. Without a remount this never arrives, and
    // the screen keeps answering the old one.
    await waitFor(
      () =>
        expect(
          READS.slice(before).some((url) => url.includes("scope=unassigned")),
        ).toBe(true),
      { timeout: 2000 },
    );
  });

  it("re-reads for a colleague's queue when the address names a person", async () => {
    window.location.hash = "#/worklist";
    renderApp();
    await waitFor(() => expect(READS.length).toBeGreaterThan(0), {
      timeout: 2000,
    });
    const before = READS.length;

    // What the board's per-member arm does. The owner travels as a query
    // parameter rather than as the scope, so this is a second shape of the same
    // question and not a rephrasing of the case above.
    window.location.hash = "#/worklist/u-colleague";

    await waitFor(
      () =>
        expect(
          READS.slice(before).some((url) => url.includes("owner=u-colleague")),
        ).toBe(true),
      { timeout: 2000 },
    );
  });
});
