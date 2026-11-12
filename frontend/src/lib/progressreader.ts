// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect } from "react";
import type { components } from "../api/schema";
import { grants } from "./accessgrants";
import { analyticsContextOptions } from "./analyticscontext";
import { useMe } from "./session";

type AnalyticsContext = components["schemas"]["AnalyticsContext"];

/**
 * Whether Progress opens on a team for this reader. A manager's default scope
 * is wider than their own, and a team they may measure is among their scopes.
 * The sidebar, the top bar and the page title name it from this one rule.
 */
export function readsTeamProgress(
  context: AnalyticsContext | undefined,
): boolean {
  // An answer that is no frame, as a proxy's error page is, names the
  // reader's own figures. Every page asks, so it must not fail them all.
  const opensOn = context?.default_scope?.kind;
  return (
    context !== undefined &&
    opensOn !== undefined &&
    opensOn !== "owner" &&
    context.allowed_scopes.some(
      (scope) => scope.kind === "team" && scope.measurable !== false,
    )
  );
}

// How long the screen's own reads must stay quiet before the name asks.
const QUIET_MS = 300;
const NAME_STALE_MS = 5 * 60_000;

/**
 * Reads the frame Progress itself reads, once, after the first screen's own
 * reads have rested. It listens to the query cache instead of rendering, so
 * the wait costs the open screen no render. A seat with neither grant is
 * refused the frame and is not asked.
 */
export function useWarmProgressName(): void {
  const client = useQueryClient();
  const me = useMe(false).data;
  const asks = grants(me, "deal", "read") || grants(me, "forecast", "read");
  useEffect(() => {
    if (!asks) return;
    let handle: number | undefined;
    const stop = client.getQueryCache().subscribe(() => settle());
    const settle = () => {
      window.clearTimeout(handle);
      if (client.isFetching() > 0) return;
      handle = window.setTimeout(() => {
        stop();
        void client.prefetchQuery({
          ...analyticsContextOptions,
          staleTime: NAME_STALE_MS,
        });
      }, QUIET_MS);
    };
    settle();
    return () => {
      stop();
      window.clearTimeout(handle);
    };
  }, [client, asks]);
}

// The name reads the cache and never asks: useWarmProgressName fills it, and
// so does Analytics. The session's own frame names it until then.
export function useReadsTeamProgress(): boolean {
  const me = useMe(false).data;
  const context = useQuery({ ...analyticsContextOptions, enabled: false });
  return readsTeamProgress(context.data ?? me?.analytics_startup?.context);
}
