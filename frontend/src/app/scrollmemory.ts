import { useEffect, useRef, useState } from "react";

// Where the reader was on the page they just left.
//
// The document itself never scrolls here — `.app` is exactly one viewport tall
// and the content COLUMN scrolls — and that column is the same element on every
// route. So the shell has always had to reset it, or the offset one page was
// left at carried straight into the next. What it did was reset to the top on
// EVERY address change, Back and Forward included: the reader who scrolled a
// long list, opened a record and came back was returned to the list and not to
// their place in it, which is half of what Back is for.
//
// Memory is per page LOAD, deliberately. A reload has no place to return to —
// the rows are being fetched again — and pretending otherwise would jump a
// reader into a list that is still arriving.

/**
 * The history entry the reader is on.
 *
 * The browser gives an entry no identity of its own, so one is stamped into
 * `history.state` the first time each entry is seen. A COUNTER rather than a
 * random id: the offsets below live only as long as this page load, so an id
 * only has to be unique within one — and an id restored from `history.state`
 * after a reload then finds nothing, which is the right answer rather than a
 * collision waiting to happen.
 */
let stamped = 0;

const ENTRY_KEY = "marginceEntry";

function entryOf(state: unknown): string | undefined {
  if (typeof state === "object" && state !== null && ENTRY_KEY in state) {
    const id = (state as Record<string, unknown>)[ENTRY_KEY];
    return typeof id === "string" ? id : undefined;
  }
  return undefined;
}

/**
 * This history entry's id, stamping one if the entry has none.
 *
 * `replaceState` rather than a push: naming the entry the reader is already on
 * must not create another one, and it fires no `hashchange`, so nothing
 * downstream mistakes the stamp for a navigation.
 */
export function historyEntryId(): string {
  const existing = entryOf(globalThis.history.state);
  if (existing) {
    return existing;
  }
  stamped += 1;
  const id = `e${stamped}`;
  const state =
    typeof globalThis.history.state === "object" &&
    globalThis.history.state !== null
      ? { ...globalThis.history.state, [ENTRY_KEY]: id }
      : { [ENTRY_KEY]: id };
  globalThis.history.replaceState(state, "");
  return id;
}

/** Forget every remembered offset. For tests, which share one document. */
export function forgetScrollMemory(): void {
  offsets.clear();
  stamped = 0;
}

const offsets = new Map<string, number>();

/**
 * Keep `column` at the offset this history entry was left at.
 *
 * `address` is the trigger: it changes on every move, including the ones the
 * browser makes for Back and Forward. On the way OUT the current offset is
 * recorded against the entry being left; on the way IN a recorded one is
 * restored and anything else opens at the top, because a page nobody has
 * visited has no place to return to.
 *
 * The restore is RETRIED while the column grows. A list restores its rows a
 * moment after the address arrives, so the first attempt lands against a column
 * that is still short and the browser clamps it — which is how a restore
 * silently becomes a scroll to somewhere near the top. Retrying until the
 * offset is actually reachable is what makes it land, and a reader who scrolls
 * in the meantime is left alone: their scroll is the answer to where they want
 * to be.
 */
export function useScrollMemory(
  column: React.RefObject<HTMLElement | null>,
  address: string,
): void {
  // Recomputed per address rather than read in the effect, so the id belongs to
  // the entry being LEFT when the cleanup runs — by then `history.state` is
  // already the next entry's.
  const [entry, setEntry] = useState(historyEntryId);
  const wanted = useRef<number | null>(null);
  // `address` is the TRIGGER and not a value the body reads: a move is what
  // makes the entry worth asking about again, and `history.state` is where the
  // answer lives. Reading the address instead would key the memory on the URL,
  // which a screen rewrites in place whenever a reader turns a dial — and that
  // is not leaving the page.
  // biome-ignore lint/correctness/useExhaustiveDependencies: trigger-only dep
  useEffect(() => {
    setEntry(historyEntryId());
  }, [address]);

  useEffect(() => {
    const scroller = column.current;
    if (!scroller) {
      return;
    }
    const target = offsets.get(entry) ?? 0;
    wanted.current = target;
    scroller.scrollTop = target;

    // A reader who scrolls has said where they want to be, so stop trying to
    // put them somewhere else — and record what they chose.
    const onScroll = () => {
      if (wanted.current !== null && scroller.scrollTop !== wanted.current) {
        wanted.current = null;
      }
    };
    scroller.addEventListener("scroll", onScroll, { passive: true });

    // The column grows as rows arrive. Each growth is another chance for an
    // offset the browser had to clamp.
    const observer =
      target > 0 && typeof ResizeObserver === "function"
        ? new ResizeObserver(() => {
            if (wanted.current !== null) {
              scroller.scrollTop = wanted.current;
            }
          })
        : null;
    observer?.observe(scroller);

    return () => {
      observer?.disconnect();
      scroller.removeEventListener("scroll", onScroll);
      offsets.set(entry, scroller.scrollTop);
    };
  }, [column, entry]);
}
