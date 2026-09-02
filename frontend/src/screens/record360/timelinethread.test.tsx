// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment jsdom */
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { components } from "../../api/schema";
import type { RecordTimeline } from "../../design-system/recordtimeline";
import { LocaleProvider } from "../../i18n";
import { TimelineThread } from "./timelinethread";

type Activity = components["schemas"]["Activity"];

const FAILED = "The thread could not be read.";
const LOADING = "Loading this record’s history…";

function row(extra: Partial<Activity>): Activity {
  return {
    id: "a",
    kind: "email",
    direction: "outbound",
    occurred_at: "2026-08-01T09:00:00Z",
    is_done: false,
    source: "manual",
    captured_by: "human:u1",
    created_at: "2026-08-01T09:00:00Z",
    updated_at: "2026-08-01T09:00:00Z",
    ...extra,
  };
}

function thread(overrides: Partial<RecordTimeline> = {}): RecordTimeline {
  return {
    activities: [],
    hasNextPage: false,
    isFetchingNextPage: false,
    fetchNextPage: () => undefined,
    isPending: false,
    isSuccess: true,
    isError: false,
    refetch: () => undefined,
    ...overrides,
  };
}

function draw(source: RecordTimeline) {
  render(
    <LocaleProvider initial="en">
      <TimelineThread thread={source} />
    </LocaleProvider>,
  );
}

afterEach(cleanup);

describe("TimelineThread", () => {
  it("waits while the first page is in flight rather than drawing an empty thread", () => {
    draw(thread({ isPending: true, isSuccess: false }));
    expect(screen.getByRole("status").textContent).toContain(LOADING);
    expect(screen.queryByText("Today")).toBeNull();
  });

  it("says the thread is missing when the read failed with nothing on hand, and offers a retry", () => {
    const refetch = vi.fn();
    draw(thread({ isError: true, isSuccess: false, refetch }));
    expect(screen.getByText(FAILED)).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Retry" }));
    expect(refetch).toHaveBeenCalledTimes(1);
    expect(screen.queryByText("Today")).toBeNull();
  });

  it("keeps the thread it has when a later page fails to arrive", () => {
    // The read behind the thread is shared with the history tab. A Load more
    // there that fails flips the error flag on the whole query while the rows
    // already read are still held — and those rows ARE the thread.
    draw(
      thread({
        activities: [row({ id: "1", subject: "First word" })],
        hasNextPage: true,
        isError: true,
        isSuccess: false,
      }),
    );
    expect(screen.queryByText(FAILED)).toBeNull();
    expect(screen.getByText("Today")).toBeTruthy();
    expect(screen.getByText("More conversations before this")).toBeTruthy();
  });
});
