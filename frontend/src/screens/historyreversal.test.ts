import { describe, expect, it } from "vitest";
import type { components } from "../api/schema";
import { historyRows, netChanges } from "./historyreversal";

type AuditHistoryEntry = components["schemas"]["AuditHistoryEntry"];

// A fixture built as a real AuditHistoryEntry rather than asserted into one:
// an `as` here would let a contract field change under these tests without a
// compiler error, and what they check is how a row's own fields pair.
function entry(
  over: Partial<AuditHistoryEntry> & { id: string },
): AuditHistoryEntry {
  return {
    actor_type: "human",
    actor_id: "human:u-1",
    actor_name: "Sam Okafor",
    action: "update",
    occurred_at: "2026-08-27T14:00:00Z",
    summary: "Sam Okafor updated the record",
    before: null,
    after: null,
    ...over,
  };
}

// A title moving B -> C, and the restore that put it back C -> B.
const changeC = entry({
  id: "c",
  occurred_at: "2026-08-27T14:00:00Z",
  before: { title: "Head of Ops" },
  after: { title: "Head of Platform" },
});
const undoOfC = entry({
  id: "r1",
  action: "restore",
  actor_id: "human:u-2",
  actor_name: "Tin Nguyen",
  occurred_at: "2026-08-27T14:02:00Z",
  summary: "Tin Nguyen restored the record",
  undid_audit_log_id: "c",
  before: { title: "Head of Platform" },
  after: { title: "Head of Ops" },
});

describe("historyRows", () => {
  // R1: the pair sits where its NEWEST member sits, so nothing appears to have
  // happened earlier than it did.
  it("pairs a reversal with what it reversed, at the reversal's time", () => {
    const rows = historyRows([undoOfC, changeC]);
    expect(rows).toHaveLength(1);
    const [row] = rows;
    expect(row.kind).toBe("pair");
    if (row.kind !== "pair") return;
    expect(row.reversal.id).toBe("r1");
    expect(row.reversed.id).toBe("c");
    expect(row.atIso).toBe("2026-08-27T14:02:00Z");
  });

  // R2: a pair whose every field went back has nothing left to show.
  it("reports an empty net when the whole change went back", () => {
    const rows = historyRows([undoOfC, changeC]);
    const [row] = rows;
    if (row.kind !== "pair") throw new Error("expected a pair");
    expect(netChanges(row)).toEqual([]);
    expect(row.whollyUndone).toBe(true);
  });

  // R2, the case that must never regress: a reversal that put back only part of
  // a change must not read as "unchanged" while a field still holds its new
  // value.
  it("reports the residual when only part of the change went back", () => {
    const twoFields = entry({
      id: "c2",
      before: { forecast_category: "Commit", renewal_owner: null },
      after: { forecast_category: "Upside", renewal_owner: "Priya Raman" },
    });
    const partial = entry({
      id: "r2",
      action: "restore",
      occurred_at: "2026-08-27T16:41:00Z",
      undid_audit_log_id: "c2",
      // renewal_owner could not be cleared, so only the category went back.
      before: { forecast_category: "Upside" },
      after: { forecast_category: "Commit" },
    });
    const rows = historyRows([partial, twoFields]);
    const [row] = rows;
    if (row.kind !== "pair") throw new Error("expected a pair");
    expect(row.whollyUndone).toBe(false);
    expect(netChanges(row)).toEqual([
      { field: "renewal_owner", oldValue: null, newValue: "Priya Raman" },
    ]);
  });

  // R4: the common case on page one of a long history.
  it("leaves a reversal standalone when its partner is not loaded", () => {
    const rows = historyRows([undoOfC]);
    expect(rows).toHaveLength(1);
    expect(rows[0].kind).toBe("unpairedReversal");
  });

  // R5: undo, redo, undo again is a chain, and a row can belong to two pairs.
  // Only the LIVE reversal collapses; a reversal that has itself been reversed
  // renders standalone, which is the same liveness already_undone is decided by.
  it("collapses only the live reversal in a chain", () => {
    const undoOfUndo = entry({
      id: "r2",
      action: "restore",
      occurred_at: "2026-08-27T15:10:00Z",
      undid_audit_log_id: "r1",
      before: { title: "Head of Ops" },
      after: { title: "Head of Platform" },
    });
    const rows = historyRows([undoOfUndo, undoOfC, changeC]);
    // r2 pairs with r1. c is reopened and stands on its own.
    expect(rows.map((row) => row.kind)).toEqual(["pair", "single"]);
    const [pair, single] = rows;
    if (pair.kind !== "pair") throw new Error("expected a pair");
    expect(pair.reversal.id).toBe("r2");
    expect(pair.reversed.id).toBe("r1");
    expect(single.kind === "single" && single.entry.id).toBe("c");
  });

  // R5 again, and this is the arm the adjacent-chain case above cannot reach:
  // when the OUTER reversal is separated from its partner, the middle reversal
  // is visited as a top-level row while still adjacent to what it reversed.
  // It must not collapse that change, because it is no longer live — undoing an
  // undo reopens the entry underneath, and a pair here would hide a change the
  // record has gone back to holding.
  it("does not collapse a reversal that has itself been reversed", () => {
    const undoOfUndo = entry({
      id: "r2",
      action: "restore",
      occurred_at: "2026-08-27T15:10:00Z",
      undid_audit_log_id: "r1",
    });
    const between = entry({ id: "x", occurred_at: "2026-08-27T15:05:00Z" });
    const rows = historyRows([undoOfUndo, between, undoOfC, changeC]);
    expect(rows.map((row) => row.kind)).toEqual([
      "unpairedReversal",
      "single",
      "single",
      "single",
    ]);
  });

  // R6: a pair must not form across a gap, or a row a reader was looking at
  // leaves the position it was occupying once another page loads.
  it("does not pair across other entries", () => {
    const between = entry({ id: "x", occurred_at: "2026-08-27T14:01:00Z" });
    const rows = historyRows([undoOfC, between, changeC]);
    expect(rows.map((row) => row.kind)).toEqual([
      "unpairedReversal",
      "single",
      "single",
    ]);
  });

  // An ordinary row carries no link and is untouched.
  it("leaves an ordinary change alone", () => {
    const rows = historyRows([changeC]);
    expect(rows.map((row) => row.kind)).toEqual(["single"]);
  });

  // R8: somebody correcting their own change is the common case, and the pair
  // must be able to say so rather than assuming two parties.
  it("reports whether the actor and the undoer are the same person", () => {
    const same = { ...undoOfC, actor_id: changeC.actor_id };
    const rows = historyRows([same, changeC]);
    const [row] = rows;
    if (row.kind !== "pair") throw new Error("expected a pair");
    expect(row.sameActor).toBe(true);

    const other = historyRows([undoOfC, changeC])[0];
    if (other.kind !== "pair") throw new Error("expected a pair");
    expect(other.sameActor).toBe(false);
  });
});
