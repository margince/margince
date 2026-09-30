// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQueries, useQuery } from "@tanstack/react-query";
import { api } from "../api/client";
import { throwProblem } from "./common";

// Names a colleague by id, batching every id asked for within one tick into
// a single `GET /users/names` request rather than one request per reference.

/** The most seats one request names — the contract's own `id` bound. */
const MAX_SEATS_PER_REQUEST = 100;

// The query every reader of one id's name shares — `useMemberName` and
// `useMemberNames` both build their observers off this SAME options object,
// keyed by the id alone, so a name either one has already read costs the
// other nothing: same cache entry, same in-flight request.
function memberNameQueryOptions(id: string) {
  return {
    queryKey: ["users", "name", id],
    queryFn: () => nameOf(id),
    // A colleague's name changes far more rarely than the screens that draw it
    // re-render, so a name already read is not asked for again.
    staleTime: 60_000,
    // The app default retries a server fault, but each retryer opens its own
    // window after this batch's has already closed, turning one failed batch
    // into up to N single-id requests. Re-asked on the next invalidation.
    retry: false,
  } as const;
}

/**
 * The name behind one id: a string names the seat, `null` is a settled absence
 * (archived, or an id this installation never held), a pending read is a name
 * still coming, and an error is a read that never arrived. Four readings,
 * because collapsing any two of them is how a reference comes to claim a
 * colleague does not exist when the server merely refused.
 */
export function useMemberName(id: string | null | undefined) {
  return useQuery({
    ...memberNameQueryOptions(id ?? ""),
    // `id` is non-null wherever the query actually runs: `enabled` gates it,
    // and the key above already distinguishes each id's cache entry.
    enabled: Boolean(id),
  });
}

/**
 * Every id a caller already holds, named at once — a list row, a board card,
 * a chronology's own colleagues — rather than one `useMemberName` per row,
 * which is not an option: a hook cannot be called inside a `.map()`.
 *
 * An id absent from the map is pending, failed, or was never asked; no caller
 * renders those apart for a bulk read today, so none is drawn here.
 */
export function useMemberNames(
  ids: readonly string[],
): ReadonlyMap<string, string> {
  const unique = [...new Set(ids.filter(Boolean))];
  const reads = useQueries({
    queries: unique.map((id) => memberNameQueryOptions(id)),
  });
  const names = new Map<string, string>();
  reads.forEach((read, index) => {
    const id = unique[index];
    if (id !== undefined && read.data != null) {
      names.set(id, read.data);
    }
  });
  return names;
}

// The ids this tick has asked about and not yet sent. A leaf component cannot
// see its siblings, so the coalescing has to live beside the request rather
// than at any one call site — one shared batch, every subscriber served from
// the one answer it produces.
let queued: string[] = [];
let batch: Promise<ReadonlyMap<string, string>> | null = null;

async function nameOf(id: string): Promise<string | null> {
  const named = await join(id);
  return named.get(id) ?? null;
}

function join(id: string): Promise<ReadonlyMap<string, string>> {
  queued.push(id);
  if (!batch) {
    // Opened as a microtask: every component that renders in this tick has
    // enqueued by the time it runs, and nothing waits on a timer.
    batch = Promise.resolve().then(() => {
      const asking = [...new Set(queued)];
      queued = [];
      batch = null;
      return readNames(asking);
    });
  }
  return batch;
}

async function readNames(
  ids: readonly string[],
): Promise<ReadonlyMap<string, string>> {
  const named = new Map<string, string>();
  for (let at = 0; at < ids.length; at += MAX_SEATS_PER_REQUEST) {
    const page = ids.slice(at, at + MAX_SEATS_PER_REQUEST);
    const { data, error } = await api.GET("/users/names", {
      params: { query: { id: page } },
    });
    if (error) {
      // Thrown, so react-query holds it as an error on every reference this
      // batch covered. Caught here it would come back as an absent name, which
      // reads as a colleague who does not exist.
      throwProblem(error);
    }
    for (const seat of data.data) {
      named.set(seat.id, seat.display_name);
    }
  }
  return named;
}
