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

// Both the record-level and field-level history rows split actor_type/actor_id,
// so the two are read for different things: the TYPE says what acted, and the ID
// supplies a name only when it holds one. Structurally typed off just those two
// fields so it serves AuditHistoryEntry and FieldHistoryEntry alike.
export function provenanceOfEntry(
  entry: Pick<AuditHistoryEntry, "actor_type" | "actor_id">,
  viewerUserId?: string,
): Provenance {
  // A Deal Room participant: a contact, and one from outside the company,
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
// Columns the write path stamps rather than a contact choosing. They sit in the
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
