// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment jsdom */

import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { en } from "../i18n/en";
import { day, renderWorklist, row } from "./worklist.testkit";
import { WalkNotice } from "./worklist.walknotice";

afterEach(cleanup);

// What a reader is told about a walk that has moved under them.

describe("what has moved since the reader started", () => {
  it("says nothing about a walk that has not moved", () => {
    render(
      notice({
        as_of: "2026-09-05T09:00:00Z",
        changed_since_snapshot: 0,
        new_available: 0,
      }),
    );

    // A line saying "0 new" would be noise on every page a reader turns.
    expect(screen.queryByRole("button")).toBeNull();
    expect(document.body.textContent?.trim()).toBe("");
  });

  it("offers a refresh for work that arrived behind the reader", async () => {
    const refreshed = vi.fn();
    render(
      notice(
        {
          as_of: "2026-09-05T09:00:00Z",
          changed_since_snapshot: 0,
          new_available: 3,
        },
        refreshed,
      ),
    );

    // The remedy is ON the notice. Naming the state and leaving the reader to
    // find the way to act on it is half an answer.
    await userEvent
      .setup()
      .click(screen.getByRole("button", { name: en["worklist.walk.refresh"] }));
    expect(refreshed).toHaveBeenCalledTimes(1);
  });

  it("explains a count that fell without offering a refresh", () => {
    render(
      notice({
        as_of: "2026-09-05T09:00:00Z",
        // Dealt with since the walk started. Refreshing does not bring these
        // back, so offering it here would point at the wrong remedy.
        changed_since_snapshot: 2,
        new_available: 0,
      }),
    );

    expect(screen.queryByRole("button")).toBeNull();
    expect(screen.getByText(/2/)).toBeTruthy();
  });

  it("reports arrivals and departures separately", () => {
    render(
      notice({
        as_of: "2026-09-05T09:00:00Z",
        changed_since_snapshot: 2,
        new_available: 3,
      }),
    );

    // NOT netted off. "Three arrived and two left" is two facts a reader acts
    // on differently; one net figure would hide both behind a number that
    // means neither.
    const text = document.body.textContent ?? "";
    expect(text).toContain("3");
    expect(text).toContain("2");
  });
});

function notice(
  walk: {
    as_of: string;
    changed_since_snapshot: number;
    new_available?: number;
  },
  onRefresh: () => void = () => {},
) {
  return (
    <LocaleProvider initial="en">
      <WalkNotice walk={walk} onRefresh={onRefresh} />
    </LocaleProvider>
  );
}

// Refreshing must start a NEW walk, not resume the frozen one.
//
// The whole point of the notice is that work arrived which this walk cannot
// show. A refetch of an infinite query re-runs every loaded page with its own
// stored cursor, and pages past the first carry the SNAPSHOT — so it would
// resume the same frozen walk, the newcomers would still be absent, and the
// notice would sit there after the reader did what it asked.
describe("refreshing starts a new walk", () => {
  it("asks for a first page with no cursor", async () => {
    const asked: string[] = [];
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL) => {
        const url = String(input instanceof Request ? input.url : input);
        if (url.includes("/worklist") && !url.includes("/worklist/")) {
          asked.push(url);
          return new Response(
            JSON.stringify(
              day({
                summary: { urgent: 0, due: 0, lower_priority: 1, total: 1 },
                queue: [
                  row({ id: "t1", source: "task", title: "Send the quote" }),
                ],
                next_cursor: "page-two",
                walk: {
                  as_of: "2026-09-05T09:00:00Z",
                  changed_since_snapshot: 0,
                  new_available: 3,
                },
              }),
            ),
            { status: 200, headers: { "content-type": "application/json" } },
          );
        }
        return new Response(JSON.stringify({ data: [] }), {
          status: 200,
          headers: { "content-type": "application/json" },
        });
      }),
    );
    renderWorklist();

    await screen.findByText("Send the quote");
    // PAGE FIRST, which is the case the concern is about: with only page one
    // loaded there is no cursor to resume and any implementation passes.
    await userEvent.click(
      await screen.findByRole("button", { name: en["worklist.more"] }),
    );
    await waitFor(() => {
      expect(asked.some((url) => url.includes("cursor="))).toBe(true);
    });
    const before = asked.length;
    await userEvent.click(
      await screen.findByRole("button", { name: en["worklist.walk.refresh"] }),
    );

    await waitFor(() => {
      expect(asked.length).toBeGreaterThan(before);
    });
    // The request the refresh made carries NO cursor. One that did would
    // resume the walk the reader is trying to leave.
    const refreshed = asked[asked.length - 1];
    expect(refreshed).not.toContain("cursor=");
  });
});
