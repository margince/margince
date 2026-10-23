// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Route } from "./router";

// Full evidence opens in the existing search reader, with its own access check.
export function searchEmailRoute(query: string, activityId: string): Route {
  return { screen: "search", id: querySegment(query), id2: activityId };
}

// The query as a path segment. routeHash joins segments as given and the
// search route decodes `id` once, so a "/" or "%" in a query must arrive encoded.
function querySegment(query: string): string {
  return encodeURIComponent(query);
}

// The third segment that opens the activity panel rather than the email one.
// The email address keeps its two segments, so a link to a message still works.
export const SEARCH_ACTIVITY_SEGMENT = "activity";

/** Where a meeting, call, note or task hit goes: the results, with it open. */
export function searchActivityRoute(query: string, activityId: string): Route {
  return {
    screen: "search",
    id: querySegment(query),
    id2: activityId,
    id3: SEARCH_ACTIVITY_SEGMENT,
  };
}
