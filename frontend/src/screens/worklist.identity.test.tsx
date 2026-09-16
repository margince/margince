// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { en } from "../i18n/en";
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
// real addresses that the Brief team board navigates to, and App.tsx hands that
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

// The queue is a MODAL on Home now rather than a screen of its own (#5560), so
// the chain from `#/worklist` to a worklist read is longer than the one these
// cases were written against: the redirect, Home's own render, the modal
// opening, and only then the read.
//
// They waited on the read alone with a two-second stopwatch, which on a loaded
// machine expired mid-chain and reported "expected 0 to be greater than 0" —
// true, and silent about which link was missing.
//
// So they wait for the queue to be OPEN first. A failure here says the redirect
// or the modal is broken; a failure on the read that follows says the screen
// asked the wrong question. Two failures that mean different things, and the
// second one is no longer racing the first one's work.
// A whole-APP mount and a redirect, not a component render: renderApp() boots
// the shell, the router resolves `#/worklist`, WorklistRedirect navigates, and
// Home renders before any of this is on screen. Testing Library's one-second
// default is sized for a component and expires mid-chain on a loaded machine.
//
// A deadline, not a delay: waitFor returns the moment the condition holds, so a
// generous one costs nothing on a fast machine and buys a true answer on a slow
// one. What it must not do is expire while the work is still going, which is
// the failure these two kept producing.
//
// Five seconds and not more, because scripts/test-budget.test.ts totals every
// waiter a case composes and derives the suite's own ceiling from the widest.
// This is the ONE generous wait in each case — the two that follow it keep the
// library default, since by the time the queue is open the mount is paid for —
// so a case spends 5000 + 1000 + 1000 and the suite ceiling does not move.
const APP_MOUNT_MS = 5_000;

async function queueIsOpen() {
  await screen.findByRole(
    "heading",
    { name: en["brief.queue.title"] },
    { timeout: APP_MOUNT_MS },
  );
}

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
      if (new URL(url, "https://test.local").pathname.endsWith("/worklist")) {
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
    await queueIsOpen();
    await waitFor(() => expect(READS.length).toBeGreaterThan(0));
    expect(READS[0]).toContain("scope=mine");
    const before = READS.length;

    // What the Brief team board's "unassigned" arm does.
    window.location.hash = "#/worklist/unassigned";

    // The read for the NEW question. Without a remount this never arrives, and
    // the screen keeps answering the old one.
    await waitFor(() =>
      expect(
        READS.slice(before).some((url) => url.includes("scope=unassigned")),
      ).toBe(true),
    );
  });

  it("re-reads for a colleague's queue when the address names a contact", async () => {
    window.location.hash = "#/worklist";
    renderApp();
    await queueIsOpen();
    await waitFor(() => expect(READS.length).toBeGreaterThan(0));
    const before = READS.length;

    // What the board's per-member arm does. The owner travels as a query
    // parameter rather than as the scope, so this is a second shape of the same
    // question and not a rephrasing of the case above.
    window.location.hash = "#/worklist/u-colleague";

    await waitFor(() =>
      expect(
        READS.slice(before).some((url) => url.includes("owner=u-colleague")),
      ).toBe(true),
    );
  });
});
