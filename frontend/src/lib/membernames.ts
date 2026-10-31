// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQueries, useQuery } from "@tanstack/react-query";
import { api } from "../api/client";
import { type Translator, useT } from "../i18n";
import { idBatcher } from "./idbatch";
import { ProblemError } from "./problem";

// Names a colleague by id, batching every id asked for within one tick into
// a single `GET /users/names` request rather than one request per reference.

/** The most seats one request names: the contract's own `id` bound. */
const MAX_SEATS_PER_REQUEST = 100;

// The query every reader of one id's name shares. `useMemberName` and
// `useMemberNames` both build their observers off this options object,
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
  const t = useT();
  return useQuery({
    ...memberNameQueryOptions(id ?? ""),
    // `id` is non-null wherever the query actually runs: `enabled` gates it,
    // and the key above already distinguishes each id's cache entry.
    enabled: Boolean(id),
    select: (seat: Seat | null) => (seat ? seatLabel(seat, t) : null),
  });
}

/** The name of a colleague who left, or null for anyone else or a pending read. */
export function useDepartedName(id: string | null | undefined): string | null {
  const read = useQuery({
    ...memberNameQueryOptions(id ?? ""),
    enabled: Boolean(id),
  });
  return read.data?.departed ? read.data.name : null;
}

/** A named seat, and whether that colleague left. */
type Seat = Readonly<{ name: string; departed: boolean }>;

/**
 * The name a screen prints. A colleague who left is marked, because the work
 * they still own counts in their team's views until somebody takes it over.
 */
function seatLabel(seat: Seat, t: Translator): string {
  return seat.departed ? t("ref.memberLeft", { name: seat.name }) : seat.name;
}

/**
 * Every id a caller already holds, named at once (a list row, a board card,
 * a chronology's own colleagues) rather than one `useMemberName` per row,
 * which is not an option: a hook cannot be called inside a `.map()`.
 *
 * An id absent from the map is pending, failed, or was never asked; no caller
 * renders those apart for a bulk read today, so none is drawn here.
 */
export function useMemberNames(
  ids: readonly string[],
): ReadonlyMap<string, string> {
  const t = useT();
  const unique = [...new Set(ids.filter(Boolean))];
  const reads = useQueries({
    queries: unique.map((id) => memberNameQueryOptions(id)),
  });
  const names = new Map<string, string>();
  reads.forEach((read, index) => {
    const id = unique[index];
    if (id !== undefined && read.data != null) {
      names.set(id, seatLabel(read.data, t));
    }
  });
  return names;
}

// Every id asked about within one tick shares one `GET /users/names` read.
const nameOf = idBatcher<Seat>(MAX_SEATS_PER_REQUEST, async (page) => {
  const { data, error } = await api.GET("/users/names", {
    params: { query: { id: [...page] } },
  });
  if (error) {
    // Thrown for these ids, because an absent name reads as a colleague who
    // does not exist.
    throw new ProblemError(error);
  }
  return new Map(
    data.data.map((seat) => [
      seat.id,
      { name: seat.display_name, departed: seat.departed === true },
    ]),
  );
});
