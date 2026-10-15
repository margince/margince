// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type {
  InvalidateQueryFilters,
  QueryClient,
  QueryKey,
} from "@tanstack/react-query";
import { dealRecordKeys, derivedRecordKeys } from "./activitykeys";
import type { EntityKind } from "./entity";
import { leadWriteKeys } from "./leadkeys";

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
type RecordWriteScope = Readonly<{ anchor?: boolean }>;

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
  return Promise.all(
    filters.map((filter) => client.invalidateQueries(filter)),
  ).then(() => undefined);
}

export function invalidateCompany(
  client: QueryClient,
  company: Readonly<{ id: string; is_anchor?: boolean }>,
): Promise<void> {
  return invalidateRecord(client, "company", company.id, {
    anchor: company.is_anchor,
  });
}
