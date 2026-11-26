// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import { routeHash } from "./router";
import { hashWithParams } from "./urlstate";
import { viewOfFilter } from "./worklistviews";

type Worklist = components["schemas"]["Worklist"];
export const WORKLIST_FILTER_PARAM = "filter";
export const WORKLIST_VIEW_PARAM = "queue_view";

export function worklistLaneTarget(
  filter: NonNullable<Worklist["filter"]>,
  scope: NonNullable<Worklist["scope"]> = "mine",
) {
  const view = viewOfFilter(filter);
  const params = new Map<string, string>(
    view === "decisions" || view === "system"
      ? [[WORKLIST_VIEW_PARAM, view]]
      : [[WORKLIST_FILTER_PARAM, filter]],
  );
  params.set("queue", "1");
  if (scope !== "mine") params.set("queue_scope", scope);
  return { route: { screen: "home" } as const, params };
}

export function worklistLaneHref(
  filter: NonNullable<Worklist["filter"]>,
  scope: NonNullable<Worklist["scope"]> = "mine",
) {
  const target = worklistLaneTarget(filter, scope);
  return hashWithParams(routeHash(target.route), target.params);
}

/** The dial naming the part of the Worklist a link opens it on. */
export const WORKLIST_SECTION_PARAM = "queue_at";
/** The Change history panel, last on the Worklist. */
export const CHANGE_HISTORY_SECTION = "history";

/** The Worklist, opened on its Change history panel. */
export function worklistHistoryHref() {
  return hashWithParams(
    routeHash({ screen: "home" }),
    new Map([
      ["queue", "1"],
      [WORKLIST_SECTION_PARAM, CHANGE_HISTORY_SECTION],
    ]),
  );
}
