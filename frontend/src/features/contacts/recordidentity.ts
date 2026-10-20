import { useQueryClient } from "@tanstack/react-query";

// The shape a list's cache entry has, narrowed to the two fields read here: the
// hook is given a key rather than a type, so it states what it needs of whatever
// that key holds instead of trusting a cast. `data` is what a list page calls its
// rows (listquery.tsx) — a wrong name here reads as "the open did not come from
// a list" and silently seeds nothing, so it is pinned by a test.
type CachedList = {
  pages?: readonly { data?: readonly { id?: string; full_name?: string }[] }[];
};

/**
 * The name a record page can draw before its own read returns.
 *
 * A record route carries an id, not a name, so a head that waits for the
 * record's read shows nothing until the network answers — and an identity on
 * screen before anything is fetched is what makes an open feel instant. The
 * name is already in the client when the open came from a list: the list's
 * pages hold the row that was clicked.
 *
 * Read synchronously and NOT subscribed to, on purpose. This is a seed for the
 * first paint, not a second source for the name — once the record's own read
 * lands, the page renders from that and this value is not consulted again. A
 * subscription would re-render the page on every unrelated list refetch to
 * change a heading that is no longer on screen.
 *
 * Answers null when the open did not come from a list — a pasted address, a
 * reload, a link from mail. The caller shows what it showed before in that case;
 * there is nothing dishonest to invent.
 */
export function useCachedRecordName(
  listKey: string,
  id: string,
): string | null {
  const client = useQueryClient();
  const cached = client.getQueriesData<CachedList>({ queryKey: [listKey] });

  // Trimmed, because whitespace is truthy and an untrimmed blank would put an
  // empty heading where the placeholder belongs.
  const names = new Set(
    cached.flatMap(([, list]) =>
      (list?.pages ?? []).flatMap((page) =>
        (page.data ?? [])
          .filter((row) => row.id === id)
          .map((row) => row.full_name?.trim())
          .filter((name) => Boolean(name)),
      ),
    ),
  );

  // ONE name across every cached list, or none that can be trusted. Two entries
  // disagreeing — one drawn before a rename, say — means neither is known to be
  // current, and the read that will say is already in flight. A heading that
  // flashes the wrong name is worse than one that arrives a moment later.
  const [only] = names;
  return names.size === 1 && only ? only : null;
}
