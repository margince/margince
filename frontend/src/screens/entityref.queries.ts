// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The record-name reads EntityRef's components hydrate from (one read per
// entity kind, keyed and cached), as opposed to the components that render
// them, which stay in entityref.tsx.

import { useQuery } from "@tanstack/react-query";
import { api } from "../api/client";
import type { EntityKind } from "../app/entity";
import { leadIdentityName } from "../format/leadname";
import { throwProblem } from "./common";

/**
 * What a read that carried no name is allowed to mean.
 *
 * A 404 is an ANSWER: the record is gone, or row-scope hides its existence from
 * this reader (the API hides a row it may not see rather than admitting it),
 * and no amount of waiting or asking again will produce a name. That is the
 * settled reading the id fallback exists for.
 *
 * Every other failure (a 403 on the object, a 5xx, a dropped connection) is a
 * read that never arrived, and it THROWS so react-query holds it as an error.
 * Flattened to null it would be indistinguishable from the answer above, and
 * the two say opposite things about whether this is worth asking again.
 */
function unnamedOrThrow(error: unknown, response: Response): null {
  if (response.status === 404) {
    return null;
  }
  throwProblem(error);
}

// Each entity endpoint names its display field differently. Missing names
// resolve to null because React Query rejects undefined query results.
const NAME_READERS: Record<EntityKind, (id: string) => Promise<string | null>> =
  {
    contact: async (id) => {
      const { data, error, response } = await api.GET("/contacts/{id}", {
        params: { path: { id } },
      });
      if (error) return unnamedOrThrow(error, response);
      return data.full_name ?? null;
    },
    company: async (id) => {
      const { data, error, response } = await api.GET("/companies/{id}", {
        params: { path: { id } },
      });
      if (error) return unnamedOrThrow(error, response);
      return data.display_name ?? null;
    },
    lead: async (id) => {
      const { data, error, response } = await api.GET("/leads/{id}", {
        params: { path: { id } },
      });
      if (error) return unnamedOrThrow(error, response);
      return leadIdentityName(data) || null;
    },
    project: async (id) => {
      const { data, error, response } = await api.GET("/projects/{id}", {
        params: { path: { id } },
      });
      if (error) return unnamedOrThrow(error, response);
      return data.name ?? null;
    },
    deal: async (id) => {
      const { data, error, response } = await api.GET("/deals/{id}", {
        params: { path: { id } },
      });
      if (error) return unnamedOrThrow(error, response);
      return data.name ?? null;
    },
  };

export function fetchEntityName(
  kind: EntityKind,
  id: string,
): Promise<string | null> {
  return NAME_READERS[kind](id);
}

/**
 * The segment that marks a query as one record's display name.
 *
 * Named because two readers key on it: the reads below, and the data layer,
 * which brings every mounted name back after a successful write
 * (app/queryclient.ts). A write can rename its record, and the trail at the top
 * of the window is naming it. Held apart by a literal in two files, a rename
 * would have gone on showing the old name until the reader reloaded.
 */
export const ENTITY_NAME_KEY = "ref";

/**
 * The three readings of a reference the page cannot put a name to.
 *
 * `pending` is a read that has not answered yet, and it is allowed to say so.
 * `unnamed` is a read that answered and carried no name: a record with a blank
 * display field, or one the API will not admit exists (see `unnamedOrThrow`);
 * there the id is what is left, and on the surfaces that keep this fallback
 * (an audit row, a history entry, a record the reader may not open) it is the
 * one traceable fact, so it stays. `failed` is a read that never arrived, and
 * it may not borrow either spelling: painting the id while the name is still on
 * its way is how a record page came to show a uuid for a moment on every load,
 * and painting it for a 403 or a 500 states as settled fact a question nothing
 * answered.
 */
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
): { name: string | null; reading: NameReading } {
  const query = useQuery({
    queryKey: [kind, ENTITY_NAME_KEY, id],
    queryFn: () => fetchEntityName(kind, id ?? ""),
    enabled: Boolean(id),
    staleTime: 60_000,
  });
  // The reading travels with the name, because a caller handed only `null`
  // cannot tell a name that is still coming from one that will never come, and
  // every caller that has had to guess has guessed the id.
  return { name: usableName(query.data), reading: readingOf(query) };
}
