import { useQuery } from "@tanstack/react-query";
import { useCallback, useMemo, useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { throwProblem } from "./common";

export type AnalyticsContext = components["schemas"]["AnalyticsContext"];
export type AnalyticsScope = components["schemas"]["AnalyticsScope"];

/**
 * What every Analytics request is about: the period asked for and the
 * population it is asked over.
 *
 * One object rather than two pieces of state, because it is one question. A
 * screen that kept them apart would refetch the cards on a period change and
 * leave one of them answering about a different population.
 */
export type AnalyticsSelection = {
  readonly scope: AnalyticsScope;
};

const CONTEXT_KEY = ["analytics-context"] as const;

/**
 * The caller's frame, read once.
 *
 * The server decides which population this person measures by default and
 * which ones they may choose; nothing here recreates that policy. A screen
 * asking the question itself would be a second answer, and the two would
 * disagree the first time somebody's grants changed.
 */
export function useAnalyticsContext() {
  return useQuery({
    queryKey: CONTEXT_KEY,
    queryFn: async () => {
      const { data, error } = await api.GET("/analytics/context", {});
      if (error) {
        throwProblem(error);
      }
      return data;
    },
  });
}

/**
 * Holds what the reader has selected, starting from the server's default.
 *
 * The scope is part of every query key, so changing it refetches rather than
 * showing the previous population's numbers under the new label.
 */
export function useAnalyticsSelection(context: AnalyticsContext | undefined) {
  const [chosen, setChosen] = useState<AnalyticsScope | null>(null);

  const selection = useMemo<AnalyticsSelection | null>(() => {
    if (!context) {
      return null;
    }
    return { scope: chosen ?? context.default_scope };
  }, [context, chosen]);

  const selectScope = useCallback((scope: AnalyticsScope) => {
    setChosen(scope);
  }, []);

  return { selection, selectScope };
}

/**
 * The scope as the wire spells it.
 *
 * `managed_teams` is the server's word for a manager's own default and names no
 * single subject, so it is sent as an OMISSION — which is exactly what it means:
 * the caller named nothing and the server resolved their lens. Sending it back
 * as a kind would be asking for somebody's managed set by name.
 */
export function scopeQuery(scope: AnalyticsScope): {
  scope_kind?: "workspace" | "team" | "owner";
  scope_id?: string;
} {
  if (scope.kind === "managed_teams") {
    return {};
  }
  if (scope.kind === "workspace") {
    return { scope_kind: "workspace" };
  }
  return { scope_kind: scope.kind, scope_id: scope.id };
}

/**
 * The scope a WRITE names, or null when the selection cannot be written to.
 *
 * A read may omit the scope and let the server resolve the reader's lens. A
 * write may not: a forecast or a share is an assertion about one named
 * population, and `managed_teams` covers several. Rather than pick one team on
 * the caller's behalf — a narrower claim than the number they are looking at —
 * this returns null and the surface withholds the action.
 */
export function writableScope(
  scope: AnalyticsScope,
): { scope_kind: "workspace" | "team" | "owner"; scope_id?: string } | null {
  if (scope.kind === "managed_teams") {
    return null;
  }
  if (scope.kind === "workspace") {
    return { scope_kind: "workspace" };
  }
  return { scope_kind: scope.kind, scope_id: scope.id };
}

/**
 * A stable key for the selected population, for query caches.
 *
 * Built from kind and id rather than the label: two teams could be renamed to
 * the same words, and a cache that collapsed them would serve one team's
 * numbers under the other's name.
 */
export function scopeKey(scope: AnalyticsScope): string {
  return scope.id ? `${scope.kind}:${scope.id}` : scope.kind;
}
