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
function useDealWinEvidence(dealId: string | null, asked: boolean) {
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

type AskedAdvance = Readonly<{
  dealId: string;
  toStage: Readonly<{ semantic: string }>;
}>;

/**
 * What the win dialog asks about contract evidence, for the advance it was
 * opened on.
 *
 * `needed` means ask how the win happened: the read found no signed contract,
 * or the server refused this deal's win for naming none. `checking` holds
 * Confirm while the read is out. `stillBare` asks again at the press, because
 * the server takes a stated reason without looking for paper.
 */
export function useWinQuestion(
  pending: AskedAdvance | null,
  refusedDealId: string | null,
) {
  const asked = pending?.toStage.semantic === "won";
  const evidence = useDealWinEvidence(pending?.dealId ?? null, asked);
  const refused = pending !== null && refusedDealId === pending.dealId;
  return {
    needed: asked && (evidence.data?.has_signed_contract === false || refused),
    checking: asked && evidence.isFetching,
    stillBare: async (dealId: string) =>
      refusedDealId === dealId ||
      (await evidence.refetch()).data?.has_signed_contract !== true,
  };
}
