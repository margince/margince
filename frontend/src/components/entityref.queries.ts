// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The record-name reads EntityRef's components hydrate from (one read per
// entity kind, keyed and cached), as opposed to the components that render
// them, which stay in entityref.tsx.

import { type UseQueryOptions, useQuery } from "@tanstack/react-query";
import { api } from "../api/client";
import type { EntityKind } from "../lib/entity";
import { idBatcher } from "../lib/idbatch";
import { unwrap } from "../lib/problem";

const REFERENCE_IDS_PER_READ = 100;

export type EntityReference = {
  name: string | null;
  archived: boolean;
  logo?: string | null;
};

function referenceReader(kind: EntityKind) {
  return idBatcher<EntityReference>(REFERENCE_IDS_PER_READ, async (page) => {
    const data = unwrap(
      await api.GET("/records/references", {
        params: { query: { kind, id: [...page] } },
      }),
    );
    return new Map(
      data.data.map((reference) => [
        reference.id,
        {
          name: reference.name,
          archived: reference.archived,
          ...(kind === "company" ? { logo: reference.logo } : {}),
        },
      ]),
    );
  });
}

const NAME_READERS: Record<
  EntityKind,
  (id: string) => Promise<EntityReference | null>
> = {
  contact: referenceReader("contact"),
  company: referenceReader("company"),
  lead: referenceReader("lead"),
  project: referenceReader("project"),
  deal: referenceReader("deal"),
};

export function fetchEntityName(
  kind: EntityKind,
  id: string,
  signal?: AbortSignal,
): Promise<string | null> {
  return fetchEntityReference(kind, id, signal).then(
    (reference) => reference?.name ?? null,
  );
}

export async function fetchEntityReference(
  kind: EntityKind,
  id: string,
  signal?: AbortSignal,
): Promise<EntityReference | null> {
  signal?.throwIfAborted();
  const reference = await NAME_READERS[kind](id);
  signal?.throwIfAborted();
  return reference;
}

// Non-company readers share a cached display name; companies share rich references.
export function entityNameQuery(
  kind: Exclude<EntityKind, "company">,
  id: string,
) {
  return {
    queryKey: [kind, ENTITY_NAME_KEY, id],
    // Other observers share this batch; retain its answer after unmount.
    queryFn: () => fetchEntityName(kind, id),
    // References change rarely relative to the pages that render them; a short
    // cache keeps a 360 from re-fetching the same name on every hover/refetch.
    staleTime: 60_000,
    retry: false,
  } as const;
}

// Retrying a failed batch here would send a separate request for each record.
export function companyReferenceQuery(id: string) {
  return {
    queryKey: ["company", ENTITY_NAME_KEY, id, "reference"],
    // The batch serves other observers too; keep its answer after unmount.
    queryFn: () => NAME_READERS.company(id),
    staleTime: 60_000,
    retry: false,
  } as const;
}

export function entityNameObserverQuery(
  kind: EntityKind,
  id: string,
): UseQueryOptions<EntityReference | string | null, Error, string | null> {
  const options =
    kind === "company" ? companyReferenceQuery(id) : entityNameQuery(kind, id);
  return {
    ...options,
    select: (value: EntityReference | string | null) =>
      typeof value === "string" ? value : (value?.name ?? null),
  };
}

// Shared with mutation invalidation so renames refresh every mounted label.
export const ENTITY_NAME_KEY = "ref";

// Pending and failed reads cannot establish that a name is absent.
// Only a settled omission may display the record ID as its fallback.
export type NameReading = "pending" | "failed" | "unnamed";

export function readingOf(
  query: Readonly<{ isPending: boolean; isError: boolean }>,
): NameReading {
  if (query.isPending) {
    return "pending";
  }
  return query.isError ? "failed" : "unnamed";
}

/**
 * A caller-supplied or read-back name is usable only when it says something.
 *
 * Blank and whitespace-only are the same claim (the source has nothing)
 * rather than a record whose name is a space, so neither skips the lookup and
 * neither becomes a label. A button carrying one is a link a reader can neither
 * read nor find.
 */
export function usableName(name: string | null | undefined): string | null {
  const trimmed = name?.trim();
  return trimmed ? trimmed : null;
}

// The resolved display name only, sharing EntityRef's exact cache entry so
// nothing is fetched twice. Exported for chrome that wants the name as plain
// text rather than as EntityRef's navigating button: the breadcrumb names the
// record you are already looking at, so linking it would go nowhere.
export function useEntityName(
  kind: EntityKind,
  id: string | null | undefined,
): { name: string | null; reading: NameReading; fetching: boolean } {
  const query = useQuery({
    ...entityNameObserverQuery(kind, id ?? ""),
    enabled: Boolean(id),
  });
  // The reading travels with the name, because a caller handed only `null`
  // cannot tell a name that is still coming from one that will never come, and
  // every caller that has had to guess has guessed the id.
  //
  // `fetching` is true while a read is under way or paused, cached answer or
  // not. A cached "no name" can be replaced by the read still coming.
  return {
    name: usableName(query.data),
    reading: readingOf(query),
    fetching: query.fetchStatus !== "idle",
  };
}
