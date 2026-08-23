import { describe, expect, it } from "vitest";
import {
  dealRecordKeys,
  dealWinKeys,
  derivedRecordKeys,
  entityTimelineKeys,
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
