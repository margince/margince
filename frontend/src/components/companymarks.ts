// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQueries } from "@tanstack/react-query";
import { stable } from "../format/collate";
import { companyReferenceQuery } from "./entityref.queries";

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

export function useCompanyMarksByID(ids: readonly string[]): CompanyMarksByID {
  const unique = [...new Set(ids.filter(Boolean))].sort(stable);
  const reads = useQueries({
    queries: unique.map(companyReferenceQuery),
  });
  const marks = new Map<string, CompanyMark>();
  const unreadable = new Set<string>();
  const pending = new Set<string>();
  unique.forEach((id, index) => {
    const read = reads[index];
    if (!read) return;
    if (read.data) {
      marks.set(id, { name: read.data.name ?? "", logoUrl: read.data.logo });
    } else if (read.isError) {
      unreadable.add(id);
    } else if (read.isPending) {
      pending.add(id);
    }
  });
  return { marks, unreadable, pending };
}

/**
 * Every company the loaded deals name, id → mark (`useCompanyMarks` resolves them).
 *
 * A company this reader may not read is in no map. The wire sends
 * `company_id` as null and names it in `masked_fields`. The card then shows
 * the withheld reading as its own mask, not a name this screen could supply.
 */
export type CompanyMarks = ReadonlyMap<string, CompanyMark>;

/**
 * What the screen knows about the companies its deals name.
 *
 * `unreadable` holds the companies whose read failed: a 403, a 5xx or a
 * dropped connection. That is not the same fact as a deal naming no company.
 * The table shows it through `EntityRef`'s failed state; this is the board's half.
 */
export type CompanyNaming = Readonly<{
  marks: CompanyMarks;
  unreadable: ReadonlySet<string>;
}>;
