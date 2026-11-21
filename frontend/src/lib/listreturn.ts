import { stepBackOutOf, steppedBackOutOf } from "./historydepth";
import {
  parseHash,
  type Route,
  routeHash,
  type Screen,
  samePage,
} from "./router";
import {
  type AddressWriters,
  GLOBAL_DIALS,
  hashWithParams,
  parseParams,
} from "./urlstate";

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

// An address on `record`, whichever tab and dials it has open.
const onRecord = (record: Route) => (hash: string) => {
  const route = parseHash(hash);
  return route.screen === record.screen && route.id === record.id;
};

/**
 * Leave a record for the screen before it, which history restores with its
 * filter and scroll; with none of ours behind, the record's list.
 */
export function leaveRecord(record: Route): void {
  stepBackOutOf(onRecord(record), () => {
    globalThis.location.hash = listAddress(record.screen);
  });
}

/** Whether the bar already stands where leaving `record` leads: just behind it, or on its list. */
export function leftRecord(record: Route, bar: string): boolean {
  return (
    steppedBackOutOf(onRecord(record)) ||
    samePage(bar, listAddress(record.screen))
  );
}

/** Close a list's create form onto the list, replacing the form's address so
 *  Back from the list does not open the form again. */
export function leaveCreateForm(
  screen: Screen,
  { navigateReplacing }: AddressWriters,
): void {
  const here = parseHash(globalThis.location.hash);
  if (here.screen !== screen || here.id === undefined) {
    return;
  }
  const list = listAddress(screen);
  navigateReplacing(parseHash(list), parseParams(list));
}
