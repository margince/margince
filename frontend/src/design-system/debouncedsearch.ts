/**
 * A search box's four honest states, on a debounce.
 *
 * Nothing typed, in flight, an answer, a failure — a picker over a set too
 * large to enumerate has to say which of those it is in, because the one thing
 * it must never do is show an empty list and let that read as "there are none".
 *
 * Shared because the list toolbar's company filter and the filter builder's
 * organization value are the same question asked on two surfaces, and a second
 * spelling is one that could quietly stop debouncing, stop cancelling, or start
 * clearing the menu out from under a reader while the next answer is still
 * coming back.
 *
 * The MARKUP is deliberately not shared. One renders menu items inside an open
 * chip, the other a value column inside a clause row; folding those together
 * would mean a component with a mode flag, which is two components wearing one
 * name.
 */

import { useEffect, useState } from "react";

/** One result, already carrying the label a reader should see. */
export type SearchResult = Readonly<{ value: string; label: string }>;

/**
 * Long enough that typing a company name is one request rather than eight,
 * short enough that the list arrives while the reader is still looking at the
 * box.
 *
 * The same rhythm as the list search (listquery.tsx's SEARCH_DEBOUNCE_MS),
 * kept as a sibling rather than imported: that one is a screen-binding concern
 * and this is a design-system one, so the number is the contract and not the
 * module.
 */
export const SEARCH_DEBOUNCE_MS = 250;

export type DebouncedSearch = Readonly<{
  results: readonly SearchResult[];
  pending: boolean;
  failed: boolean;
}>;

/**
 * Runs `search` for `query` on a debounce, reporting which state the answer is
 * in.
 *
 * A query in flight KEEPS the previous results: clearing them would empty the
 * menu under the reader on every keystroke, and an empty menu is the one thing
 * that reads as a confident "no matches". `pending` is reported alongside them
 * so a caller can say a newer answer is coming rather than leave the older one
 * standing as the settled one.
 *
 * A FAILED query drops them, which is the opposite call made for the opposite
 * reason: there is no newer answer coming, so hits from a query no longer on
 * screen would sit under an error line contradicting them. The box going back
 * to empty drops them too, where they would be results for a query nobody can
 * see any more.
 *
 * An in-flight answer that arrives after the query moved on is discarded rather
 * than shown — otherwise a slow response for "acme" lands on top of the results
 * for "acme corp" and the reader picks from the wrong list.
 */
export function useDebouncedSearch(
  search: ((query: string) => Promise<readonly SearchResult[]>) | undefined,
  query: string,
): DebouncedSearch {
  const [results, setResults] = useState<readonly SearchResult[]>([]);
  const [pending, setPending] = useState(false);
  const [failed, setFailed] = useState(false);

  useEffect(() => {
    if (!search) {
      return;
    }
    if (!query) {
      setResults([]);
      setPending(false);
      setFailed(false);
      return;
    }
    let cancelled = false;
    setPending(true);
    const timer = setTimeout(() => {
      search(query)
        .then((next) => {
          if (cancelled) {
            return;
          }
          setResults(next);
          setFailed(false);
          setPending(false);
        })
        .catch(() => {
          if (cancelled) {
            return;
          }
          setResults([]);
          setFailed(true);
          setPending(false);
        });
    }, SEARCH_DEBOUNCE_MS);
    return () => {
      cancelled = true;
      clearTimeout(timer);
    };
  }, [query, search]);

  return { results, pending, failed };
}
