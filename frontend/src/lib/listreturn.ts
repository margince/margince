import { stepsBackOutOf } from "./historydepth";
import { parseHash, type Route, routeHash, type Screen } from "./router";
import { GLOBAL_DIALS, hashWithParams, parseParams } from "./urlstate";

// The address each list screen last stood at in this page load, filter
// included. Memory, not storage: after a reload the parent is the plain list.
const lastListAddress = new Map<Screen, string>();

/**
 * Note the address the reader is at, if it is a list.
 *
 * The Ask dialog's dials are dropped. They stand over whatever list is open,
 * and a way back must not reopen a question the reader closed.
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

/**
 * Forget every remembered list, the way a reload does.
 *
 * The map above is page-load memory, and a test file is one page load for every
 * case in it. Without this, a case that leaves a narrowed list hands the next
 * one a parent it never visited. A record opened cold leads back to someone
 * else's filter.
 */
export function forgetListAddresses(): void {
  lastListAddress.clear();
}

/** The list a record of `screen` leads back to, narrowed as the reader left it. */
export function listAddress(screen: Screen): string {
  return lastListAddress.get(screen) ?? routeHash({ screen });
}

/**
 * Leave a record for the screen before it.
 *
 * The installed app has no browser Back, so this is the only way out. The
 * record's own tabs are views of one page, so they are stepped over. With an
 * earlier screen of ours in history, history is the truest answer, because it
 * also brings back the list's filter and scroll offset. A record with none of
 * ours behind it goes to its list instead of out of the app.
 */
export function leaveRecord(record: Route): void {
  const steps = stepsBackOutOf((hash) => {
    const route = parseHash(hash);
    return route.screen === record.screen && route.id === record.id;
  });
  if (steps > 0) {
    globalThis.history.go(-steps);
    return;
  }
  globalThis.location.hash = listAddress(record.screen);
}
