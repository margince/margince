// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { unwrap } from "./problem";
import { worklistKey } from "./workkeys";

export type HandledForYou = components["schemas"]["HandledForYou"];

export type Receipt = components["schemas"]["Receipt"];

/**
 * What the product did on this reader's behalf.
 *
 * No `enabled` gate, unlike the manager reads beside it. A receipt is the
 * reader's own and every act it reports was taken for them, so there is no tier
 * to hold. The server refuses a caller with no human behind it, which is the
 * only refusal this read has.
 */
export function useHandledForYou() {
  return useQuery({
    queryKey: [...worklistKey, "handled"],
    queryFn: async (): Promise<HandledForYou> => {
      return unwrap(await api.GET("/worklist/handled", {}));
    },
  });
}
