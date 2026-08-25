import { useMemo } from "react";
import { announceAddressChanged, useHash } from "./router";

/**
 * The query half of the address: the state that names a VIEW of what is on
 * screen — how a list is narrowed, in what order, on which page — rather than
 * what is on screen, which is the path's job (app/router.tsx).
 *
 * This exists because none of it was addressable. A reader who filtered the
 * companies list, opened a company and pressed Back arrived at an unfiltered
 * list, and there was no way to send anybody a link to what they were looking
 * at. Holding it in `useState` beside the address is what made both true.
 *
 * A `ReadonlyMap` rather than `URLSearchParams`: the latter is mutable and
 * multi-value, and neither is true of a dial. One value each, and a snapshot a
 * caller can hold without it changing underneath — which is what lets a screen
 * DERIVE its query from the address instead of mirroring it into state that can
 * then disagree with the URL bar.
 */
export type UrlParams = ReadonlyMap<string, string>;

/** The address with no dials set. Shared, so an empty query is one object. */
export const NO_URL_PARAMS: UrlParams = new Map<string, string>();

/**
 * The dials `hash` carries.
 *
 * An empty value reads as ABSENT rather than as a dial set to nothing, so
 * `#/companies?q=` and `#/companies` mean the same thing. The writer below
 * never emits one; a person editing the address bar can, and the two spellings
 * answering differently is the kind of difference nobody can see.
 */
export function parseParams(hash: string): UrlParams {
  const query = hash.indexOf("?");
  if (query < 0) {
    return NO_URL_PARAMS;
  }
  const params = new Map<string, string>();
  for (const [key, value] of new URLSearchParams(hash.slice(query + 1))) {
    if (value) {
      // A repeated key is one dial written twice, and the last is what the
      // writer meant. Keeping the first would make a hand-edited address behave
      // differently from every address this module produces.
      params.set(key, value);
    }
  }
  return params;
}

/**
 * `hash` with `params` as its query and the path it already had.
 *
 * Keys are SORTED, so one set of dials has exactly one address: two readers who
 * chose the same filters in a different order get comparable links, and a write
 * that only reordered keys is recognisable as the no-op it is.
 */
/**
 * Key order for a written address, and it is BYTE order rather than the reader's.
 *
 * `localeCompare` is the usual advice for sorting strings and is wrong here: the
 * point of sorting at all is that one view of a list has ONE address, so two
 * readers who narrow the same list the same way can compare links and a rewrite
 * that only reordered keys is recognisable as the no-op it is. A locale-aware
 * order is a different order in a different locale, which would make the same
 * view produce different URLs for a German reader and an English one.
 */
function byKey(one: string, other: string): number {
  if (one === other) {
    return 0;
  }
  return one < other ? -1 : 1;
}

export function hashWithParams(hash: string, params: UrlParams): string {
  const query = hash.indexOf("?");
  const path = query < 0 ? hash : hash.slice(0, query);
  const search = new URLSearchParams();
  for (const key of [...params.keys()].sort(byKey)) {
    const value = params.get(key);
    if (value) {
      search.set(key, value);
    }
  }
  const dials = search.toString();
  // A bare document has no hash at all; the path has to be spelled or the
  // written address would be a query hanging off nothing.
  return dials ? `${path || "#/"}?${dials}` : path || "#/";
}

/**
 * The dials the address carries RIGHT NOW, outside a render.
 *
 * A render's params are a snapshot, and two writes in one event handler would
 * both build on it — the second silently discarding the first, because nothing
 * batches them the way React batches state. Pressing a saved view is that case:
 * it restores the sort, the filters, the archived toggle and the page size, and
 * a caller composing those on the snapshot would land the last one alone.
 */
export function currentParams(): UrlParams {
  return parseParams(globalThis.location.hash);
}

/**
 * Set the dials, OVERWRITING the current history entry.
 *
 * Replace rather than push, and the reason is what Back is for. A reader
 * narrowing a list turns several dials to reach one view; pushing each turn
 * would bury the screen they came from under a dozen entries, so Back — the one
 * key that exists for getting out of things — would stop getting them out. The
 * entry left behind therefore always describes the list as it stands, which is
 * exactly what makes Back from a record restore it.
 *
 * `history.replaceState` and not `location.replace`: the latter DISCARDS the
 * entry's state object. Nothing here keeps state in it, but app/scrollmemory.ts
 * stamps the entry's identity there, so every turn of a dial threw away the name
 * of the place the reader was standing in and the offset remembered under it
 * became unreachable. Passing the current state through keeps the entry the same
 * entry. app/router.tsx's `navigateReplacing` is the same write for the PATH
 * half, held to the same obligation by app/addressstate.test.ts.
 *
 * It costs the `hashchange` that `location.replace` would have fired, which is
 * why the announcement below is not an optimisation but the thing that makes
 * this work at all.
 */
export function replaceParams(params: UrlParams): void {
  const next = hashWithParams(globalThis.location.hash, params);
  if (next === globalThis.location.hash) {
    // Nothing moved. Replacing anyway would be a write per keystroke on a
    // search box that has settled back to where it started.
    return;
  }
  globalThis.history.replaceState(globalThis.history.state, "", next);
  // Told directly rather than waited for: `hashchange` arrives a task later, and
  // in that gap the list would still be reading the dials the reader has just
  // changed — one fetch for the previous query before the one they asked for.
  announceAddressChanged();
}

/**
 * The dials the address carries, and the one way to change them.
 *
 * The setter is module-level and therefore stable, so a caller may depend on it
 * without a memo and an effect keyed on it cannot re-run for a new identity.
 */
export function useUrlParams(): [UrlParams, (next: UrlParams) => void] {
  const hash = useHash();
  // Keyed on the whole address rather than on its query, because that is what
  // the store hands back: one string in, one params object out, stable for as
  // long as the reader stays put. A query object rebuilt every render would
  // re-key every react-query read that names it.
  const params = useMemo(() => parseParams(hash), [hash]);
  return [params, replaceParams];
}
