// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Route, Screen } from "../lib/router";

// Which ids name a page rather than a record. Imports nothing that draws, so
// bootstrap.ts can ask it before the shell loads.

// Each on its own screen only: `#/leads/import` is the lead whose id is "import".
export const CREATE_ID = "new";
export const IMPORT_ID = "import";

/** The screens whose list opens a create form at `CREATE_ID`, which `C` opens. */
export const CREATE_SCREENS: ReadonlySet<Screen> = new Set([
  "contacts",
  "companies",
  "leads",
  "deals",
  "projects",
]);

/** `#/deals/new`: the list itself, with its create form open. */
export function opensCreateForm(route: Route): boolean {
  return route.id === CREATE_ID && CREATE_SCREENS.has(route.screen);
}

/** A page of its own below a screen's list, by the id that names it. */
export const RESERVED_PAGE_IDS = {
  contacts: IMPORT_ID,
} as const satisfies Partial<Record<Screen, string>>;

export type ReservedPageScreen = keyof typeof RESERVED_PAGE_IDS;

function hasReservedPage(screen: Screen): screen is ReservedPageScreen {
  return Object.hasOwn(RESERVED_PAGE_IDS, screen);
}

/** The screen whose reserved page `route` opens, if it opens one. */
export function reservedPageOf(route: Route): ReservedPageScreen | undefined {
  const { screen } = route;
  return hasReservedPage(screen) && RESERVED_PAGE_IDS[screen] === route.id
    ? screen
    : undefined;
}

export function opensReservedPage(route: Route): boolean {
  return opensCreateForm(route) || reservedPageOf(route) !== undefined;
}
