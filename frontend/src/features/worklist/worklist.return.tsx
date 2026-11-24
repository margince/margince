// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
import { type ContactTab, contactTabRoute } from "../../lib/contacttab";
import { RecordHrefScope } from "../../lib/recordhref";
import {
  navigate,
  parseHash,
  type Route,
  routeHash,
  type Screen,
} from "../../lib/router";
import {
  currentParams,
  hashWithParams,
  parseParams,
  type UrlParams,
  useUrlParams,
} from "../../lib/urlstate";
import {
  WORKLIST_FILTER_PARAM,
  WORKLIST_VIEW_PARAM,
} from "../../lib/worklistrouting";

// The way back from a record to the Worklist drawer it was opened from.
//
// The drawer lives on Home's address, so opening a record from it leaves that
// address and the drawer with it. The record's own address carries the
// drawer's dials instead, and the top bar's Back turns them into the way back.

const FROM_PARAM = "from";
const FROM_WORKLIST = "worklist";
const DRAWER_DIALS = [
  WORKLIST_VIEW_PARAM,
  WORKLIST_FILTER_PARAM,
  "queue_scope",
  "owner",
] as const;

// The records that keep the way back. Any other destination a row links to
// keeps an untouched address, so the drawer's dials never shadow its own.
const RETURNING_SCREENS: ReadonlySet<Screen> = new Set([
  "contacts",
  "companies",
  "deals",
  "leads",
]);

/** The drawer's dials plus the marker, when `params` says the record came from it. */
export function worklistReturn(params: UrlParams): UrlParams | undefined {
  if (params.get(FROM_PARAM) !== FROM_WORKLIST) {
    return undefined;
  }
  const carried = new Map([[FROM_PARAM, FROM_WORKLIST]]);
  for (const dial of DRAWER_DIALS) {
    const value = params.get(dial);
    if (value) carried.set(dial, value);
  }
  return carried;
}

/**
 * The way back as dials for a move WITHIN the record — a tab switch — so the
 * record keeps it however many tabs the reader opens.
 */
export function keptWorklistReturn(): UrlParams | undefined {
  return worklistReturn(currentParams());
}

/**
 * A move from a record to another tab of it, or to a record it opens, that
 * keeps the way back: every such move goes through here so none drops it.
 */
export function navigateKeepingReturn(route: Route): void {
  navigate(route, keptWorklistReturn());
}

/** One tab of the contact on screen, through the same move. */
export function openContactTab(id: string, tab: ContactTab): void {
  navigateKeepingReturn(contactTabRoute(id, tab));
}

/** The record `href` opens, when it is one that keeps the way back. */
export function returningRecord(href: string): Route | undefined {
  const route = parseHash(href);
  return route.id && RETURNING_SCREENS.has(route.screen) ? route : undefined;
}

/** `href` with the drawer's dials added, when it addresses a returning record. */
export function withWorklistReturn(href: string, drawer: UrlParams): string {
  if (!returningRecord(href)) {
    return href;
  }
  const merged = new Map(parseParams(href));
  merged.set(FROM_PARAM, FROM_WORKLIST);
  for (const dial of DRAWER_DIALS) {
    const value = drawer.get(dial);
    if (value) merged.set(dial, value);
  }
  return hashWithParams(href, merged);
}

/**
 * Home with the drawer open on the dials the record at `hash` carried, when
 * the record was opened from the drawer.
 */
export function worklistReturnHref(hash: string): string | undefined {
  const carried = returningRecord(hash) && worklistReturn(parseParams(hash));
  if (!carried) {
    return undefined;
  }
  const dials = new Map(carried);
  dials.delete(FROM_PARAM);
  dials.set("queue", "1");
  return hashWithParams(routeHash({ screen: "home" }), dials);
}

/**
 * Marks every record link under it as opened from the Worklist, the shared
 * components' included. The drawer's dials are the address's; a list drawn
 * elsewhere names its own.
 */
export function WorklistReturnScope({
  dials,
  children,
}: Readonly<{ dials?: UrlParams; children: ReactNode }>) {
  const [params] = useUrlParams();
  const drawer = dials ?? params;
  return (
    <RecordHrefScope href={(href) => withWorklistReturn(href, drawer)}>
      {children}
    </RecordHrefScope>
  );
}
