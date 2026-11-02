// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { queryOptions, skipToken } from "@tanstack/react-query";
import { api } from "../api/client";
import { leadKey } from "./leadkeys";
import { unwrap } from "./problem";

// The lead row, under the key every lead write invalidates (leadkeys.ts). No id
// means no request.
export function leadQuery(leadId: string | undefined) {
  return queryOptions({
    queryKey: leadKey(leadId ?? ""),
    queryFn:
      leadId == null
        ? skipToken
        : async () =>
            unwrap(
              await api.GET("/leads/{id}", {
                params: { path: { id: leadId } },
              }),
            ),
  });
}
