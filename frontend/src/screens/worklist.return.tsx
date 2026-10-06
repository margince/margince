// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { ArrowLeft } from "lucide-react";
import { createContext, type ReactNode, useContext } from "react";
import {
  navigate,
  parseHash,
  type Route,
  routeHash,
  type Screen,
} from "../app/router";
import {
  currentParams,
  hashWithParams,
  parseParams,
  type UrlParams,
  useUrlParams,
} from "../app/urlstate";
import { useT } from "../i18n";
import { type ContactTab, contactTabRoute } from "./contacttab";
import { WORKLIST_FILTER_PARAM } from "./worklist.header";

// The way back from a record to the Worklist drawer it was opened from.
//
// The drawer lives on Home's address, so opening a record from it leaves that
// address and the drawer with it. The record's own address carries the
// drawer's dials instead, and the record head turns them into a link back.

const FROM_PARAM = "from";
const FROM_WORKLIST = "worklist";
const DRAWER_DIALS = [WORKLIST_FILTER_PARAM, "queue_scope", "owner"] as const;

// The records whose head draws the way back. Any other destination a row
// links to (a settings queue, a privacy case) gets an untouched address, so
// its own dials are never shadowed by the drawer's.
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
 * Move to another tab of the record on screen. Every in-record tab move goes
 * through here so none of them drops the way back.
 */
export function navigateWithinRecord(route: Route): void {
  navigate(route, keptWorklistReturn());
}

/** One tab of the contact on screen, through the same move. */
export function openContactTab(id: string, tab: ContactTab): void {
  navigateWithinRecord(contactTabRoute(id, tab));
}

/** `href` with the drawer's dials added, when it addresses a returning record. */
export function withWorklistReturn(
  href: string,
  drawer: UrlParams | null,
): string {
  if (!drawer) {
    return href;
  }
  const route = parseHash(href);
  if (!route.id || !RETURNING_SCREENS.has(route.screen)) {
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

/** Home with the drawer open on the dials the record carried back. */
export function worklistReturnHref(carried: UrlParams): string {
  const dials = new Map(carried);
  dials.delete(FROM_PARAM);
  dials.set("queue", "1");
  return hashWithParams(routeHash({ screen: "home" }), dials);
}

const DrawerDials = createContext<UrlParams | null>(null);

/** Marks its rows as opened from the drawer, whose dials are the address's. */
export function WorklistReturnScope({
  children,
}: Readonly<{ children: ReactNode }>) {
  const [params] = useUrlParams();
  return <DrawerDials.Provider value={params}>{children}</DrawerDials.Provider>;
}

/** The drawer's dials when the caller is drawn inside it, null anywhere else. */
export function useDrawerDials(): UrlParams | null {
  return useContext(DrawerDials);
}

/** "Back to Worklist" on a record opened from the drawer; nothing otherwise. */
export function WorklistReturnLink() {
  const t = useT();
  const [params] = useUrlParams();
  const carried = worklistReturn(params);
  if (!carried) {
    return null;
  }
  return (
    <a className="link-button" href={worklistReturnHref(carried)}>
      <ArrowLeft aria-hidden="true" />
      {t("brief.queue.back")}
    </a>
  );
}
