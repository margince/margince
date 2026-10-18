import { backStaysInApp, parseHash, routeHash, type Screen } from "./router";
import { GLOBAL_DIALS, hashWithParams, parseParams } from "./urlstate";

// The address each list screen last stood at in this page load, its filter
// query included. Memory rather than storage: a reload starts a new visit, and
// the plain list is the honest parent for a record opened cold.
const lastListAddress = new Map<Screen, string>();

/**
 * Note the address the reader is at, if it is a list.
 *
 * The Ask dialog's dials are dropped: they stand over whatever list is open,
 * and a way back to the list must not reopen a question the reader closed.
 */
export function rememberListAddress(hash: string): void {
  const route = parseHash(hash);
  if (route.id !== undefined) {
    return;
  }
  const dials = new Map(parseParams(hash));
  for (const key of GLOBAL_DIALS) {
    dials.delete(key);
  }
  lastListAddress.set(route.screen, hashWithParams(routeHash(route), dials));
}

/** The list a record of `screen` leads back to, narrowed as the reader left it. */
export function listAddress(screen: Screen): string {
  return lastListAddress.get(screen) ?? routeHash({ screen });
}

/**
 * Leave a record for the screen before it.
 *
 * The installed app has no browser Back, so this is the only way out. With an
 * earlier screen of ours in history, history is the truest answer, because it
 * also brings back the list's scroll offset. A record opened cold has none of
 * ours behind it, so it goes to its list instead of out of the app.
 */
export function leaveRecord(screen: Screen): void {
  if (backStaysInApp()) {
    globalThis.history.back();
    return;
  }
  globalThis.location.hash = listAddress(screen);
}
