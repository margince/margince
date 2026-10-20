import { describe, expect, it } from "vitest";
import { row } from "../../testing/worklist.testkit";
import {
  destinationOf,
  reviewShortfall,
  reviewWork,
  sellerWork,
} from "./worklist.destinations";

// Which screen a row belongs on, and the one case that must not go wrong.

describe("splitting the day by destination", () => {
  it("keeps seller work and judgements apart", () => {
    const queue = [
      row({ id: "a", source: "customer_waiting", destination: "today" }),
      row({ id: "b", source: "approval", destination: "review" }),
      row({ id: "c", source: "capture_health", destination: "system_health" }),
      row({ id: "d", source: "task", destination: "today" }),
    ];

    expect(sellerWork(queue).map((item) => item.id)).toEqual(["a", "d"]);
    expect(reviewWork(queue).map((item) => item.id)).toEqual(["b", "c"]);
  });

  it("loses no row between the two", () => {
    const queue = [
      row({ id: "a", destination: "today" }),
      row({ id: "b", destination: "review" }),
      row({ id: "c", destination: "receipt" }),
    ];

    // The two halves partition the queue. A row in neither would be work the
    // reader can no longer reach from any screen, which is the failure a split
    // introduces and nothing else here would catch.
    expect(sellerWork(queue).length + reviewWork(queue).length).toBe(
      queue.length,
    );
  });

  it("reads an unclassified row as seller work", () => {
    // An older server sends no destination. Treating that as review would
    // empty a rep's day the moment they talked to one — every row swept onto a
    // screen they were not looking at — while treating it as today leaves the
    // queue exactly as it was before the split existed.
    const legacy = row({ id: "old" });
    delete (legacy as { destination?: unknown }).destination;

    expect(destinationOf(legacy)).toBe("today");
    expect(sellerWork([legacy]).map((item) => item.id)).toEqual(["old"]);
    expect(reviewWork([legacy])).toEqual([]);
  });
});

// What the Review panel is allowed to say about its own completeness.

describe("saying how much review work is not on screen", () => {
  it("reports the shortfall when the day holds more than the panel", () => {
    expect(reviewShortfall(2, 7)).toEqual({
      loaded: 2,
      total: 7,
      floor: false,
    });
  });

  it("says nothing once the panel holds the whole day", () => {
    // "3 of 3" is noise on a complete list, and the completeness line above
    // the queue falls silent on the same condition.
    expect(reviewShortfall(3, 3)).toBeNull();
  });

  it("says nothing when the walk outran the day's own count", () => {
    // The two are counted over different populations — the day's candidates
    // against the rows this walk has served — so a loaded count above the
    // total is not a shortfall. Drawing "5 of 3" reads as a broken product.
    expect(reviewShortfall(5, 3)).toBeNull();
  });

  it("never calls a group cut at its read limit complete", () => {
    expect(reviewShortfall(200, 200, true)).toEqual({
      loaded: 200,
      total: 200,
      floor: true,
    });
  });

  it("says nothing when the server sends no partition", () => {
    // An older server has no buckets. A figure invented here would be a claim
    // about a day this client cannot count.
    expect(reviewShortfall(2, undefined)).toBeNull();
  });
});

describe("review totals follow the selected cut", () => {
  it("does not compare a filtered panel with the whole day's review total", () => {
    expect(reviewShortfall(2, 9, false, "decisions")).toBeNull();
    expect(reviewShortfall(2, 9, false, "system")).toBeNull();
    expect(reviewShortfall(2, 9, false, "all")).toEqual({
      loaded: 2,
      total: 9,
      floor: false,
    });
  });
  it("keeps a bounded filtered group explicit without borrowing the day total", () => {
    expect(reviewShortfall(2, 9, true, "system")).toEqual({
      loaded: 2,
      total: 2,
      floor: true,
    });
  });
});
