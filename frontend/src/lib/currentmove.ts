// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  type QueryClient,
  useMutation,
  useQueryClient,
} from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { useT } from "../i18n";
import { ProblemError, unwrap } from "./problem";

type Receipt = components["schemas"]["RecommendationReceipt"];

export async function refreshSuggestedMoves(client: QueryClient) {
  await Promise.all([
    client.invalidateQueries({ queryKey: ["deal-status"] }),
    client.invalidateQueries({ queryKey: ["worklist"] }),
    client.invalidateQueries({ queryKey: ["contact360"] }),
  ]);
}

export function useCurrentMove<Operand>(open: (operand: Operand) => void) {
  const t = useT();
  const client = useQueryClient();
  return useMutation({
    mutationFn: async ({
      receipt,
    }: {
      receipt: Receipt | undefined;
      operand: Operand;
    }) => {
      if (!receipt) return;
      const card = unwrap(
        await api.GET("/deals/{id}/status", {
          params: {
            path: { id: receipt.deal_id },
            query: {
              facts_only: true,
              if_recommendation: receipt.basis_fingerprint,
              if_rule: receipt.rule,
            },
          },
        }),
        t,
      );
      const current = card.next?.recommendation;
      if (
        !current ||
        current.deal_id !== receipt.deal_id ||
        current.rule !== receipt.rule ||
        current.policy_version !== receipt.policy_version ||
        current.basis_fingerprint !== receipt.basis_fingerprint
      ) {
        throw new ProblemError({ status: 409, code: "version_skew" }, t);
      }
      return card;
    },
    onSuccess: (_card, request) => open(request.operand),
    onError: () => refreshSuggestedMoves(client),
  });
}
