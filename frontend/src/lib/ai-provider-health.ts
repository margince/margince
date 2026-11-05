// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type QueryClient, useQuery } from "@tanstack/react-query";
import { api } from "../api/client";
import { useHealthScope, useHealthVisible } from "./healthscope";
import { unwrap } from "./problem";

// Which providers are not answering, read once for every surface that shows it.
// The Providers list and the System health card both call this with the same
// query key, so they share one fetch and cannot disagree about a provider.
export function useProviderHealth(canSee: boolean) {
  const scope = useHealthScope();
  const visible = useHealthVisible();
  return useQuery({
    queryKey: ["ai-provider-health", scope],
    enabled: canSee && visible && scope !== undefined,
    // A malformed body fails the read inside the gate, not a reading outside
    // any card. A server older than `configured` omits it: none configured.
    queryFn: async ({ signal }) => {
      const health = unwrap(await api.GET("/ai/provider-health", { signal }));
      const configured: unknown = health?.configured ?? [];
      if (!Array.isArray(health?.providers) || !isNames(configured)) {
        throw new Error("malformed provider health response");
      }
      return { ...health, configured };
    },
    // Same cadence and same off-with-the-grant rule as useAiHealth: the
    // question is whether it answers now.
    refetchInterval: canSee ? 60_000 : false,
  });
}

function isNames(value: unknown): value is string[] {
  return (
    Array.isArray(value) && value.every((name) => typeof name === "string")
  );
}

// A boot whose stored binding cannot be served may fall back to the offline
// fake. Its lanes answer with canned text while the screen still names a vendor.
export function answersFromStandIn(
  configured: readonly string[] | undefined,
): boolean {
  return configured?.includes("fake") === true;
}

// Whether every lane answers from the fake, so no real vendor is reachable.
export function answersOnlyFromStandIn(
  configured: readonly string[] | undefined,
): boolean {
  return (
    answersFromStandIn(configured) &&
    configured?.every((p) => p === "fake") === true
  );
}

// A key, settings or routing save, and a successful key test, change what the
// server holds about a provider's health; the badge reads it again now rather
// than at the next poll.
export function invalidateProviderHealth(queryClient: QueryClient) {
  return Promise.all([
    queryClient.invalidateQueries({ queryKey: ["ai-provider-health"] }),
    queryClient.invalidateQueries({ queryKey: ["system-health"] }),
  ]);
}
