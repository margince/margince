import type { QueryKey } from "@tanstack/react-query";
import type { EntityKind } from "../app/entity";

// Which cached reads render a record's timeline. Writing an activity has to
// invalidate ALL of them, and they are not the same set for every record kind:
// every record page reads older pages and narrowed reads through its own
// ["activities", kind, id] query, while the company, contact and project
// pages draw the FIRST page out of the composite 360 payload and fetch
// nothing under that key until the reader asks for more. A mutation that
// names only the first key writes successfully and shows nothing, which reads
// exactly like a broken endpoint.
//
// Derived here once rather than spelled at each mutation site, so a new screen
// that renders a timeline from a different read is fixed in one place.

export function entityTimelineKeys(
  entityType: EntityKind,
  entityId: string,
): QueryKey[] {
  const keys: QueryKey[] = [["activities", entityType, entityId]];
  const seed = TIMELINE_SEED_KEYS[entityType]?.(entityId);
  if (seed) {
    keys.push(seed);
  }
  const derived = DERIVED_FROM_TIMELINE[entityType]?.(entityId);
  if (derived) {
    keys.push(derived);
  }
  return keys;
}

const ORGANIZATION_360_KEY = (id: string): QueryKey => ["organization360", id];

// The composite reads that carry a timeline's first page, by record kind —
// spelled the way each page's own query spells its key.
const TIMELINE_SEED_KEYS: Partial<
  Record<EntityKind, (entityId: string) => QueryKey>
> = {
  organization: (id) => ORGANIZATION_360_KEY(id),
  person: (id) => ["person360", id],
  project: (id) => ["project", id, "360"],
};

// Reads WRITTEN FROM a record rather than showing it. The deal status card
// says what the deal's own fields and its activity MEAN, so anything that
// moves either leaves it describing a deal that has since changed — and unlike
// a timeline visibly missing its newest row, a stale sentence reads as a
// current judgement.
//
// It is derived from BOTH halves, which is why it has a helper of its own
// below as well as a place here: logging an activity reaches it through the
// timeline keys, and advancing a stage or editing the value reaches it through
// dealRecordKeys. A card naming a stage the deal has left is the failure this
// prevents.
const DERIVED_FROM_TIMELINE: Partial<
  Record<EntityKind, (entityId: string) => QueryKey>
> = {
  deal: (id) => DEAL_STATUS_KEY(id),
};

const DEAL_STATUS_KEY = (id: string): QueryKey => ["deal-status", id];

// Which cached reads a write to the DEAL RECORD itself invalidates — a stage
// advance, an amount, a close date. Spelled once here so a new writer picks up
// the derived reads by using the helper rather than by remembering them.
export function dealRecordKeys(dealId: string): QueryKey[] {
  return [["deal", dealId], ...derivedRecordKeys("deal", dealId)];
}

// The reads written FROM a record, by the key its own page reads it under.
// The generic edit form consults this, so an edit to any record kind reaches
// whatever is derived from it without every form naming them.
export function derivedRecordKeys(
  recordKey: string,
  recordId: string,
): QueryKey[] {
  const derived = DERIVED_FROM_RECORD[recordKey]?.(recordId);
  return derived ? [derived] : [];
}

// A deal's COVERAGE is written from its stakeholder edges: the rail's seats,
// the committee map and the risk chips all read GET /deals/{id}/coverage, and
// every one of them is stale the moment a seat is added, re-roled or removed.
// Keyed per deal by the reader (dealCoverageKey); named here as the prefix,
// because a writer usually knows only that SOME deal's edges moved — a
// stakeholder is seated from the person's page as readily as from the deal's.
export const DEAL_COVERAGE_KEY: QueryKey = ["deal-coverage"];

const DERIVED_FROM_RECORD: Record<string, (id: string) => QueryKey> = {
  deal: (id) => DEAL_STATUS_KEY(id),
  // Keyed on the relationship's own id nowhere: what goes stale is the deal the
  // edge names, and the edit form knows only the edge. The prefix covers it.
  relationship: () => DEAL_COVERAGE_KEY,
};

// A task is also a row in the standing work queue, which is keyed per workspace
// rather than per record — so completing or logging one has to reach further
// than the record's own timeline.
export const TASK_QUEUE_KEY: QueryKey = ["tasks"];

export function taskWriteKeys(
  entityType: EntityKind,
  entityId: string,
): QueryKey[] {
  return [...entityTimelineKeys(entityType, entityId), TASK_QUEUE_KEY];
}

// Which cached reads carry a deal's project and its phase. A won deal moves
// its project into delivery in the same server write, so besides the project
// page and list, the company page — it embeds the account's projects with
// their phase — is stale the moment the advance returns. A deal names no
// contact of its own (the Deal schema carries organization_id and project_id
// only), so there is no person page to reach from here. Derived beside the
// timeline keys so the 360 keys keep one spelling.
export function dealWinKeys(
  deal:
    | { project_id?: string | null; organization_id?: string | null }
    | undefined,
): QueryKey[] {
  const keys: QueryKey[] = [["projects"]];
  if (deal?.project_id) {
    keys.push(["project", deal.project_id]);
  }
  if (deal?.organization_id) {
    keys.push(ORGANIZATION_360_KEY(deal.organization_id));
  }
  return keys;
}

// ── Every timeline, not one record's ────────────────────────────────────────

// A change to who may read a MESSAGE reaches every record the message is filed
// against, and the client is not told which those are: the server names the
// activities a decision touched, and `Activity.links` is not on that answer —
// putting a record list on a privacy response for the sake of a cache is the
// wrong trade. So the reads that could be showing the old audience are found by
// their SHAPE instead of by their record.
//
// It is a wide invalidation and that is affordable here, not everywhere: an
// audience change is a deliberate, rare human act, and react-query refetches
// only the queries that are actually mounted — the rest are marked stale and
// re-read when something asks for them.
//
// The shapes are derived from the tables above rather than listed again, so a
// record kind that grows a timeline read joins this by being added there. A
// second list would go stale exactly the way the record-scoped invalidation it
// replaces did, and just as quietly.
const SHAPE_ID = "\u0000id";

// Every key shape that draws a message: the per-record timelines, the composite
// 360 payloads that carry their first page, the reads derived from a timeline,
// and the canonical email read itself — a drawer's thread-member page can carry
// a message the reader did not open and did not change.
function messageBearingShapes(): unknown[][] {
  const shapes: unknown[][] = [["activities"], ["email-presentation"]];
  for (const seed of Object.values(TIMELINE_SEED_KEYS)) {
    shapes.push(seed(SHAPE_ID) as unknown[]);
  }
  for (const derived of Object.values(DERIVED_FROM_TIMELINE)) {
    shapes.push(derived(SHAPE_ID) as unknown[]);
  }
  return shapes;
}

/**
 * Whether a cached read could be showing a message whose audience just changed.
 *
 * Matched as a PREFIX with the id position wild, so `["activities", …]` covers
 * every record's timeline while `["project", id, "360"]` covers the project
 * 360 payload without also matching `["project", id]` — the project record
 * itself, which carries no message and does not need re-reading.
 */
export function showsAMessage(query: { queryKey: QueryKey }): boolean {
  const key = query.queryKey as unknown[];
  return messageBearingShapes().some(
    (shape) =>
      key.length >= shape.length &&
      shape.every((segment, at) => segment === SHAPE_ID || segment === key[at]),
  );
}

// The canonical email read's key belongs to the component that reads under it,
// so this is a re-export rather than a second spelling. Two copies of a cache
// key do not fail loudly: they fail as a drawer that quietly stops refreshing
// after somebody changes who may read the message.
export { emailDetailKey as emailPresentationKey } from "../design-system/emaildetail";
