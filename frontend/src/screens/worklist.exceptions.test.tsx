// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment jsdom */

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { en } from "../i18n/en";
import { TeamExceptionsPanel } from "./worklist.exceptions";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

// What a lead is shown, and the two things the panel must not do.

describe("what needs the lead", () => {
  it("names the basis each row was judged against", async () => {
    stubExceptions({
      as_of: "2026-09-05T09:00:00Z",
      truncated: false,
      exceptions: [
        {
          kind: "response_breached",
          owner: { kind: "user", id: "u1", label: "Lena Fischer" },
          subject: { type: "lead", id: "l1", label: "Kirsten at LOXXESS" },
          since: "2026-09-05T07:00:00Z",
          consequence: "customer_waits",
          // The policy's own state, not a number invented for the reading.
          threshold: "breached",
        },
      ],
    });

    renderPanel();

    expect(await screen.findByText("Kirsten at LOXXESS")).toBeTruthy();
    // The BASIS is on screen. A verdict without it is one a lead cannot
    // dispute, which is the whole reason the server carries the field.
    expect(screen.getByText("breached")).toBeTruthy();
    expect(screen.getByText("Lena Fischer")).toBeTruthy();
  });

  it("says nobody rather than printing a raw id", async () => {
    stubExceptions({
      as_of: "2026-09-05T09:00:00Z",
      truncated: false,
      exceptions: [
        {
          kind: "unassigned",
          // No label and no id: work nobody has taken.
          owner: { kind: "unassigned" },
          subject: { type: "deal", id: "d1", label: "Acme expansion" },
          since: "2026-09-05T07:00:00Z",
          consequence: "deal_drifts",
          threshold: "no owner stated by the lane that raised it",
        },
      ],
    });

    renderPanel();

    await screen.findByText("Acme expansion");
    expect(screen.getByText(en["worklist.exceptions.nobody"])).toBeTruthy();
  });

  it("admits a bounded page is not a clear team", async () => {
    stubExceptions({
      as_of: "2026-09-05T09:00:00Z",
      // The server read to its own bound. A lead taking this list for the
      // whole of it would stop looking exactly where the rest begins.
      truncated: true,
      exceptions: [
        {
          kind: "revenue_at_risk",
          owner: { kind: "user", id: "u1", label: "Lena Fischer" },
          subject: { type: "deal", id: "d2", label: "LOXXESS renewal" },
          since: "2026-09-05T07:00:00Z",
          consequence: "deal_drifts",
          threshold: "at or above the pipeline's median open deal",
        },
      ],
    });

    renderPanel();

    await screen.findByText("LOXXESS renewal");
    expect(screen.getByText(en["worklist.exceptions.truncated"])).toBeTruthy();
  });

  it("asks nothing when the reader cannot hold the tier", async () => {
    const fetched = stubExceptions({
      as_of: "2026-09-05T09:00:00Z",
      truncated: false,
      exceptions: [],
    });

    render(
      <QueryClientProvider
        client={
          new QueryClient({ defaultOptions: { queries: { retry: false } } })
        }
      >
        <LocaleProvider initial="en">
          <TeamExceptionsPanel enabled={false} onOwner={() => {}} />
        </LocaleProvider>
      </QueryClientProvider>,
    );

    // A rep asking earns a 403, and a panel that rendered that as an error
    // would tell them a surface exists which is not theirs.
    expect(fetched.mock.calls.length).toBe(0);
  });
});

function stubExceptions(body: unknown) {
  const fetched = vi.fn(
    async () =>
      new Response(JSON.stringify(body), {
        status: 200,
        headers: { "content-type": "application/json" },
      }),
  );
  vi.stubGlobal("fetch", fetched);
  return fetched;
}

function renderPanel() {
  return render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <LocaleProvider initial="en">
        <TeamExceptionsPanel enabled onOwner={() => {}} />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}
