// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { NO_URL_PARAMS, type UrlParams } from "../../lib/urlstate";

// Which Brief the reader is looking at: the day, or the week. The view is in
// the address, because readers return to the Brief and send it to each other.
// A link that could not name its view would open the wrong one.

/** Morning is what waits today; Weekly is the week closed and the week ahead. */
export const VIEWS = ["morning", "weekly"] as const;
export type BriefView = (typeof VIEWS)[number];

export type BriefAddress = Readonly<{
  view: BriefView;
  week?: string;
  /** The team whose week Weekly shows; absent is the reader's own week. */
  team?: string;
}>;

/** The address a reader who has chosen nothing is at. */
export const DEFAULT_ADDRESS: BriefAddress = { view: "morning" };

const VIEW_PARAM = "view";
const WEEK_PARAM = "week";
const TEAM_PARAM = "team";
// Written by older builds, which had a Mine/Team switch. Read as nothing, and
// cleared on the next write.
const RETIRED_PARAMS = ["scope"];

/**
 * Read the address. An unknown view falls back to the default rather than
 * erroring: an address is something a link can carry from an older build.
 */
export function addressFrom(params: UrlParams): BriefAddress {
  const view = asView(params.get(VIEW_PARAM));
  const week = params.get(WEEK_PARAM);
  // A team only names a week, so Morning reads no team.
  const team = view === "weekly" ? params.get(TEAM_PARAM) : undefined;
  return { view, ...(week ? { week } : {}), ...(team ? { team } : {}) };
}

/**
 * Write the address, omitting whatever is already the default.
 *
 * `#/home` and `#/home?view=morning` are the same page, and the shorter one is
 * what a reader who has changed nothing should be able to copy. The writer
 * sorts its keys (hashWithParams), so the same state is always the same string.
 */
export function paramsFor(
  address: BriefAddress,
  existing: UrlParams = NO_URL_PARAMS,
): UrlParams {
  const params = new Map(existing);
  for (const key of [VIEW_PARAM, WEEK_PARAM, TEAM_PARAM, ...RETIRED_PARAMS])
    params.delete(key);
  if (address.view !== DEFAULT_ADDRESS.view) {
    params.set(VIEW_PARAM, address.view);
  }
  if (address.week) params.set(WEEK_PARAM, address.week);
  if (address.team && address.view === "weekly")
    params.set(TEAM_PARAM, address.team);
  return params;
}

function asView(raw: string | undefined): BriefView {
  return VIEWS.find((view) => view === raw) ?? DEFAULT_ADDRESS.view;
}
