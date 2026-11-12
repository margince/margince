// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
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

// The frame Progress itself reads, so the name agrees with the page on any
// first screen. A seat with neither grant is refused the frame and is not asked.
export function useReadsTeamProgress(): boolean {
  const me = useMe(false).data;
  const context = useQuery({
    ...analyticsContextOptions,
    // Every page asks for its name; the name does not need a read per page.
    staleTime: 5 * 60_000,
    enabled: grants(me, "deal", "read") || grants(me, "forecast", "read"),
  });
  return readsTeamProgress(context.data);
}
