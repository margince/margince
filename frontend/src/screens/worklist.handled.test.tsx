// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment jsdom */

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { en } from "../i18n/en";
import { HandledForYouPanel } from "./worklist.handled";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

// What a reader is told about work already done, and the one thing this panel
// must never do.

describe("what was handled for the reader", () => {
  it("AC-WORKLIST-TRUST-01: reports what happened and offers nothing to do about it", async () => {
    stubHandled({
      as_of: "2026-09-05T09:00:00Z",
      truncated: false,
      receipts: [
        {
          id: "01a05500-0000-7000-8000-00000000e001",
          kind: "email_sent",
          summary: "Sent the confirmation to Kirsten",
          occurred_at: "2026-09-05T08:00:00Z",
          subject: { type: "person", id: "p1", label: "Kirsten Vogel" },
        },
      ],
    });

    render(panel());
    await screen.findByText("Sent the confirmation to Kirsten");

    expect(screen.getByText("Kirsten Vogel")).toBeTruthy();
    // NO VERBS. The work is done, and a control here would ask the reader to
    // redo it on the one surface that exists to tell them they need not.
    //
    // Asserted over the TABLE rather than the panel: Disclosure draws a native
    // <summary> to fold itself, which is not a button role and would let a
    // whole-panel assertion pass while a row carried a verb.
    const table = screen.getByRole("table");
    expect(
      table.querySelectorAll("button, a, input, [role='button']").length,
    ).toBe(0);
  });

  it("says no record where the act named none", async () => {
    stubHandled({
      as_of: "2026-09-05T09:00:00Z",
      truncated: false,
      receipts: [
        {
          id: "01a05500-0000-7000-8000-00000000e002",
          kind: "rule_ran",
          summary: "Reordered the follow-up queue",
          occurred_at: "2026-09-05T08:00:00Z",
          // Not every act is about a record. An absent subject is a real
          // state, not a missing field.
        },
      ],
    });

    render(panel());
    await screen.findByText("Reordered the follow-up queue");

    expect(screen.getByText(en["worklist.handled.noRecord"])).toBeTruthy();
  });

  it("admits a bounded read is not everything that was done", async () => {
    stubHandled({
      as_of: "2026-09-05T09:00:00Z",
      truncated: true,
      receipts: [
        {
          id: "01a05500-0000-7000-8000-00000000e003",
          kind: "email_sent",
          summary: "Sent the confirmation",
          occurred_at: "2026-09-05T08:00:00Z",
        },
      ],
    });

    render(panel());
    await screen.findByText("Sent the confirmation");

    // A reader who took this list for everything would close the page
    // believing they had seen it all.
    expect(screen.getByText(en["worklist.handled.truncated"])).toBeTruthy();
  });
});

function stubHandled(body: unknown) {
  vi.stubGlobal(
    "fetch",
    vi.fn(
      async () =>
        new Response(JSON.stringify(body), {
          status: 200,
          headers: { "content-type": "application/json" },
        }),
    ),
  );
}

function panel() {
  return (
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <LocaleProvider initial="en">
        <HandledForYouPanel />
      </LocaleProvider>
    </QueryClientProvider>
  );
}
