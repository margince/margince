import type { components } from "../api/schema";
import type { Provenance } from "../design-system/trust";
import { stable } from "../format/collate";
import { provenanceOf } from "./common";

type AuditHistoryEntry = components["schemas"]["AuditHistoryEntry"];
type FieldHistoryEntry = components["schemas"]["FieldHistoryEntry"];

export type ActorFacet = "all" | "human" | "agent";
export type FieldGroup = { field: string; changes: FieldHistoryEntry[] };

// Group field-history rows by field for the mockup's per-field sections.
// First-seen field order is preserved; within a group, newest change first.
export function groupByField(entries: FieldHistoryEntry[]): FieldGroup[] {
  const byField = new Map<string, FieldHistoryEntry[]>();
  for (const entry of entries) {
    const bucket = byField.get(entry.field);
    if (bucket) {
      bucket.push(entry);
    } else {
      byField.set(entry.field, [entry]);
    }
  }
  return [...byField.entries()].map(([field, changes]) => ({
    field,
    changes: [...changes].sort((a, b) => stable(b.changed_at, a.changed_at)),
  }));
}

export function distinctFields(entries: FieldHistoryEntry[]): string[] {
  const seen: string[] = [];
  for (const entry of entries) {
    if (!seen.includes(entry.field)) seen.push(entry.field);
  }
  return seen;
}

// One feed going into a merged chronology: the rows loaded so far, and
// whether older ones exist that have not been fetched.
export type Feed<Row> = { rows: Row[]; hasMore: boolean };

/**
 * Interleave two independently-paged feeds into one chronology, cut at the
 * point where it stops being complete.
 *
 * Two feeds paged separately cannot simply be concatenated and sorted: below
 * the oldest row of a feed that still has more, the merge is missing rows it
 * does not know about, and the result reads as a complete history with gaps in
 * it — the one failure a reader cannot see. So the merged list ends at the
 * newest such boundary, and the caller says what was cut.
 *
 * A feed that is fully loaded imposes no boundary: nothing older is missing
 * from it. When neither feed has more, the merge is the whole history.
 *
 * The cut is STRICT. Both feeds page on (timestamp, id), not on timestamp
 * alone, so a feed whose oldest loaded row sits at T may still have unfetched
 * rows at exactly T. Keeping the boundary row would put a same-second gap
 * inside a stretch the merge presents as whole — which is the one failure this
 * function exists to prevent, reappearing one row further down.
 */
export function mergeChronology<Row>(
  feeds: readonly Feed<Row>[],
  at: (row: Row) => string,
): { rows: Row[]; truncated: boolean } {
  // Instants, never the strings that spell them. Two feeds are written by two
  // stores, and "2026-07-19T09:00:00Z" sorts against "2026-07-19T09:00:00.5Z"
  // and "2026-07-19T11:00:00+02:00" by character, which orders neither the way
  // the clock does. The whole point of this function is an order a reader can
  // trust, so it compares numbers.
  const instant = (row: Row) => Date.parse(at(row));
  // Both folds carry a seed, so neither depends on the filter above still
  // being there to keep them off an empty array: an unseeded reduce throws on
  // one, and a guard two lines away is not where the next reader looks.
  const boundaries = feeds
    .filter((feed) => feed.hasMore && feed.rows.length > 0)
    .map((feed) =>
      feed.rows.reduce(
        (oldest, row) => Math.min(oldest, instant(row)),
        Number.POSITIVE_INFINITY,
      ),
    );
  // A feed that has more but loaded NOTHING bounds the merge at the top: its
  // very newest row is unknown, so no part of the merge is provably complete.
  const blind = feeds.some((feed) => feed.hasMore && feed.rows.length === 0);
  const floor =
    boundaries.length > 0
      ? boundaries.reduce(
          (newest, boundary) => Math.max(newest, boundary),
          Number.NEGATIVE_INFINITY,
        )
      : undefined;

  const all = feeds
    .flatMap((feed) => feed.rows)
    .sort((a, b) => instant(b) - instant(a));
  if (blind) {
    return { rows: [], truncated: true };
  }
  if (floor === undefined) {
    return { rows: all, truncated: false };
  }
  // A row is cut by every OTHER truncated feed's floor, never by its own.
  //
  // The gap this guards against is a row from feed B landing between two rows
  // feed A returned. A feed's own paging cannot do that: it hides rows OLDER
  // than its oldest, and its oldest is a row we have. Cutting a feed at its
  // own floor dropped that row for nothing — and where a feed was the only one
  // with rows at all, it dropped every row it had and the chronology rendered
  // empty over a list it was holding.
  //
  // Ties across feeds still go: a row at another feed's floor may have an
  // unloaded sibling above it, which is the (time, id) page break the floor
  // exists to respect.
  const rows = all.filter((row) =>
    feeds.every(
      (feed) =>
        feed.rows.includes(row) ||
        !feed.hasMore ||
        feed.rows.length === 0 ||
        instant(row) >
          feed.rows.reduce(
            (oldest, other) => Math.min(oldest, instant(other)),
            Number.POSITIVE_INFINITY,
          ),
    ),
  );
  // A floor exists only because some feed reported more, so the merged view is
  // short of the account's history by construction — whether or not this cut
  // dropped any loaded row.
  return { rows, truncated: true };
}

// Both the record-level and field-level history rows split actor_type/actor_id,
// so the two are read for different things: the TYPE says what acted, and the ID
// supplies a name only when it holds one. Structurally typed off just those two
// fields so it serves AuditHistoryEntry and FieldHistoryEntry alike.
export function provenanceOfEntry(
  entry: Pick<AuditHistoryEntry, "actor_type" | "actor_id">,
  viewerUserId?: string,
): Provenance {
  // A Deal Room participant: a person, and one from outside the organization,
  // which is its own arm rather than the machine treatment or the colleague
  // one. `actor_id` is `buyer:<participant uuid>` — an identifier no reader can
  // look up and no lookup here resolves — so the tag says the kind and stops.
  if (entry.actor_type === "buyer") {
    return { kind: "buyer" };
  }
  if (entry.actor_type !== "human") {
    return machineProvenance(entry.actor_type, entry.actor_id);
  }
  // The spine stores the principal id, which for a human is `human:<uuid>`
  // (principal.Principal.ID), while the session reports the bare user id.
  // Compared as-is the two never match, and every row a reader wrote came
  // back attributed to a teammate.
  const userId = entry.actor_id.startsWith("human:")
    ? entry.actor_id.slice("human:".length)
    : entry.actor_id;
  return {
    kind: "human",
    self: Boolean(viewerUserId) && userId === viewerUserId,
    userId,
  };
}

// What a change nobody typed reads as.
//
// The three non-human kinds are three different facts about a record and were
// collapsed into one: every non-human actor read as "Automated by <actor_id>",
// so a scheduled sweep and a mailbox connector both named an agent that had not
// acted, and a passport uuid went in front of a reader who cannot look one up.
//
// actor_type is the authority on WHICH of them acted — a closed enum on this
// projection — and the id is read only for the NAME inside it, by the one
// function that spells that principal grammar out: `provenanceOf` resolves both
// connector grammars and drops an id that is a bare uuid rather than printing
// it. The id is expected to carry its own kind, and where it does not the kind
// is put back from actor_type, so a row stamped with a bare id reads the same as
// one stamped with the whole principal.
function machineProvenance(
  actorType: Exclude<AuditHistoryEntry["actor_type"], "human" | "buyer">,
  actorId: string,
): Provenance {
  const prefix = `${actorType}:`;
  // An id that is JUST the kind names nothing, and prefixing it would promote
  // the kind to a name: a bare `system` would read "System task system".
  const carriesKind = actorId === actorType || actorId.startsWith(prefix);
  return provenanceOf(carriesKind ? actorId : prefix + actorId);
}

// One changed field, as the record-level projection already holds it.
export type EntryFieldChange = {
  field: string;
  oldValue: string | null;
  newValue: string | null;
};

// The fields ONE audit row touched.
//
// The record-level entry carries both images, so the detail beside a change
// costs no second read — which is what lets the plain-language list show what
// actually moved instead of sending the reader to another sub-tab for it.
//
// A key the image does not hold reads as absent rather than as empty text: the
// diff draws its own wording for a field that did not exist before or does not
// now, and "" is a value somebody stored.
// Columns the write path stamps rather than a person choosing. They sit in the
// audit image because the row really did change, and showing them as field
// changes tells a reader somebody edited "updated at" — which nobody did, and
// which they cannot act on. The restore drops them for the same reason.
const DERIVED_COLUMNS: ReadonlySet<string> = new Set([
  "updated_at",
  "created_at",
  "id",
  "version",
]);

export function entryFieldChanges(
  entry: Pick<AuditHistoryEntry, "before" | "after" | "edge">,
): EntryFieldChange[] {
  // An entry that changed a LINK has no fields of THIS record. Its images are
  // the link's own columns — role, started_at, the primary flag — and drawing
  // them here would tell a reader somebody edited fields the record does not
  // have, under labels that do not exist for them.
  //
  // The rule lives here rather than at each call site because every reader of a
  // change list asks the same question: the row, the collapsed pair's face, and
  // the net a pair reports all go through this function, and a call site that
  // forgot would show the link's columns on one surface and not another.
  if (entry.edge) {
    return [];
  }
  const before = entry.before ?? {};
  const after = entry.after ?? {};
  const fields: string[] = [];
  for (const field of [...Object.keys(after), ...Object.keys(before)]) {
    if (!fields.includes(field)) {
      fields.push(field);
    }
  }
  return fields
    .filter((field) => !DERIVED_COLUMNS.has(field))
    .map((field) => ({
      field,
      oldValue: imageValue(before[field]),
      newValue: imageValue(after[field]),
    }))
    .filter((change) => change.oldValue !== change.newValue);
}

// A jsonb value as the diff renders it. Anything that is not already text is
// spelled the way the wire spells it, because a stored document is still what
// the row holds and "[object Object]" tells a reader nothing they can check.
function imageValue(value: unknown): string | null {
  if (value === null || value === undefined) {
    return null;
  }
  return typeof value === "string" ? value : JSON.stringify(value);
}
