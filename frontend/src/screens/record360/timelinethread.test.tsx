// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment jsdom */
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
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

// A complete email summary, so the fixture keeps describing the wire: a cast
// one can drop a required field and go on compiling after the shape moves.
function summary(
  displayStatus: "team" | "withheld",
): NonNullable<Activity["email_summary"]> {
  return {
    activity_id: "a1",
    occurred_at: "2026-08-01T09:00:00Z",
    attachment_count: 0,
    move: "none",
    version: 1,
    display_status: displayStatus,
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

function draw(
  source: RecordTimeline,
  onOpenEmail?: (activityId: string) => void,
) {
  render(
    <LocaleProvider initial="en">
      <TimelineThread thread={source} onOpenEmail={onOpenEmail} />
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

  // The spine names each past conversation by its subject. Naming one without
  // a way in is the thing #3824 is about — it reads as the analytics being a
  // different kind of object from the mail they describe — and the deal and
  // the lead reach the spine through here, so this is where the opener has to
  // arrive.
  it("opens a conversation it names, through the page's own drawer", async () => {
    const opened: string[] = [];
    draw(
      thread({
        activities: [
          row({
            id: "a1",
            subject: "Renewal terms",
            email_summary: summary("team"),
          }),
        ],
      }),
      (id) => opened.push(id),
    );

    await userEvent.click(
      screen.getByRole("button", { name: /Renewal terms/ }),
    );
    expect(opened).toEqual(["a1"]);
  });

  // A message outside the reader's audience is NAMED and not offered: a
  // control that draws a placeholder teaches a reader that citations do not
  // work, which costs more than the press it saves.
  it("names a withheld conversation without offering to open it", () => {
    draw(
      thread({
        activities: [
          row({
            id: "a2",
            subject: "Renewal terms",
            email_summary: summary("withheld"),
          }),
        ],
      }),
      () => undefined,
    );

    expect(screen.getByText(/Renewal terms/)).toBeTruthy();
    expect(screen.queryByRole("button", { name: /Renewal terms/ })).toBeNull();
  });
});
