// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { entryDepth, hashAt, traverseTo } from "./historydepth";
import {
  navigate,
  navigateReplacing,
  parseHash,
  type Route,
  routeHash,
  samePage,
} from "./router";
import { hashWithParams, parseParams, type UrlParams } from "./urlstate";

/**
 * A move to make once an action's work is done, from the address the action
 * started on. Each move happens only while the reader is still on that page; a
 * dial turned on it is not leaving. A reader who left during the work keeps the
 * page they chose, and the work and its toast still land.
 */
export type Departure = Readonly<{
  stillHere: () => boolean;
  navigate: (route: Route, dials?: UrlParams) => void;
  navigateReplacing: (route: Route, dials?: UrlParams) => void;
}>;

/**
 * Notes where the reader is now. Call it from the action itself (the click,
 * the `mutate`, a mutation's `onMutate`), never from render: a render can be
 * older than the address the reader acted on.
 */
export function fromHere(): Departure {
  const origin = globalThis.location.hash;
  const stillHere = () => samePage(globalThis.location.hash, origin);
  return {
    stillHere,
    navigate: (route, dials) => {
      if (stillHere()) navigate(route, dials);
    },
    navigateReplacing: (route, dials) => {
      if (stillHere()) navigateReplacing(route, dials);
    },
  };
}

// One spelling per address, so `#/` and `#/home` or two dial orders compare equal.
const spelled = (hash: string): string =>
  hashWithParams(routeHash(parseHash(hash)), parseParams(hash));

/**
 * Close what is on screen onto `route`. When the entry behind is that address,
 * history goes back to it, so no entry is left that reopens what was closed.
 * Otherwise the entry on screen becomes that address.
 */
export function closeOnto(route: Route, dials?: UrlParams): void {
  const target = spelled(hashWithParams(routeHash(route), dials ?? new Map()));
  const depth = entryDepth();
  const behind = depth === undefined ? undefined : hashAt(depth - 1);
  if (
    depth !== undefined &&
    behind !== undefined &&
    spelled(behind) === target
  ) {
    // Refused only while an earlier close is still on its way there.
    traverseTo(depth - 1);
    return;
  }
  navigateReplacing(route, dials);
}
