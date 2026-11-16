// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { queryOptions, skipToken } from "@tanstack/react-query";
import { api } from "../api/client";
import { unwrap } from "./problem";

// The contact row, under the key a record write invalidates (recordwritekeys.ts).
// No id yet means no request, which a caller still waiting on one relies on.
export function contactQuery(contactId: string | undefined) {
  return queryOptions({
    queryKey: ["contact", contactId],
    queryFn:
      contactId == null
        ? skipToken
        : async ({ signal }) =>
            unwrap(
              await api.GET("/contacts/{id}", {
                signal,
                params: { path: { id: contactId } },
              }),
            ),
  });
}

export function contact360Query(contactId: string) {
  return queryOptions({
    queryKey: ["contact360", contactId],
    queryFn: async ({ signal }) =>
      unwrap(
        await api.GET("/contacts/{id}/360", {
          signal,
          params: { path: { id: contactId } },
        }),
      ),
  });
}

// The contacts around a contact: current colleagues and the others on its deals.
// One page at the contract's ceiling. The composer matches typed names in it,
// and a name on a second page would never be offered.
export function relatedContactsQuery(
  contactId: string | undefined,
  readerScope: string,
) {
  return queryOptions({
    queryKey: ["contactsRelatedTo", contactId, readerScope],
    queryFn:
      contactId == null
        ? skipToken
        : async ({ signal }) =>
            unwrap(
              await api.GET("/contacts", {
                signal,
                params: { query: { related_to: contactId, limit: 200 } },
              }),
            ).data,
    staleTime: 60_000,
  });
}

type RowRead<T> = Readonly<{
  isError: boolean;
  isFetching: boolean;
  isFetchedAfterMount: boolean;
  data?: T;
}>;

// The row this page may show as the server's answer. A row cached before the
// page mounted is not trusted while its first re-read is in flight, because
// that re-read may refuse it. A failed read keeps its old row in the cache,
// and that old row is not trusted either.
export function confirmedContactRow<T>(record: RowRead<T>): T | undefined {
  if (record.isError) return undefined;
  if (record.isFetching && !record.isFetchedAfterMount) return undefined;
  return record.data;
}

export function archivedContactRow<T extends { archived_at?: string | null }>(
  record: RowRead<T>,
): T | undefined {
  const row = confirmedContactRow(record);
  return row?.archived_at ? row : undefined;
}

// What the opening may name. It names nobody while a refused 360 or an
// untrusted archived row waits on the row read, so neither leaks a name.
export function openingIdentity<T extends { archived_at?: string | null }>(
  record: RowRead<T>,
  view: Readonly<{ isError: boolean }>,
): { contact?: T; hideIdentity: boolean } {
  if (view.isError || record.data?.archived_at) return { hideIdentity: true };
  return { contact: confirmedContactRow(record), hideIdentity: false };
}

// A 360 refused while the row is re-read may be an archive the row will report.
// So the contact page waits for the row before it calls itself unopened. It
// shows no record while it waits, because the old 360 would offer actions the
// refusal has withdrawn. A cached archived row waits for its re-read the same way.
export function contactPageState(
  record: Readonly<{
    isPending: boolean;
    isError: boolean;
    isFetching: boolean;
    data?: { archived_at?: string | null };
  }>,
  view: Readonly<{ isLoading: boolean; isError: boolean }>,
): "opening" | "notOpened" | "ready" {
  if (record.isError) return "notOpened";
  if (record.isPending || view.isLoading) return "opening";
  if (record.isFetching && (view.isError || record.data?.archived_at)) {
    return "opening";
  }
  if (view.isError) return "notOpened";
  return "ready";
}
