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
