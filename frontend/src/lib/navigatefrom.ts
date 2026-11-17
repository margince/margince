// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { depthBackOutOf, hashAt, traverseTo } from "./historydepth";
import { routeIdentity } from "./routeidentity";
import {
  navigate,
  navigateReplacing,
  parseHash,
  type Route,
  routeHash,
  samePage,
} from "./router";
import type { UrlParams } from "./urlstate";

/**
 * A move to make once an action's work is done, from the address the action
 * started on. A move happens only while the reader is still on that page; a
 * dial turned on it is not leaving. A reader who left during the work keeps
 * the page they chose, and the work and its toast still land.
 */
export type Departure = Readonly<{
  stillHere: () => boolean;
  navigate: (route: Route, dials?: UrlParams) => void;
  navigateReplacing: (route: Route, dials?: UrlParams) => void;
  // Leaves a record the work closed. Another tab of that record is not
  // leaving, because the record is gone from every tab.
  closeOnto: (route: Route, dials?: UrlParams) => void;
}>;

/**
 * Notes where the reader is now. Call it from the action itself (the click,
 * the `mutate`, a mutation's `onMutate`). Never call it from render, which can
 * be older than the address the reader acted on.
 */
export function fromHere(): Departure {
  const origin = globalThis.location.hash;
  const stillHere = () => samePage(globalThis.location.hash, origin);
  const screen = (hash: string) => routeIdentity(parseHash(hash));
  const stillOnScreen = () =>
    screen(globalThis.location.hash) === screen(origin);
  return {
    stillHere,
    navigate: (route, dials) => {
      if (stillHere()) navigate(route, dials);
    },
    navigateReplacing: (route, dials) => {
      if (stillHere()) navigateReplacing(route, dials);
    },
    closeOnto: (route, dials) => {
      if (stillOnScreen()) closeOnto(route, dials);
    },
  };
}

/**
 * Close what is on screen onto `route`. When the nearest earlier entry outside
 * this screen's own tabs is that page, history goes back to it, dials as the
 * reader left them, so no entry is left that reopens what was closed.
 * Otherwise the entry on screen becomes `route`.
 */
export function closeOnto(route: Route, dials?: UrlParams): void {
  const target = routeHash(route);
  const screen = routeIdentity(parseHash(globalThis.location.hash));
  const at = depthBackOutOf(
    (hash) =>
      !samePage(hash, target) && routeIdentity(parseHash(hash)) === screen,
  );
  const behind = at === undefined ? undefined : hashAt(at);
  if (at !== undefined && behind !== undefined && samePage(behind, target)) {
    // Refused while any traversal is still in flight.
    traverseTo(at);
    return;
  }
  navigateReplacing(route, dials);
}
