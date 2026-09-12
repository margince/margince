// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, render, screen } from "@testing-library/react";
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
  it("AC-WORKLIST-TRUST-01: offers nothing to do about work somebody agreed to", async () => {
    stubHandled({
      as_of: "2026-09-05T09:00:00Z",
      truncated: false,
      receipts: [
        {
          id: "01a05500-0000-7000-8000-00000000e001",
          kind: "email_sent",
          summary: "Sent the confirmation to Kirsten",
          occurred_at: "2026-09-05T08:00:00Z",
          subject: { type: "contact", id: "p1", label: "Kirsten Vogel" },
        },
      ],
    });

    render(panel());
    await screen.findByText("Sent the confirmation to Kirsten");

    expect(screen.getByText("Kirsten Vogel")).toBeTruthy();
    // NO VERBS on a receipt for a DECISION. The work was agreed to and is
    // done, so a control here would ask the reader to redo it on the one
    // surface that exists to tell them they need not. A correction nobody was
    // asked about is the deliberate exception, covered below.
    //
    // Asserted over the TABLE rather than the panel: Disclosure draws a native
    // <summary> to fold itself, which is not a button role and would let a
    // whole-panel assertion pass while a row carried a verb.
    const table = screen.getByRole("table");
    expect(
      table.querySelectorAll("button, a, input, [role='button']").length,
    ).toBe(0);
  });

  // The one row on this panel that carries a verb, and why it must.
  //
  // A close date the nightly sweep corrected was never staged as a card and
  // never agreed to by anyone. This receipt is its ONLY telling, so if the
  // reader disagrees, the way back has to be here — there is no approvals row
  // to go and reject.
  it("offers the way back on a correction nobody was asked about", async () => {
    stubHandled({
      as_of: "2026-09-05T09:00:00Z",
      truncated: false,
      receipts: [
        {
          id: "01a05500-0000-7000-8000-00000000e004",
          kind: "close_date_correction",
          summary: 'Corrected the close date on "Ablösung Checkout"',
          occurred_at: "2026-09-05T08:00:00Z",
          subject: { type: "deal", id: "d1", label: "Ablösung Checkout" },
          undo: {
            audit_log_id: "01a05500-0000-7000-8000-0000000000a1",
            version: 4,
            reversed: false,
          },
        },
      ],
    });

    render(panel());
    await screen.findByText('Corrected the close date on "Ablösung Checkout"');

    expect(
      screen.getByRole("button", { name: en["history.undo.action"] }),
    ).toBeTruthy();
  });

  // A correction already put back keeps its row and says so. Dropping the row
  // on success would leave the reader unsure whether their press landed or the
  // list simply moved under them.
  it("says a correction was already put back instead of offering it twice", async () => {
    stubHandled({
      as_of: "2026-09-05T09:00:00Z",
      truncated: false,
      receipts: [
        {
          id: "01a05500-0000-7000-8000-00000000e005",
          kind: "close_date_correction",
          summary: 'Corrected the close date on "Ablösung Checkout"',
          occurred_at: "2026-09-05T08:00:00Z",
          subject: { type: "deal", id: "d1", label: "Ablösung Checkout" },
          undo: {
            audit_log_id: "01a05500-0000-7000-8000-0000000000a2",
            version: 4,
            reversed: true,
          },
        },
      ],
    });

    render(panel());
    await screen.findByText(en["worklist.handled.putBackDone"]);

    expect(
      screen.queryByRole("button", { name: en["history.undo.action"] }),
    ).toBeNull();
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

  // A CACHED list outlives the read that failed. The rows stay in the cache
  // while the refetch errors, so a count taken off the payload alone stood in
  // the footer band reporting a total beside a body saying the receipts could
  // not be read — and of those two the number is the one a reader believes.
  it("withholds the count when a refetch over the cached list fails", async () => {
    stubHandled({
      as_of: "2026-09-05T09:00:00Z",
      truncated: false,
      receipts: [
        {
          id: "01a05500-0000-7000-8000-00000000e004",
          kind: "email_sent",
          summary: "Sent the confirmation",
          occurred_at: "2026-09-05T08:00:00Z",
        },
      ],
    });
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    render(panel(client));
    await screen.findByText("1 done for you");

    // Through the panel's OWN query rather than a second render over a failing
    // stub: what has to be reached is the state where the rows are still in the
    // cache and the last read failed, which only a refetch produces.
    vi.stubGlobal(
      "fetch",
      vi.fn(
        async () =>
          new Response(
            JSON.stringify({ title: "The receipts could not be read" }),
            {
              status: 502,
              headers: { "content-type": "application/problem+json" },
            },
          ),
      ),
    );
    await act(async () => {
      await client.refetchQueries();
    });

    expect(await screen.findByText(en["state.failed"])).toBeTruthy();
    expect(screen.queryByText(/done for you/)).toBeNull();
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

// The client is a parameter for the one test that has to reach the panel's own
// query after it has answered once; every other frame is a first read and does
// not care which client holds it.
function panel(
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } }),
) {
  return (
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <HandledForYouPanel />
      </LocaleProvider>
    </QueryClientProvider>
  );
}
