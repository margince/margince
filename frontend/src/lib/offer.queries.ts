// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { api } from "../api/client";
import { unwrap } from "./problem";

export function useOffer(id?: string) {
  return useQuery({
    queryKey: ["offer", id],
    queryFn: async () => {
      if (!id) throw new Error("an offer id is required");
      return unwrap(
        await api.GET("/offers/{id}", { params: { path: { id } } }),
      );
    },
    enabled: !!id,
    // Only regeneration's response carries the transient disclosure and diff.
    refetchOnMount: (query) => query.state.data?.ai_generated !== true,
  });
}
