import { describe, expect, it } from "vitest";
import type { components } from "../api/schema";
import { distinctFields, groupByField, mergeChronology } from "./history.logic";

type FieldHistoryEntry = components["schemas"]["FieldHistoryEntry"];

const e = (
  field: string,
  at: string,
  actor: FieldHistoryEntry["actor_type"] = "human",
) =>
  ({
    id: at,
    entity_type: "deal",
    entity_id: "d1",
    field,
    old_value: null,
    new_value: "x",
    changed_at: at,
    actor_type: actor,
    actor_id: "u1",
  }) as const;

describe("groupByField", () => {
  it("groups entries by field, newest-first within a group, first-seen field order", () => {
    const groups = groupByField([
      e("name", "2026-01-01"),
      e("amount", "2026-01-02"),
      e("name", "2026-03-01"),
    ]);
    expect(groups.map((g) => g.field)).toEqual(["name", "amount"]);
    expect(groups[0].changes.map((c) => c.changed_at)).toEqual([
      "2026-03-01",
      "2026-01-01",
    ]);
  });
  it("returns [] for no entries", () => {
    expect(groupByField([])).toEqual([]);
  });
});

describe("distinctFields", () => {
  it("lists fields in first-seen order without dupes", () => {
    expect(
      distinctFields([e("name", "1"), e("amount", "2"), e("name", "3")]),
    ).toEqual(["name", "amount"]);
  });
});

describe("mergeChronology", () => {
  const row = (at: string) => ({ at });
  const at = (r: { at: string }) => r.at;

  it("interleaves both feeds newest-first when both are fully loaded", () => {
    const merged = mergeChronology(
      [
        { rows: [row("2026-03-01"), row("2026-01-01")], hasMore: false },
        { rows: [row("2026-02-01")], hasMore: false },
      ],
      at,
    );
    expect(merged.rows.map(at)).toEqual([
      "2026-03-01",
      "2026-02-01",
      "2026-01-01",
    ]);
    expect(merged.truncated).toBe(false);
  });

  it("cuts the OTHER feed's rows at a truncated feed's oldest, so the merge has no invisible gaps", () => {
    // The activity feed stops at 2026-02-01 and has older rows unfetched.
    // A change from January must NOT render under it: between them sit
    // activities nobody has loaded.
    const merged = mergeChronology(
      [
        { rows: [row("2026-03-01"), row("2026-02-01")], hasMore: true },
        { rows: [row("2026-02-15"), row("2026-01-01")], hasMore: false },
      ],
      at,
    );
    // The activity feed's own 2026-02-01 stays — it is a row we are holding,
    // and nothing unloaded sits above it. January's change goes: activities
    // nobody has fetched sit between it and the row above.
    expect(merged.rows.map(at)).toEqual([
      "2026-03-01",
      "2026-02-15",
      "2026-02-01",
    ]);
    expect(merged.truncated).toBe(true);
  });

  it("drops ANOTHER feed's row at the boundary, because the feeds page on (time, id)", () => {
    // The change shares a second with the activity feed's oldest loaded row,
    // and that feed pages on (time, id) — so an activity at the same instant
    // may sit unloaded above the change. The change goes; the activity that
    // DEFINES the boundary stays, because it is a row we are holding.
    const merged = mergeChronology(
      [
        { rows: [row("2026-03-01"), row("2026-02-01")], hasMore: true },
        { rows: [row("2026-02-01")], hasMore: false },
      ],
      at,
    );
    expect(merged.rows.map(at)).toEqual(["2026-03-01", "2026-02-01"]);
    expect(merged.truncated).toBe(true);
  });

  it("keeps every row a lone truncated feed has loaded", () => {
    // Its own paging hides rows OLDER than its oldest, never rows between two
    // it returned — so cutting it at its own floor dropped the only row it
    // had and rendered an empty chronology over a list it was holding.
    const merged = mergeChronology(
      [
        { rows: [row("2026-03-01")], hasMore: true },
        { rows: [], hasMore: false },
      ],
      at,
    );
    expect(merged.rows.map(at)).toEqual(["2026-03-01"]);
    expect(merged.truncated).toBe(true);
  });

  it("cuts each feed at the other's floor when both have more", () => {
    // Neither feed has an unloaded row newer than the other's oldest, so both
    // boundary rows are provably placed and both stay. What each feed hides
    // is older than its own floor, and that is what `truncated` says.
    const merged = mergeChronology(
      [
        { rows: [row("2026-03-01"), row("2026-02-01")], hasMore: true },
        { rows: [row("2026-03-05"), row("2026-02-20")], hasMore: true },
      ],
      at,
    );
    expect(merged.rows.map(at)).toEqual([
      "2026-03-05",
      "2026-03-01",
      "2026-02-20",
    ]);
    expect(merged.truncated).toBe(true);
  });

  it("shows nothing when a feed has more but has loaded nothing yet", () => {
    // Its newest row is unknown, so no part of the merge is provably complete
    // — an empty in-flight state, never a list that looks whole.
    const merged = mergeChronology(
      [
        { rows: [row("2026-03-01")], hasMore: false },
        { rows: [], hasMore: true },
      ],
      at,
    );
    expect(merged.rows).toEqual([]);
    expect(merged.truncated).toBe(true);
  });

  it("returns an empty, untruncated merge when both feeds are empty and complete", () => {
    const merged = mergeChronology(
      [
        { rows: [], hasMore: false },
        { rows: [], hasMore: false },
      ],
      at,
    );
    expect(merged.rows).toEqual([]);
    expect(merged.truncated).toBe(false);
  });
});

// Two feeds are written by two stores. A merge that compares the STRINGS puts
// "…:00.500Z" before "…:00Z" and a +02:00 offset in the wrong hour entirely —
// an order the reader has no way to know is wrong.
it("orders by the instant, not by how the timestamp is spelled", () => {
  const rows = [
    { id: "fractional", at: "2026-07-19T09:00:00.500Z" },
    { id: "offset", at: "2026-07-19T11:30:00+02:00" },
    { id: "plain", at: "2026-07-19T09:00:00Z" },
  ];
  const merged = mergeChronology([{ rows, hasMore: false }], (row) => row.at);
  expect(merged.rows.map((row) => row.id)).toEqual([
    "offset",
    "fractional",
    "plain",
  ]);
});
