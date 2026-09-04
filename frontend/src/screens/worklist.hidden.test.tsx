// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment jsdom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { LocaleProvider } from "../i18n";
import { HiddenBacklogPanel } from "./worklist.hidden";

// The surface that reports the queue's own worst failure.
//
// Its healthy answer is a row of zeros, which is also what a broken read looks
// like — so what this file is about is the cases where the two must not render
// alike: a failed read, and a read the server cut short.

type HiddenBacklog = components["schemas"]["HiddenBacklog"];

function backlog(over: Partial<HiddenBacklog> = {}): HiddenBacklog {
  return {
    as_of: "2026-09-02T09:00:00Z",
    shown: 12,
    set_aside: 0,
    not_sales: 0,
    past_horizon: 0,
    unlinked: 0,
    colleagues: 0,
    truncated: false,
    clear: true,
    ...over,
  };
}

function draw(answer: HiddenBacklog | "fails", enabled = true) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async () =>
      answer === "fails"
        ? new Response(JSON.stringify({ code: "unavailable" }), {
            status: 503,
            headers: { "content-type": "application/problem+json" },
          })
        : new Response(JSON.stringify(answer), {
            status: 200,
            headers: { "content-type": "application/json" },
          }),
    ),
  );
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <HiddenBacklogPanel enabled={enabled} />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("the hidden-backlog panel", () => {
  it("names each rule that is holding work back, and how much", async () => {
    draw(backlog({ clear: false, past_horizon: 3, not_sales: 7 }));

    await waitFor(() =>
      expect(screen.getByText("Too old for the queue")).toBeTruthy(),
    );
    expect(screen.getByText("3 waiting")).toBeTruthy();
    expect(screen.getByText("Judged not sales work")).toBeTruthy();
    expect(screen.getByText("7 waiting")).toBeTruthy();
  });

  // A rule holding nothing back is not news. Four rows of zeros would bury the
  // one figure that found something.
  it("draws only the rules that found something", async () => {
    draw(backlog({ clear: false, past_horizon: 3 }));

    await waitFor(() =>
      expect(screen.getByText("Too old for the queue")).toBeTruthy(),
    );
    expect(screen.queryByText("Set aside by you")).toBeNull();
    expect(screen.queryByText("Judged not sales work")).toBeNull();
  });

  // THE failure this whole reading exists for. A read cut short by its own scan
  // bound reports every difference as zero, so the numbers look perfect at the
  // moment the check stopped working. The caveat has to be on screen before any
  // figure a reader might otherwise trust.
  it("says the figures are floors when the read was cut short", async () => {
    draw(backlog({ clear: false, truncated: true, past_horizon: 2 }));

    await waitFor(() =>
      expect(screen.getByText(/floors, not totals/)).toBeTruthy(),
    );
  });

  // Zeros are this surface's HEALTHY answer, so a failed read drawn as zeros
  // would report perfect health exactly when the guardrail broke.
  it("says it could not read rather than drawing a clean bill of health", async () => {
    draw("fails");

    // The PRESENT text, not merely the absent one: asserting only that the
    // clear message is missing passes against a component that rendered
    // nothing at all, which is how the first version of this test survived a
    // mutation that dropped the error state entirely.
    await waitFor(() =>
      expect(screen.getByText(/Could not be loaded/)).toBeTruthy(),
    );
    expect(screen.queryByText(/Nothing is being held back/)).toBeNull();
  });

  it("says so plainly when nothing is held back", async () => {
    draw(backlog());

    await waitFor(() =>
      expect(screen.getByText(/Nothing is being held back/)).toBeTruthy(),
    );
  });

  // A seat with no route to this reading does not fire the request. The figures
  // are gated server-side either way, so this is about not making a call behind
  // a reader's back rather than about safety.
  it("asks nothing when the reader has no tier for it", () => {
    const fetched = vi.fn();
    vi.stubGlobal("fetch", fetched);
    draw(backlog(), false);

    expect(fetched).not.toHaveBeenCalled();
  });
});
