import { useCallback, useEffect, useMemo, useRef } from "react";
import { routeHash } from "../app/router";
import { currentParams, type UrlParams, useUrlParams } from "../app/urlstate";
import { settingsHref } from "./settingsrouting";

/**
 * The address of ONE rights case, and the state that keeps a card's open row
 * and the URL bar saying the same thing.
 *
 * The worklist has named subject requests on their statutory clock since the
 * lane shipped, and its row links here — to the queue, not to the case. An
 * officer following it landed on a page of twenty rows with nothing saying
 * which one they had been sent to read, and the row they wanted was as likely
 * to be below the fold as on screen. Naming the case in the address is what
 * closes that gap.
 *
 * `case` rather than `dsr`: the address is read by people, and the queue calls
 * these cases in every sentence it draws.
 */
export const CASE_PARAM = "case";

/**
 * How far the queue chases a linked case before saying it is not here.
 *
 * Twenty rows a page, so this reaches two hundred — past every queue this
 * product has seen, and short of the unbounded walk an id naming no case would
 * otherwise start.
 */
const MAX_PAGES_CHASED = 10;

/** The address of the privacy queue with one case named. */
export function caseHref(id: string): string {
  return `${routeHash(settingsHref("privacy"))}?${CASE_PARAM}=${encodeURIComponent(id)}`;
}

/** The case the address names, or nothing. */
export function caseInParams(params: UrlParams): string | undefined {
  return params.get(CASE_PARAM);
}

/**
 * What the queue knows about the case it was sent to open.
 *
 * `absent` is the state worth the trouble. The list pages twenty at a time and
 * the linked case may be on none of the pages loaded so far, so a card that
 * merely seeded its expansion would leave the reader on an ordinary queue with
 * no sign their link had named anything — the exact hand-off the address was
 * added to remove. The card says so instead, and keeps saying it until more
 * pages arrive or the reader opens another row.
 */
export type LinkedCase =
  | { readonly kind: "none" }
  | { readonly kind: "loading"; readonly id: string }
  | { readonly kind: "shown"; readonly id: string }
  | { readonly kind: "absent"; readonly id: string };

/**
 * The open row, and the address, as one value.
 *
 * The reader can arrive with a case named, open a different one, close it, or
 * page further into the queue until the named one loads. Every one of those
 * moves has to leave the URL describing what is on screen, or the link the
 * officer copies out is not the case they are looking at.
 *
 * Opening a row REPLACES the history entry rather than pushing one, the same
 * choice app/urlstate.ts makes for every other dial: a reader who opened four
 * cases in a row still has one Back out of the queue.
 */
export function useLinkedCase(
  loadedIds: readonly string[],
  moreToLoad: boolean,
  loadMore?: () => void,
): {
  readonly expandedId: string | null;
  readonly linked: LinkedCase;
  readonly toggle: (id: string) => void;
} {
  const [params, setParams] = useUrlParams();
  // THE ADDRESS IS THE STATE, and there is deliberately no second copy of it
  // beside this. An open row mirrored into `useState` is a value that can
  // disagree with the URL bar — the reader closes a row and the link they copy
  // out still names it, or they follow a second link and the first row stays
  // open — and every one of those is invisible until somebody pastes the
  // address to a colleague. Writing the dial and reading it back is what makes
  // those states unrepresentable rather than merely tested for.
  const expandedId = caseInParams(params) ?? null;

  const loaded = useMemo(() => new Set(loadedIds), [loadedIds]);
  // The count is stamped with the case it belongs to, so a second link resets
  // it by naming a different case rather than through an effect that watches
  // for the change.
  const pagesPulled = useRef<{ id: string; pages: number }>({
    id: "",
    pages: 0,
  });
  const pagesFor = (id: string) =>
    pagesPulled.current.id === id ? pagesPulled.current.pages : 0;

  const linked: LinkedCase = !expandedId
    ? { kind: "none" }
    : loaded.has(expandedId)
      ? { kind: "shown", id: expandedId }
      : moreToLoad && pagesFor(expandedId) < MAX_PAGES_CHASED
        ? { kind: "loading", id: expandedId }
        : { kind: "absent", id: expandedId };

  // FETCH FORWARD until the case arrives, rather than telling the reader to
  // press Load more. The queue pages twenty at a time and the linked case can
  // be on any of them, so a notice saying "further down" would hand the work
  // back to the officer we sent the link to — and the row they were sent to
  // read is the only reason they are on this screen.
  //
  // BOUNDED, because an id naming no case at all would otherwise pull the whole
  // queue one page at a time looking for it. After the bound the reader is told
  // the case is not here, which is the honest answer and the one they can act
  // on. The counter resets when the address names a different case.
  useEffect(() => {
    if (linked.kind !== "loading" || !loadMore) {
      return;
    }
    pagesPulled.current = { id: linked.id, pages: pagesFor(linked.id) + 1 };
    loadMore();
  });

  const toggle = useCallback(
    (id: string) => {
      // Read outside the render rather than from the snapshot above: two
      // toggles in one handler would both build on a stale copy and the second
      // would discard the first, which is what app/urlstate.ts says about
      // every other dial.
      const next = new Map(currentParams());
      if (caseInParams(next) === id) {
        next.delete(CASE_PARAM);
      } else {
        next.set(CASE_PARAM, id);
      }
      setParams(next);
    },
    [setParams],
  );

  return { expandedId, linked, toggle };
}
