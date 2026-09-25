// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { throwProblem } from "./common";

export type MagicReceipt = components["schemas"]["MagicReceipt"];
export type MagicLine = components["schemas"]["MagicLine"];
export type MagicNotShown = components["schemas"]["MagicNotShown"];
// Derived from the line rather than spelled again: a lane renamed in crm.yaml
// is a compile error here instead of a heading over rows that never arrive.
export type MagicLane = MagicLine["lane"];

export const magicKey = ["magic"] as const;

/**
 * What the machinery did since this reader last looked.
 *
 * No `enabled` gate: every line is about the reader's own records, scoped by
 * the same row predicates their record pages use, and the server refuses a
 * caller with no human behind it — which is the only refusal this read has.
 *
 * `since` is left to the server. Absent, it resolves to the reader's own last
 * brief cutoff, which is the window they have not already read; a client
 * choosing one would be a second answer to "since when".
 */
export function useMagic() {
  return useQuery({
    queryKey: magicKey,
    queryFn: async (): Promise<MagicReceipt> => {
      const { data, error } = await api.GET("/magic", {});
      if (error) {
        throwProblem(error);
      }
      return data;
    },
  });
}
