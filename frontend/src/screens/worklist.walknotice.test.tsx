// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment jsdom */

import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { en } from "../i18n/en";
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
