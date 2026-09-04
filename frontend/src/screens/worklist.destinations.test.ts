import { describe, expect, it } from "vitest";
import { destinationOf, reviewWork, sellerWork } from "./worklist.destinations";
import { row } from "./worklist.testkit";

// Which screen a row belongs on, and the one case that must not go wrong.

describe("splitting the day by destination", () => {
  it("keeps seller work and judgements apart", () => {
    const queue = [
      row({ id: "a", source: "customer_waiting", destination: "today" }),
      row({ id: "b", source: "approval", destination: "review" }),
      row({ id: "c", source: "sync_health", destination: "system_health" }),
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
