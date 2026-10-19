// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQueries } from "@tanstack/react-query";
import { api } from "../api/client";
import { stable } from "../format/collate";
import { unwrap } from "../lib/problem";

// Names many companies by id, a hundred to a request. One read per company
// made a page of rows wait on dozens of requests before it showed names.

/** A company's display name and the mark drawn beside it. */
export type CompanyMark = { name: string; logoUrl?: string | null };

/**
 * What the batched read knows about each id asked for.
 *
 * `unreadable` holds ids whose read failed (a 403, a 5xx, a lost connection).
 * An id missing from `marks` after the read answered is a different fact: row
 * scope hides it. `pending` holds ids whose read has not answered yet.
 */
export type CompanyMarksByID = Readonly<{
  marks: ReadonlyMap<string, CompanyMark>;
  unreadable: ReadonlySet<string>;
  pending: ReadonlySet<string>;
}>;

/** The most companies one read asks for: the contract's own `id` bound. */
const COMPANY_MARK_BATCH = 100;

export function useCompanyMarksByID(ids: readonly string[]): CompanyMarksByID {
  // Sorted and deduplicated, so the same set of ids is the same query key
  // whatever order the rows named them in.
  const unique = [...new Set(ids.filter(Boolean))].sort(stable);
  const batches: string[][] = [];
  for (let at = 0; at < unique.length; at += COMPANY_MARK_BATCH) {
    batches.push(unique.slice(at, at + COMPANY_MARK_BATCH));
  }
  const reads = useQueries({
    queries: batches.map((batch) => ({
      queryKey: ["companies", "marks", batch],
      queryFn: async (): Promise<Map<string, CompanyMark>> => {
        // A refused read is held as an error, as components/entityref.tsx
        // does, so each row it covers says its company did not load.
        const data = unwrap(
          await api.GET("/companies", {
            params: {
              // Archived as well: archiving a company leaves the records that
              // name it naming it, and the single-record read answers those.
              query: {
                id: batch,
                include_anchor: true,
                include_archived: true,
                limit: batch.length,
              },
            },
          }),
        );
        return new Map(
          data.data.map((company) => [
            company.id,
            { name: company.display_name, logoUrl: company.logo_url },
          ]),
        );
      },
      // A company's name and mark change far more rarely than a page
      // refetches, so a row that already has one does not ask again.
      staleTime: 60_000,
    })),
  });
  const marks = new Map<string, CompanyMark>();
  const unreadable = new Set<string>();
  const pending = new Set<string>();
  reads.forEach((read, index) => {
    const batch = batches[index] ?? [];
    if (read.data) {
      for (const [id, mark] of read.data) {
        marks.set(id, mark);
      }
      return;
    }
    const into = read.isError ? unreadable : pending;
    for (const id of batch) {
      into.add(id);
    }
  });
  return { marks, unreadable, pending };
}
