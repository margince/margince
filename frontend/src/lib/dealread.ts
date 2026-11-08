// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { api } from "../api/client";
import { unwrap } from "./problem";

/**
 * useDeal is the read of one deal record, for the deal page and the Worklist's details.
 *
 * A hook rather than a query inline in the screen, so the four record reads
 * are mounted the same way. That lets one suite prove the live cadence over
 * all of them (FE-PARAM-5, app/queryclient.ts). The cadence itself is the
 * client's, keyed on the read: nothing is spread in here to forget.
 */
export function useDeal(id: string) {
  return useQuery({
    queryKey: ["deal", id],
    queryFn: async () => {
      return unwrap(
        await api.GET("/deals/{id}", {
          params: { path: { id } },
        }),
      );
    },
  });
}
