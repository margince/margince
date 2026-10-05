// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type QueryClient, useQuery } from "@tanstack/react-query";
import { api } from "../api/client";
import { throwProblem } from "./common";

// Which providers are not answering, read once for every surface that shows it.
// The Providers list and the System health card both call this with the same
// query key, so they share one fetch and cannot disagree about a provider.
export function useProviderHealth(canSee: boolean) {
  return useQuery({
    queryKey: ["ai-provider-health"],
    enabled: canSee,
    queryFn: async () => {
      const { data, error } = await api.GET("/ai/provider-health");
      if (error) throwProblem(error);
      return data;
    },
    // Same cadence and same off-with-the-grant rule as useAiHealth: the
    // question is whether it answers now.
    refetchInterval: canSee ? 60_000 : false,
  });
}

// A key, settings or routing save, and a successful key test, change what the
// server holds about a provider's health; the badge reads it again now rather
// than at the next poll.
export function invalidateProviderHealth(queryClient: QueryClient) {
  return queryClient.invalidateQueries({ queryKey: ["ai-provider-health"] });
}
