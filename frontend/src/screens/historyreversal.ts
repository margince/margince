import type { components } from "../api/schema";
import { type EntryFieldChange, entryFieldChanges } from "./history.logic";

type AuditHistoryEntry = components["schemas"]["AuditHistoryEntry"];

// A reversal and the change it reversed, read as ONE thing.
//
// A restore is an ordinary update carrying the `restore` verb, so on its own it
// renders as a fresh change: a fourth entry in a chronology of three, saying
// "restored the record" without naming what it restored, above a button that
// says "put back" and means redo. Pairing the two rows is what makes the
// chronology countable again.
//
// The pairing lives apart from the rendering because two of its rules are the
// ones most likely to be wrong, and both are decidable without a screen: what a
// pair's NET movement is, and when a pair may not form at all.

// PairRow is a reversal shown together with what it reversed.
export type PairRow = {
  kind: "pair";
  reversal: AuditHistoryEntry;
  reversed: AuditHistoryEntry;
  // The pair's position in the chronology: the REVERSAL's time, because it is
  // the newer of the two and nothing may appear to have happened earlier than
  // it did.
  atIso: string;
  // Whether every field the reversed change moved has gone back. False means a
  // residual survives, and the row must show it rather than claiming the pair
  // came to nothing — a row saying "unchanged" over data that moved is the one
  // outcome worse than showing two rows.
  whollyUndone: boolean;
  // Whether one person undid their own change. The copy differs: naming the same
  // person twice reads as two parties who happen to share a name.
  sameActor: boolean;
};

export type HistoryRow =
  | { kind: "single"; entry: AuditHistoryEntry; atIso: string }
  // A reversal whose partner is not in the loaded entries. Not a degenerate
  // pair: it says only what it can prove, which is that it undid something
  // earlier.
  | { kind: "unpairedReversal"; entry: AuditHistoryEntry; atIso: string }
  | PairRow;

// netChanges is what a pair left behind, per field.
//
// COMPUTED, never inferred from the fact that a reversal exists. A restore can
// put back a subset — a multi-field change where one field's clear is refused
// lands a partial — and the difference between "this came to nothing" and "this
// field still moved" is the whole reason a reader trusts the row.
//
// Computed over the images this VIEWER received, which are field-masked. Two
// readers can therefore get different answers for one pair, and each is told the
// truth about what they can see.
export function netChanges(row: PairRow): EntryFieldChange[] {
  const undone = new Map(
    entryFieldChanges(row.reversal).map((change) => [change.field, change]),
  );
  return entryFieldChanges(row.reversed).filter((change) => {
    const back = undone.get(change.field);
    // The field went back exactly when the reversal landed the value the
    // change had moved away from.
    return !back || back.newValue !== change.oldValue;
  });
}

// historyRows groups a page of entries, newest first, into what the screen draws.
//
// Two entries pair only when they are ADJACENT in the loaded list. A pair that
// could form across a gap would relocate a row as more pages load — a change
// from March leaving March and reappearing inside an August disclosure while
// somebody is reading it.
export function historyRows(
  entries: readonly AuditHistoryEntry[],
): HistoryRow[] {
  // A reversal that has itself been reversed is not live, so it does not
  // collapse anything: undoing an undo REOPENS the entry underneath, which is
  // what keeps the trail navigable in both directions rather than a ratchet.
  const reversedIds = new Set(
    entries
      .map((entry) => entry.undid_audit_log_id)
      .filter((id): id is string => Boolean(id)),
  );

  const rows: HistoryRow[] = [];
  for (let i = 0; i < entries.length; i += 1) {
    const entry = entries[i];
    const undid = entry.undid_audit_log_id;
    if (!undid) {
      rows.push({ kind: "single", entry, atIso: entry.occurred_at });
      continue;
    }
    if (reversedIds.has(entry.id)) {
      // Superseded by a later reversal: it is the reversed half of the pair
      // above it, not the live end of the chain.
      rows.push({ kind: "single", entry, atIso: entry.occurred_at });
      continue;
    }
    const next = entries[i + 1];
    if (!next || next.id !== undid) {
      rows.push({ kind: "unpairedReversal", entry, atIso: entry.occurred_at });
      continue;
    }
    const pair: PairRow = {
      kind: "pair",
      reversal: entry,
      reversed: next,
      atIso: entry.occurred_at,
      whollyUndone: false,
      sameActor: entry.actor_id === next.actor_id,
    };
    pair.whollyUndone = netChanges(pair).length === 0;
    rows.push(pair);
    i += 1;
  }
  return rows;
}
