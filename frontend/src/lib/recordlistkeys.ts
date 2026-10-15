// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** The record types whose rows a list screen caches. */
export type ListedRecordType = "company" | "contact" | "lead" | "deal";

// The list cache each type is filed under. Spelled out rather than derived,
// because `${type}s` asks for "companys", which nothing reads, and a key that
// matches no cache is not an error anywhere: invalidateQueries simply finds
// nothing, and the list goes on showing what changed as it was.
export const RECORD_LIST_KEY: Record<ListedRecordType, string> = {
  company: "companies",
  contact: "contacts",
  lead: "leads",
  deal: "deals",
};
