import { useQuery } from "@tanstack/react-query";
import { useCallback, useMemo } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { unwrap } from "../../lib/problem";
import { replaceDial, useUrlParams } from "../../lib/urlstate";

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
 * The server decides which population this colleague measures by default and
 * which ones they may choose; nothing here recreates that policy. A screen
 * asking the question itself would be a second answer, and the two would
 * disagree the first time somebody's grants changed.
 */
export function useAnalyticsContext() {
  return useQuery({
    queryKey: CONTEXT_KEY,
    queryFn: async () => {
      return unwrap(await api.GET("/analytics/context", {}));
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
  const [params] = useUrlParams();
  const chosen = context?.allowed_scopes.find(
    (scope) => scopeKey(scope) === params.get("scope"),
  );

  const selection = useMemo<AnalyticsSelection | null>(() => {
    if (!context) {
      return null;
    }
    return { scope: chosen ?? context.default_scope };
  }, [context, chosen]);

  const selectScope = useCallback((scope: AnalyticsScope) => {
    replaceDial("scope", scopeKey(scope));
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

/** The same scope as a prebuilt report's body names it. */
export function reportScope(
  scope: AnalyticsScope,
): components["schemas"]["ReportScope"] | undefined {
  const query = scopeQuery(scope);
  return query.scope_kind
    ? { kind: query.scope_kind, id: query.scope_id }
    : undefined;
}

/**
 * The scope a WRITE names, or null when the selection cannot be written to.
 *
 * A write names one population, so it cannot omit the scope the way a read can.
 * `managed_teams` covers several, and picking one would claim less than the
 * number on screen. A scope marked `assertable: false` is one the seat may read
 * and the write routes refuse. Both return null and the surface withholds the
 * action.
 */
export function writableScope(
  scope: AnalyticsScope,
): { scope_kind: "workspace" | "team" | "owner"; scope_id?: string } | null {
  if (scope.kind === "managed_teams" || scope.assertable === false) {
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
