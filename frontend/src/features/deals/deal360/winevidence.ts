// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { api } from "../../../api/client";
import { unwrap } from "../../../lib/problem";

/**
 * Whether the deal has the signed agreement a win rests on, asked while the
 * dialog that closes it as won is open.
 *
 * Never kept, because a cached answer from before an agreement was archived
 * would let the reader through to a refusal. The server asks again when the
 * win lands. A failed read leaves the data empty, and the dialog then asks
 * only if the server refuses.
 */
export function useDealWinEvidence(dealId: string | null, asked: boolean) {
  return useQuery({
    queryKey: ["deals", dealId, "win-evidence"],
    enabled: asked && dealId !== null,
    gcTime: 0,
    staleTime: 0,
    retry: false,
    queryFn: async () =>
      unwrap(
        await api.GET("/deals/{id}/win-evidence", {
          params: { path: { id: dealId ?? "" } },
        }),
      ),
  });
}
