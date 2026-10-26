import { describe, expect, it } from "vitest";
import type { components } from "../../api/schema";
import type { RecordTimeline } from "../../design-system/recordtimeline";
import { firstMessage } from "./leadcards";

type Activity = components["schemas"]["Activity"];

function row(extra: Partial<Activity>): Activity {
  return {
    offers_email: false,
    id: "a1",
    kind: "email",
    direction: "inbound",
    body: "We run 120 vans.",
    occurred_at: "2026-08-02T09:00:00Z",
    is_done: true,
    source: "capture",
    captured_by: "system",
    created_at: "2026-08-02T09:00:00Z",
    updated_at: "2026-08-02T09:00:00Z",
    ...extra,
  };
}

function timeline(activities: Activity[], hasNextPage = false): RecordTimeline {
  return {
    activities,
    hasNextPage,
    isFetchingNextPage: false,
    fetchNextPage: () => undefined,
    isPending: false,
    isSuccess: true,
    isError: false,
    refetch: () => undefined,
  };
}

describe("firstMessage", () => {
  it("takes the oldest message from the lead that has words in it", () => {
    const first = firstMessage(
      timeline([
        row({ id: "later", occurred_at: "2026-08-09T09:00:00Z" }),
        row({
          id: "ours",
          direction: "outbound",
          occurred_at: "2026-08-01T09:00:00Z",
        }),
        row({
          id: "hidden",
          content_state: "withheld",
          occurred_at: "2026-08-01T08:00:00Z",
        }),
        row({ id: "first" }),
      ]),
    );
    expect(first?.id).toBe("first");
  });

  // On a cut page the oldest row in hand is not the first they sent. The
  // card would quote the wrong message as their ask.
  it("names nothing while older pages are unread", () => {
    expect(firstMessage(timeline([row({})], true))).toBeUndefined();
  });
});
