// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { queryOptions, skipToken } from "@tanstack/react-query";
import { api } from "../api/client";
import { unwrap } from "../lib/problem";

// The contact row, under the key a record write invalidates (recordwritekeys.ts).
// No id yet means no request, which a caller still waiting on one relies on.
export function contactQuery(contactId: string | undefined) {
  return queryOptions({
    queryKey: ["contact", contactId],
    queryFn:
      contactId == null
        ? skipToken
        : async () =>
            unwrap(
              await api.GET("/contacts/{id}", {
                params: { path: { id: contactId } },
              }),
            ),
  });
}

// A 360 refused while the row is re-read may be an archive the row will report.
// So the contact page waits for the row before it calls itself unopened.
export function contactPageState(
  record: Readonly<{
    isPending: boolean;
    isError: boolean;
    isFetching: boolean;
  }>,
  view: Readonly<{ isLoading: boolean; isError: boolean }>,
): "opening" | "notOpened" | "ready" {
  if (record.isPending || view.isLoading) return "opening";
  if (record.isError || (view.isError && !record.isFetching)) {
    return "notOpened";
  }
  return "ready";
}
