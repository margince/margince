// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  type InvalidateQueryFilters,
  partialMatchKey,
  type QueryClient,
  type QueryKey,
} from "@tanstack/react-query";
import {
  dealRecordKeys,
  dealStageHistoryKey,
  derivedRecordKeys,
  recordListsKey,
} from "./activitykeys";
import type { EntityKind } from "./entity";
import { leadScoreKey, leadWriteKeys } from "./leadkeys";

// Which cached reads a write to ONE record makes stale, by record kind.
//
// It exists because no record kind is read under a single key, and prefix
// invalidation does not walk sideways: a contact's own fields are served to
// the detail page as `["contact", id]`, to the page shell as `["contact360",
// id]` and to the brief as `["contactBrief", id]`, three siblings under no
// common prefix. A writer naming one of them leaves the other two painting
// the state the reader just changed — which is exactly what every restore
// callback did, each naming a DIFFERENT one of the three.
//
// Spelled here once rather than at each site, the way `activitykeys.ts`
// already does it for a record's timeline and `leadkeys.ts` for a lead's.
// The map is total over EntityKind so a new record kind cannot be added
// without deciding what its write invalidates.
export function recordWriteKeys(kind: EntityKind, id: string): QueryKey[] {
  return RECORD_WRITE_KEYS[kind](id);
}

// `anchor` is the record's `is_anchor`: the installation's own company.
// `stageMove` is a deal write that may have moved its stage, so its stage
// moves are read again.
type RecordWriteScope = Readonly<{ anchor?: boolean; stageMove?: boolean }>;

// The anchor is also read as the settings company profile and as /me's brand.
// Not a ["company"] prefix: it would also stale every company record read.
const ANCHOR_WRITE_FILTERS: readonly InvalidateQueryFilters[] = [
  { queryKey: ["company"], exact: true },
  { queryKey: ["me"] },
];

const RECORD_WRITE_KEYS: Record<EntityKind, (id: string) => QueryKey[]> = {
  contact: (id) => [
    ["contact", id],
    ["contact360", id],
    ["contactBrief", id],
    ...derivedRecordKeys("contact", id),
  ],
  company: (id) => [
    ["company", id],
    ["company360", id],
    ["account-scan", id],
    // The fit's completeness counts the profile fields and facts a write changes.
    ["company-growth-fit", id],
    // The dossier's prose cites the profile fields and facts a write changes.
    ["company-dossier", id],
    ...derivedRecordKeys("company", id),
  ],
  deal: (id) => dealRecordKeys(id),
  // The lead's set is already declared beside the lead's own reads, including
  // the board and the list its detail page does not prefix-reach.
  lead: (id) => leadWriteKeys(id),
  project: (id) => [["project", id]],
};

// Invalidate every read a write to one record makes stale. The callers are
// restore callbacks, which have no cache work of their own to do beyond this
// — the history panel invalidates its OWN queries, and the record is the
// caller's half of that pair.
export function invalidateRecord(
  client: QueryClient,
  kind: EntityKind,
  id: string,
  scope: RecordWriteScope = {},
): Promise<void> {
  const filters: InvalidateQueryFilters[] = recordWriteKeys(kind, id).map(
    (queryKey) => ({ queryKey }),
  );
  if (scope.anchor) filters.push(...ANCHOR_WRITE_FILTERS);
  if (scope.stageMove) filters.push({ queryKey: dealStageHistoryKey(id) });
  return Promise.all(
    filters.map((filter) => client.invalidateQueries(filter)),
  ).then(() => undefined);
}

// The reads the server refuses once a record is archived or merged away.
const RETIRED_REFUSED_KEYS: Partial<
  Record<EntityKind, (id: string) => QueryKey[]>
> = {
  contact: (id) => [["contact360", id], recordListsKey("contact", id)],
  lead: (id) => [leadScoreKey(id), recordListsKey("lead", id)],
};

// invalidateRecord for a record just archived or merged away, less the reads
// the server now refuses. Their readers still hold the live row just after
// the write, so a refresh then would only fail. They stop asking once the
// archived row arrives.
export async function invalidateRetiredRecord(
  client: QueryClient,
  kind: EntityKind,
  id: string,
): Promise<void> {
  const refused = RETIRED_REFUSED_KEYS[kind]?.(id) ?? [];
  await Promise.all(
    refused.map((queryKey) => client.cancelQueries({ queryKey })),
  );
  const askable = (queryKey: QueryKey) =>
    !refused.some((key) => partialMatchKey(queryKey, key));
  await Promise.all(
    recordWriteKeys(kind, id).map((queryKey) =>
      client.invalidateQueries({
        queryKey,
        predicate: (query) => askable(query.queryKey),
      }),
    ),
  );
}

export function invalidateCompany(
  client: QueryClient,
  company: Readonly<{ id: string; is_anchor?: boolean }>,
): Promise<void> {
  return invalidateRecord(client, "company", company.id, {
    anchor: company.is_anchor,
  });
}
