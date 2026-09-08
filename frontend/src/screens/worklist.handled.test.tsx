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

  // The COMMON day, and the one the panel used to draw worst. An empty list is
  // what most days answer with — the contract says so — and the state derived
  // from the query's flags alone called it `ready`: a table's three column
  // names over no rows, which says neither "nothing was done" nor anything
  // else. The sentence is the whole answer, so the table must not be there
  // beside it drawing a header for rows that do not exist.
  it("says nothing was done rather than drawing an empty table", async () => {
    stubHandled({
      as_of: "2026-09-05T09:00:00Z",
      truncated: false,
      receipts: [],
    });

    render(panel());

    expect(await screen.findByText(en["worklist.handled.empty"])).toBeTruthy();
    expect(screen.queryByRole("table")).toBeNull();
    // And no figure in the footer band either: a count of nothing is a row of
    // chrome saying zero on a panel that has already said it in words.
    expect(screen.queryByText(/done for you/)).toBeNull();
  });

  // An answer carrying no list AT ALL is not a quiet day. `receipts` is
  // required on the wire, so its absence is version skew — and reading it as
  // empty would report a clear receipt over a response nobody could parse, on
  // the one surface a reader checks the product's own acts against.
  it("says it could not be read rather than reporting a clear day", async () => {
    stubHandled({ as_of: "2026-09-05T09:00:00Z", truncated: false });

    render(panel());

    expect(await screen.findByText(/Could not be loaded/)).toBeTruthy();
    expect(screen.queryByText(en["worklist.handled.empty"])).toBeNull();
    expect(screen.queryByRole("table")).toBeNull();
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
