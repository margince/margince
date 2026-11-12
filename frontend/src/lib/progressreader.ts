// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import { useMe } from "./session";

type AnalyticsContext = components["schemas"]["AnalyticsContext"];

/**
 * Whether Progress opens on a team for this reader. A manager's default scope
 * is wider than their own, and a team is among the scopes they may read.
 * The sidebar, the top bar and the page title name it from this one rule.
 */
export function readsTeamProgress(
  context: AnalyticsContext | undefined,
): boolean {
  return (
    context !== undefined &&
    context.default_scope.kind !== "owner" &&
    context.allowed_scopes.some((scope) => scope.kind === "team")
  );
}

export function useReadsTeamProgress(): boolean {
  return readsTeamProgress(useMe(false).data?.analytics_startup?.context);
}
