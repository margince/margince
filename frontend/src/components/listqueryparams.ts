import { GLOBAL_DIALS, type UrlParams } from "../lib/urlstate";

/**
 * How long a typed filter settles before it becomes a request.
 *
 * Exported because it is a product decision about what a keystroke costs.
 * Any surface that turns typing into a server query reads it here. So the app
 * has one answer to "how responsive is a filter", instead of one per screen.
 */
export const SEARCH_DEBOUNCE_MS = 250;

export type ListQuery = {
  q: string;
  sort: string;
  includeArchived: boolean;
  filters: Record<string, string>;
  /**
   * Rows per page, chosen by the reader in the table footer. It is the size of
   * a rendered page, and a read is larger. One read fetches several of these
   * at once (see `listFetchLimit`), so the pager offers them with no wait.
   */
  perPage: number;
};

/** The page sizes the footer offers; the first is the default. */
export const LIST_PAGE_SIZES = [25, 50, 100] as const;

/**
 * The page size `value` names, or undefined for anything else.
 *
 * An address is text a reader can edit, so `per=7` and `per=banana` are both
 * ordinary. Neither is an error worth showing: the list opens at the size it
 * would have opened at. Neither may become a request either, because
 * `listFetchLimit` divides by it.
 */
function pageSizeOf(value: string | undefined): number | undefined {
  return LIST_PAGE_SIZES.find((size) => String(size) === value);
}

/**
 * The address prefix a list uses when it is not the only one on its route.
 *
 * Two lists on one route share one address, and the flat vocabulary below then
 * describes both at once. The settings Data-model tab draws the product table
 * and the offer-template table side by side. Sorting products by `sku` sent
 * `sort=sku` to `/offer-templates` (a 422 the reader never asked for). The
 * product `active` chip narrowed the templates read as well. A scope names
 * every dial the list owns (`products.q`, `products.sort`), so the two lists
 * on one route hold two parameter spaces.
 *
 * Undefined, and the dials keep their bare names. A bare `#/companies?q=acme`
 * is a link readers paste. Prefixing every list in the product would break
 * every one of them for no reader's benefit. A scope is only for lists that
 * share an address.
 *
 * The two spaces are told apart by the dot, so a wire filter may not contain
 * one. A scoped list reads only the keys carrying its own prefix. An unscoped
 * list reads only the keys carrying no prefix at all. Neither writes over the
 * other's.
 */
function scoped(scope: string | undefined, name: string): string {
  return scope ? `${scope}.${name}` : name;
}

/**
 * Is `key` a dial of the list at `scope`, rather than another list's?
 *
 * A global dial is nobody's: it belongs to something standing over the page
 * (see `GLOBAL_DIALS`). So it is neither read as this list's filter nor
 * overwritten when this list rewrites its own dials.
 */
function ownedBy(key: string, scope: string | undefined): boolean {
  if (GLOBAL_DIALS.has(key)) {
    return false;
  }
  return scope ? key.startsWith(`${scope}.`) : !key.includes(".");
}

/**
 * `live` with every dial the list at `scope` owns replaced by `mine`.
 *
 * A list writes its whole state at once. Without this, the second list on a
 * route would erase the first one's dials with every keystroke. For a list that
 * owns the address alone this is the plain replacement it always was, since
 * every key is then its own.
 */
export function replaceOwn(
  live: UrlParams,
  mine: UrlParams,
  scope: string | undefined,
): UrlParams {
  const next = new Map<string, string>();
  for (const [key, value] of live) {
    if (!ownedBy(key, scope)) {
      next.set(key, value);
    }
  }
  for (const [key, value] of mine) {
    next.set(key, value);
  }
  return next;
}

/**
 * The list's own dials, and the names they go by in the address.
 *
 * `q`, `sort` and `include_archived` are the wire's own spellings. A filter
 * chip's key is already a wire param name, since `...query.filters` is spread
 * onto the request. So the address and the endpoint describe a narrowed list
 * in one vocabulary, instead of two that must be kept in step.
 * Every other key in the address is therefore a filter, and no list of them is
 * kept here. A screen that adds a chip gets an addressable one for free. A
 * table of known filters would be a second copy of every screen's chip set.
 *
 * `per` is the exception and has no wire spelling, because a rendered page size
 * is a choice about drawing rather than about which rows exist. It is the
 * list's own name, which is why a filter param may not be called `per`.
 */
// Which rendered page is on screen, under the list's own name. The server is
// paged by keyset cursor. A page number is a slice of rows already fetched
// (see listFetchLimit), so the server is never asked for it.
// A stale cursor in an address would 422; a stale page number shows the
// last page there is.
const PAGE_PARAM = "page";

/**
 * The rendered page `params` names, or 1 for an address naming none or junk.
 *
 * Exported for the one screen that composes this codec by hand, outside
 * `useListQuery`. The deals screen drives a board and a table from a single
 * query. It reaches for these pieces instead of growing a second answer to
 * what a narrowed list's address looks like.
 */
export function listPageOf(params: UrlParams, scope?: string): number {
  const asked = Number(params.get(scoped(scope, PAGE_PARAM)));
  return Number.isInteger(asked) && asked > 1 ? asked : 1;
}

/** `params` with the rendered page set, page one being spelled by absence. */
export function withListPage(
  params: UrlParams,
  page: number,
  scope?: string,
): UrlParams {
  const next = new Map(params);
  const name = scoped(scope, PAGE_PARAM);
  if (page > 1) {
    next.set(name, String(page));
  } else {
    next.delete(name);
  }
  return next;
}

const PER_PAGE_PARAM = "per";

/** A screen with no drawing dial of its own, which is most of them. */
export const NO_SCREEN_DIALS: readonly string[] = [];

const LIST_OWN_PARAMS: ReadonlySet<string> = new Set([
  "q",
  "sort",
  "include_archived",
  PER_PAGE_PARAM,
  PAGE_PARAM,
]);

/**
 * This list's dials in `params`, keyed by their bare names.
 *
 * One reading for both spellings: a scoped list's `products.q` and an unscoped
 * list's `q` arrive here as `q`. So everything below is written once against
 * the vocabulary, rather than twice against two prefixes. Another list's keys
 * are left out of the result. That keeps the second table on a route from
 * reading the first one's filters as its own.
 */
export function ownDials(
  params: UrlParams,
  scope: string | undefined,
): Map<string, string> {
  const mine = new Map<string, string>();
  const prefix = scope ? `${scope}.` : "";
  for (const [key, value] of params) {
    if (ownedBy(key, scope)) {
      mine.set(key.slice(prefix.length), value);
    }
  }
  return mine;
}

/**
 * The address keys a screen owns, held out of the list's parameter space.
 *
 * The codec treats every key it does not recognise as a wire filter. That is
 * the right default: it is how a chip a screen adds becomes addressable with
 * no code here. A screen's own drawing dial is the exception. Board or table,
 * and which pipeline's board, decide how the same rows are presented. They say
 * nothing about which rows exist. Left in the parameter space, such a dial was
 * spread onto the request (`GET /leads?view=board` went out). It was counted
 * as a narrowing and wiped by "clear filters", which flipped the board back to
 * a table.
 *
 * One pair, because there were two. The deals board grew its own copies of
 * these two functions, and the leads queue did not grow them at all. That is
 * the difference between the two screens' defects. A screen names its drawing
 * dials once and the codec holds them apart.
 */
export function withoutScreenDials(
  params: UrlParams,
  dials: readonly string[],
): UrlParams {
  if (dials.length === 0) {
    return params;
  }
  const rest = new Map(params);
  for (const dial of dials) {
    rest.delete(dial);
  }
  return rest;
}

/**
 * `listDials` with the screen's own dials carried over from `carrying`.
 *
 * A dial the list dials already name wins over the live address. A saved deal
 * view records the pipeline it was saved on. Applying one writes that pipeline
 * through the list's own filter path. Carrying the live address over the top
 * of it left the reader looking at another board's stages, with the saved view
 * highlighted.
 */
export function mergeScreenDials(
  listDials: UrlParams,
  carrying: UrlParams,
  dials: readonly string[],
): UrlParams {
  if (dials.length === 0) {
    return listDials;
  }
  const merged = new Map(listDials);
  for (const dial of dials) {
    const value = carrying.get(dial);
    if (value && !merged.has(dial)) {
      merged.set(dial, value);
    }
  }
  return merged;
}

/**
 * The value an opening filter carries once the reader has removed it.
 *
 * A list that opens narrowed (the leads queue opens on "Mine") cannot spell
 * "All" by absence. Back mounts the list afresh, and a fresh list reads a bare
 * address as "just arrived" and narrows it again. So the removal is written out
 * (`owner_id=all`). It is read as a removal only for a key the opening state
 * holds, so a filter whose real vocabulary has an "all" keeps it.
 */
const CLEARED_OPENING_FILTER = "all";

export function isClearedFilterMarker(value: string): boolean {
  return value === CLEARED_OPENING_FILTER;
}

function isClearedOpening(
  key: string,
  value: string,
  opening: ListQuery,
): boolean {
  return isClearedFilterMarker(value) && key in opening.filters;
}

/**
 * The query `params` describes, with `opening` standing where it says nothing.
 *
 * An address that carries any dial at all is the whole truth about the list.
 * Filters are replaced rather than merged. A reader who cleared the owner chip
 * and shared the link must not have it put back by the screen's default.
 * `opening` therefore only stands in for an address that says nothing at all.
 * It does so only until this list has written its own opening state there.
 */
export function listQueryFromParams(
  params: UrlParams,
  opening: ListQuery,
  /**
   * Whether this list has already written its opening state to the address.
   *
   * Until it has, a bare address means "arrived now" and the screen's own
   * answers apply. Once it has, a bare address means the reader cleared
   * everything, which is a different list. Some screens open pre-narrowed
   * (the leads queue opens on a rep's own leads). There, this is the whole
   * behaviour of the owner chip's "all".
   */
  seeded: boolean,
  /** The address prefix this list's dials carry; see `scoped`. */
  scope?: string,
): ListQuery {
  const mine = ownDials(params, scope);
  const filters: Record<string, string> = {};
  for (const [key, value] of mine) {
    if (LIST_OWN_PARAMS.has(key) || isClearedOpening(key, value, opening)) {
      continue;
    }
    filters[key] = value;
  }
  // An address carrying a dial, or a reader who has turned one, has expressed
  // what the list should be. Only a reader who has expressed nothing at all
  // gets the screen's opening answers.
  //
  // Count this list's dials, and ignore the address's size. On a route holding
  // two lists the other one seeds first. Counting its keys would tell this
  // list that a reader had cleared filters they never saw.
  const expressed = seeded || mine.size > 0;
  return {
    q: mine.get("q") ?? "",
    // No sort in an expressed address means the server's own order, and the
    // screen's order is a different list. A saved view that names no sort asks
    // for the server's order. Absence can only spell one of them, so the
    // writer below spells a sort out even when it is the default one.
    sort: mine.get("sort") ?? (expressed ? "" : opening.sort),
    includeArchived: mine.get("include_archived") === "true",
    filters: expressed ? filters : opening.filters,
    perPage: pageSizeOf(mine.get(PER_PAGE_PARAM)) ?? opening.perPage,
  };
}

/**
 * The address `query` produces: every dial that is away from its default.
 *
 * A default left out is what keeps a shared link about what the reader chose.
 * `#/companies?q=acme` says one thing. The same address carrying the page size
 * and `include_archived=false` says the same thing while hiding it. It invites
 * the next reader to think three dials were turned.
 *
 * Sort is the exception, and is written even when it is the screen's own. An
 * unsorted list is a state a reader can reach. A saved view that names no sort
 * asks for the server's order rather than this screen's. Absence cannot spell
 * both that and "whatever this list opens in". So absence is kept for the one
 * that has no other spelling, and the ordinary sort says its name.
 */
export function paramsFromListQuery(
  query: ListQuery,
  opening: ListQuery,
  /** The address prefix this list's dials carry; see `scoped`. */
  scope?: string,
): UrlParams {
  const params = new Map<string, string>();
  const set = (name: string, value: string) =>
    params.set(scoped(scope, name), value);
  for (const [key, value] of Object.entries(query.filters)) {
    set(key, value);
  }
  for (const key of Object.keys(opening.filters)) {
    if (!(key in query.filters)) {
      set(key, CLEARED_OPENING_FILTER);
    }
  }
  if (query.q) {
    set("q", query.q);
  }
  if (query.sort) {
    set("sort", query.sort);
  }
  if (query.includeArchived) {
    set("include_archived", "true");
  }
  if (query.perPage !== opening.perPage) {
    set(PER_PAGE_PARAM, String(query.perPage));
  }
  return params;
}

/** The most rows one list read may ask for (the contract's CAP-PAGE ceiling). */
const MAX_ROWS_PER_READ = 200;

/**
 * How many rows to fetch for a footer page size of `perPage`: as many whole
 * rendered pages as one read is allowed to carry.
 *
 * A keyset cursor can only walk forward one read at a time, so a pager can
 * only number the pages already in hand. Reading a page at a time numbers one
 * page, and every further number costs a round trip. That made a second page
 * appear only after the reader had been told none existed. Reading in whole
 * multiples puts several numbered pages in hand at once. Each is a slice of
 * one response, so they also form one consistent snapshot. No row shows up
 * twice and none is skipped between them.
 *
 * Whole multiples matter. A remainder would leave a last page shorter than the
 * size the footer names. A request above the ceiling is clamped with no error,
 * so the pager would offer numbers the response has no rows for.
 */
export function listFetchLimit(perPage: number): number {
  return Math.max(1, Math.floor(MAX_ROWS_PER_READ / perPage)) * perPage;
}
