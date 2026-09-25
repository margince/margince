/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import { LocaleProvider } from "../i18n";
import { ForecastReview } from "./analytics.forecast.review";
import { jsonResponse } from "./company.fixtures";

// The check a workspace has not had yet.
//
// What these hold is the honesty of the panel in the one state it used to have
// nothing to say in. A 404 from the assurance read means nobody has started
// this workspace — the nightly pass skips it — and a panel that drew that as an
// ordinary empty state would tell a reader the pipeline was found clean.

const PREVIEW = {
  started: false,
  eligible_deals: 340,
  findings: [
    { type: "close_past", severity: "high", count: 88 },
    { type: "no_next_step", severity: "low", count: 12 },
  ],
  readiness: "needs_review",
  sources: [{ source: "mail", state: "checked" }],
};

type Posted = { url: string };

// The server as a never-checked workspace presents it: the assurance read
// answers 404 and the preview answers what a first pass would find.
function show({
  conflict,
  preview = PREVIEW,
  refuse,
}: {
  conflict?: boolean;
  preview?: typeof PREVIEW;
  refuse?: { status: number; detail: string };
} = {}) {
  const posted: Posted[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      const request = input instanceof Request ? input : null;
      const url = String(request ? request.url : input);
      if (request?.method === "POST") {
        posted.push({ url });
        if (refuse) {
          return jsonResponse(
            {
              title: "Forbidden",
              status: refuse.status,
              detail: refuse.detail,
            },
            refuse.status,
          );
        }
        return conflict
          ? jsonResponse(
              {
                title: "Conflict",
                status: 409,
                detail: "a check is already running",
              },
              409,
            )
          : jsonResponse({ status: "enqueued" }, 202);
      }
      if (url.includes("/forecast/assurance/preview")) {
        return jsonResponse(preview);
      }
      if (url.includes("/forecast/assurance/exceptions")) {
        return jsonResponse({ data: [] });
      }
      if (url.includes("/forecast/assurance")) {
        return jsonResponse({ title: "Not found", status: 404 }, 404);
      }
      return jsonResponse({}, 404);
    }),
  );
  render(
    <QueryClientProvider
      client={
        new QueryClient({
          defaultOptions: {
            queries: { retry: false },
            mutations: { retry: false },
          },
        })
      }
    >
      <LocaleProvider initial="en">
        <ForecastReview />
      </LocaleProvider>
    </QueryClientProvider>,
  );
  return { posted };
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("a workspace nobody has started", () => {
  it("offers the check, and says how big the first one would be", async () => {
    show();

    // The size is the whole point of the offer: starting raises every finding
    // at once and mints a task per faulted deal, so a reader consenting to it
    // needs the number, not just the verb.
    expect(await screen.findByText(/340/)).toBeTruthy();
    expect(screen.getByText(/100/)).toBeTruthy();
    expect(screen.getByRole("button", { name: "Start checking" })).toBeTruthy();
  });

  it("says nothing has been checked rather than that nothing was found", async () => {
    show();

    await screen.findByRole("button", { name: "Start checking" });
    // A reader who takes an unchecked pipeline for a clean one has been told
    // the opposite of what happened, so the panel must never say "nothing to
    // check" in this state.
    expect(screen.queryByText("Nothing to check.")).toBeNull();
    expect(screen.getByText(/Nothing checked yet/)).toBeTruthy();
  });

  it("starts the check on the press, and then says it is running", async () => {
    const user = userEvent.setup();
    const { posted } = show();

    await user.click(
      await screen.findByRole("button", { name: "Start checking" }),
    );

    await waitFor(() => expect(posted.length).toBe(1));
    expect(posted[0]?.url).toContain("/forecast/assurance/runs");
    // The findings are not there yet — the pass runs in the background — so the
    // panel has to say so rather than fall back to the offer, which would read
    // as though the press did nothing.
    expect(await screen.findByText(/check is running/i)).toBeTruthy();
  });

  it("reads a refused second press as the check already running", async () => {
    const user = userEvent.setup();
    show({ conflict: true });

    await user.click(
      await screen.findByRole("button", { name: "Start checking" }),
    );

    // 409 is not a failure to report as one: the pass this reader wanted is
    // already under way, and an error notice would send them to fix something.
    expect(await screen.findByText(/check is running/i)).toBeTruthy();
  });

  // The misreading this whole surface exists to prevent, on the one screen
  // that had reproduced it: a preview that could not read its sources reports
  // no findings, and no findings is exactly what a clean pipeline reports.
  it("does not report a clean pipeline when it could not read the sources", async () => {
    show({
      preview: {
        ...PREVIEW,
        eligible_deals: 0,
        findings: [],
        readiness: "checks_incomplete",
        sources: [{ source: "mail", state: "unavailable" }],
      },
    });

    await screen.findByRole("button", { name: "Start checking" });
    // The zeroes must not be printed as a scope: a reader takes "0 findings"
    // for a sound pipeline and starts the cycle on it.
    expect(screen.queryByText(/Findings it would raise/)).toBeNull();
    // What is said instead names the source that went unread, so the reader
    // knows there is something to fix rather than nothing to do.
    expect(await screen.findByText(/mailbox/i)).toBeTruthy();
  });

  it("says why a refused press did nothing", async () => {
    const user = userEvent.setup();
    show({ refuse: { status: 403, detail: "you may not start a check" } });

    await user.click(
      await screen.findByRole("button", { name: "Start checking" }),
    );

    // A stopped spinner and nothing else is indistinguishable from a button
    // that is not wired up, and a seat without `forecast: create` sees exactly
    // that on every press.
    expect(await screen.findByText(/could not be started/i)).toBeTruthy();
  });
});
