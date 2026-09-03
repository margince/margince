import { describe, expect, it } from "vitest";
import {
  dealRecordKeys,
  dealWinKeys,
  derivedRecordKeys,
  entityTimelineKeys,
  showsAMessage,
  taskWriteKeys,
} from "./activitykeys";

// The company, contact and project pages draw the timeline's first page from
// their composite 360 read and fetch nothing under the per-record activities
// key until the reader asks for more. A write that invalidated only the
// latter succeeded and showed nothing.

describe("which reads a timeline write has to invalidate", () => {
  it("names the composite read that seeds the timeline, spelled as that page spells it", () => {
    expect(entityTimelineKeys("organization", "o1")).toEqual([
      ["activities", "organization", "o1"],
      ["organization360", "o1"],
    ]);
    expect(entityTimelineKeys("person", "p1")).toEqual([
      ["activities", "person", "p1"],
      ["person360", "p1"],
    ]);
    expect(entityTimelineKeys("project", "j1")).toEqual([
      ["activities", "project", "j1"],
      ["project", "j1", "360"],
    ]);
  });

  it("names the deal status card, which is WRITTEN FROM the timeline it does not show", () => {
    // A stale timeline visibly lacks the new row. A stale card says something
    // confident about a deal that has since moved, which reads as a current
    // judgement rather than a missing update.
    expect(entityTimelineKeys("deal", "d1")).toEqual([
      ["activities", "deal", "d1"],
      ["deal-status", "d1"],
    ]);
  });

  it("names only the record's own timeline for a kind with no seeded page", () => {
    for (const kind of ["lead"] as const) {
      expect(entityTimelineKeys(kind, "x1")).toEqual([
        ["activities", kind, "x1"],
      ]);
    }
  });

  it("adds the workspace work queue when the write is a task", () => {
    expect(taskWriteKeys("organization", "o1")).toEqual([
      ["activities", "organization", "o1"],
      ["organization360", "o1"],
      ["tasks"],
    ]);
  });

  it("a won deal reaches the project list, the project's page and the company page that embeds it", () => {
    expect(dealWinKeys({ project_id: "j1", organization_id: "o1" })).toEqual([
      ["projects"],
      ["project", "j1"],
      ["organization360", "o1"],
    ]);
  });

  it("a won deal naming no project still refreshes the project list and nothing it cannot name", () => {
    expect(dealWinKeys({ project_id: null, organization_id: null })).toEqual([
      ["projects"],
    ]);
    expect(dealWinKeys(undefined)).toEqual([["projects"]]);
  });
});

describe("which reads a write to the record itself invalidates", () => {
  it("reaches the deal status card, which is written from the deal's own fields", () => {
    // The card names what the stage and the value MEAN. Advancing a stage
    // without this leaves it describing a stage the deal has left, stated with
    // the confidence of a current reading.
    expect(dealRecordKeys("d1")).toEqual([
      ["deal", "d1"],
      ["deal-status", "d1"],
    ]);
  });

  it("reaches the same card from the generic edit form, by record kind", () => {
    expect(derivedRecordKeys("deal", "d1")).toEqual([["deal-status", "d1"]]);
  });

  it("names nothing for a record kind with no derived read", () => {
    expect(derivedRecordKeys("person", "p1")).toEqual([]);
  });
});

// A message is filed against several records, and the client is never told
// which: the audience answer names activities, not the records they hang off.
// So the reads that could be drawing a changed message are found by SHAPE, and
// the shape has to be wide enough to catch every timeline and narrow enough not
// to re-read the whole app.
describe("which reads could be showing a message", () => {
  const matches = (key: unknown[]) => showsAMessage({ queryKey: key });

  it("matches every record's timeline, not only the one on screen", () => {
    expect(matches(["activities", "deal", "d1"])).toBe(true);
    expect(matches(["activities", "person", "p1"])).toBe(true);
    // Narrowed and paged reads hang further keys off the same prefix, and they
    // draw the same messages.
    expect(matches(["activities", "person", "p1", { kind: "email" }])).toBe(
      true,
    );
  });

  it("matches the composite reads that carry a timeline's first page", () => {
    expect(matches(["organization360", "o1"])).toBe(true);
    expect(matches(["person360", "p1"])).toBe(true);
    expect(matches(["project", "j1", "360"])).toBe(true);
  });

  it("matches the canonical email read, which a drawer's thread page also carries", () => {
    // The anchor message may not be the one that changed: a presentation
    // embeds a page of thread-member summaries, and one of those can be.
    expect(matches(["email-presentation", "a1"])).toBe(true);
  });

  it("matches what a timeline is derived into", () => {
    expect(matches(["deal-status", "d1"])).toBe(true);
  });

  it("leaves reads that carry no message alone", () => {
    // The project RECORD, which is the same head as its 360 payload and two
    // segments long. Matching it would re-read every project page in the cache
    // for a change that cannot have touched one.
    expect(matches(["project", "j1"])).toBe(false);
    expect(matches(["deal", "d1"])).toBe(false);
    expect(matches(["deals"])).toBe(false);
    expect(matches(["tasks"])).toBe(false);
    expect(matches(["held-threads"])).toBe(false);
  });
});
