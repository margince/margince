// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { throwProblem } from "./common";

// The stored routing document and the ETag it was read at. One read, shared by
// every card that shows a binding and every editor that writes one.

export type RoutingRead = {
  routing: components["schemas"]["AiRouting"];
  version: string;
};

export const ROUTING_KEY = ["ai-routing"] as const;

export async function fetchRouting(): Promise<RoutingRead> {
  const { data, error, response } = await api.GET("/ai/routing");
  if (error || !response.ok) {
    throwProblem(error);
  }
  if (!data) throw new Error("AI routing unavailable");
  return { routing: data, version: response.headers.get("ETag") ?? "" };
}

export function useRouting(enabled: boolean) {
  return useQuery({ enabled, queryKey: ROUTING_KEY, queryFn: fetchRouting });
}

// The vendors the routing document names, chat lanes and the embedding lane
// alike. The embedding lane is in here on purpose: retrieval binds separately
// and can be the only thing pointing at an unkeyed vendor, which is exactly the
// case a reader would otherwise find out about from a failed reindex.
//
// `null` for a body that is not the routing document. `tiers` and `embeddings`
// are both REQUIRED of the response, so the type above says they are there —
// but the type is a promise the WIRE does not keep: nothing validates a 200,
// and reading `Object.values(undefined)` threw, which the error boundary turned
// into the whole settings page saying "this view no longer works". A server too
// old, a projection that lost a field or a proxy answering something else are
// all real ways to get such a body, and none of them should cost a reader the
// page. The caller already draws an unanswered read; this is one.
export function boundProviders(
  routing: RoutingRead["routing"],
): Set<string> | null {
  const usage = providerUsage(routing);
  return usage === null ? null : new Set(usage.keys());
}

export type ProviderUse = {
  for: string[];
  baseUrls: string[];
  // The models routing binds on this vendor, each model and lane once however
  // many tiers share it.
  models: { model: string; lane: "chat" | "embeddings" | "decisions" }[];
};

// What each bound vendor is bound FOR, in reading order: the tiers by their
// own names, then the embedder and the decision model. One walk of the
// document, so "which vendors" and "for what" cannot disagree.
export function providerUsage(
  routing: RoutingRead["routing"],
): Map<string, ProviderUse> | null {
  if (routing.tiers === undefined || routing.embeddings === undefined) {
    return null;
  }
  const usage = new Map<string, ProviderUse>();
  const add = (
    role: string,
    binding: { provider: string; model: string; base_url?: string },
  ) => {
    const held = usage.get(binding.provider) ?? {
      for: [],
      baseUrls: [],
      models: [],
    };
    held.for.push(role);
    const lane = role === "embeddings" || role === "decisions" ? role : "chat";
    if (
      !held.models.some((m) => m.model === binding.model && m.lane === lane)
    ) {
      held.models.push({ model: binding.model, lane });
    }
    if (binding.base_url) held.baseUrls.push(binding.base_url);
    usage.set(binding.provider, held);
  };
  for (const [tier, binding] of Object.entries(routing.tiers)) {
    add(tier, binding);
  }
  add("embeddings", routing.embeddings);
  // Bound apart from the tiers, and its key is demanded like theirs.
  if (routing.decisions) {
    add("decisions", routing.decisions);
  }
  return usage;
}
