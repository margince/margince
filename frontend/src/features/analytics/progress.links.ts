// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { MessageKey } from "../../i18n/en";
import { routeHash } from "../../lib/router";
import { useViewerId } from "../../lib/session";
import { hashWithParams } from "../../lib/urlstate";
import {
  type AnalyticsScope,
  scopeKey,
  useAnalyticsContext,
} from "./analytics.context";

type Scope = Pick<AnalyticsScope, "kind" | "id">;

/**
 * Progress on the population an Analytics figure is about, and what the link
 * is called. Progress reads one team member, or one team inside the reader's
 * own record access, so any other scope has no door.
 */
export function useProgressDoor(
  scope: Scope,
): Readonly<{ href: string; label: MessageKey }> | undefined {
  const viewer = useViewerId();
  const offered = useAnalyticsContext().data?.allowed_scopes.find(
    (each) => scopeKey(each) === scopeKey(scope),
  );
  if (!scope.id || (scope.kind !== "owner" && scope.kind !== "team"))
    return undefined;
  if (scope.kind === "team" && offered?.measurable !== true) return undefined;
  const href = hashWithParams(
    routeHash({ screen: "progress" }),
    new Map([["scope", scopeKey(scope)]]),
  );
  if (scope.kind === "team") return { href, label: "progress.teamTitle" };
  return {
    href,
    label: scope.id === viewer ? "nav.myProgress" : "progress.myTitle",
  };
}

/** Sales goals, on the team member a figure is about when it names one. */
export function goalsHref(scope: Scope): string {
  const dials = new Map<string, string>();
  if (scope.kind === "owner" && scope.id) dials.set("seller", scope.id);
  return hashWithParams(routeHash({ screen: "goals" }), dials);
}
