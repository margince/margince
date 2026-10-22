import { useQuery } from "@tanstack/react-query";
import { api } from "../api/client";
import { unwrap } from "./problem";

export type TierName = "auto_execute" | "confirmation_required" | "dynamic";
export type VerbTierMap = Record<string, TierName>;

// Approval `kind` → the tool verb that stages it. Any item in the approvals
// inbox is confirm-first by definition, so an unmapped kind still reads
// "confirm" — the map exists to surface the ORIGINATING tool + its catalog tier.
export const KIND_TO_VERB: Record<string, string> = {
  send_email: "send_email",
  advance_deal: "progress_deal",
  promote_lead: "promote_lead",
  coldstart: "enrich",
};

export function dotTier(tier: string | undefined): "auto" | "confirm" {
  return tier === "auto_execute" ? "auto" : "confirm";
}

export function approvalDotTier(
  kind: string,
  map: VerbTierMap,
): "auto" | "confirm" {
  const verb = KIND_TO_VERB[kind];
  return dotTier(verb ? map[verb] : undefined);
}

// verbTier reads the live catalog tier for an action verb (e.g. "progress_deal").
export function verbTier(verb: string, map: VerbTierMap): "auto" | "confirm" {
  return dotTier(map[verb]);
}

export function useAgentTierMap(): VerbTierMap {
  const query = useQuery({
    queryKey: ["agent-tools"],
    staleTime: 5 * 60_000,
    queryFn: async () => {
      return unwrap(await api.GET("/agent-tools"));
    },
  });
  return tierMapOf(query.data?.data ?? []);
}

// tierMapOf keys each tool's tier by its name, and each router action's tier by
// the verb it runs. A verb behind a router keeps its own tier.
export function tierMapOf(
  tools: readonly {
    name: string;
    tier: TierName;
    actions?: readonly { verb: string; tier: TierName }[];
  }[],
): VerbTierMap {
  const map: VerbTierMap = {};
  for (const tool of tools) {
    map[tool.name] = tool.tier;
    for (const action of tool.actions ?? []) {
      map[action.verb] = action.tier;
    }
  }
  return map;
}
