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
// One page can have more than one of these. A full-height screen gives its own
// element the overflow and the page column then never moves at all — on the
// companies list the rows are 8800px inside a 400px box while the column that
// surrounds it is exactly its own height. Each such element is a LANE, named by
// its caller, and a lane is remembered separately: restoring a list's rows into
// the page column would scroll the wrong box by the right amount.
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
  lane = "page",
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

  // The element this lane scrolls, as a value the effect below can depend on.
  //
  // A ref cannot be one: it is filled in after the render that returns it, and
  // an effect keyed on the ref OBJECT never re-runs when the element inside it
  // appears or is replaced. That is not a corner case — a screen that opens on a
  // board and draws its table only when the address asks for one has no rows at
  // all on the first commit, so the effect ran once against nothing and never
  // again. It cost that screen its scroll memory entirely, and intermittently,
  // because whether the table is there on the first commit is a matter of timing.
  //
  // No dependency list, so this runs after EVERY render and asks one identity
  // question. Setting state only when the answer changed is what keeps that from
  // being a loop.
  const [scroller, setScroller] = useState<HTMLElement | null>(null);
  useEffect(() => {
    if (column.current !== scroller) {
      setScroller(column.current);
    }
  });

  useEffect(() => {
    if (!scroller) {
      return;
    }
    const place = `${entry}:${lane}`;
    const target = offsets.get(place) ?? 0;
    wanted.current = target;
    const seek = () => {
      if (wanted.current === null) {
        return;
      }
      // Nor ours to WRITE once the reader has moved on: the element may now
      // belong to the surface that replaced this one, and scrolling it would
      // move a page the reader is currently reading.
      if (historyEntryId() !== entry) {
        wanted.current = null;
        return;
      }
      scroller.scrollTop = wanted.current;
      // Within a pixel, not exactly: a scroll offset is fractional on a display
      // whose device pixel ratio is not a whole number, and an equality test
      // against the number asked for is then never satisfied. The restore never
      // declared itself finished, so the reader's own scrolling went unrecorded
      // and Back returned them to whatever the first clamp had been.
      if (Math.abs(scroller.scrollTop - wanted.current) < 1) {
        // Landed, so the restore is over. From here the element belongs to the
        // reader again and what it holds is worth remembering.
        wanted.current = null;
        offsets.set(place, scroller.scrollTop);
      }
    };
    seek();

    // A reader who takes hold of the list has said where they want to be, so
    // stop trying to put them somewhere else.
    //
    // Read from GESTURES and not from scroll events, because a scroll event
    // cannot say who caused it: the retries below scroll this element too, and
    // every clamp along the way arrives as one. Telling the two apart by the
    // offset they report is a race the retries lose — a growth-triggered retry
    // can update the expected value between the browser scrolling and the event
    // for it being dispatched, and the restore then switches itself off partway.
    // A wheel, a touch or a key cannot be produced by an assignment to
    // `scrollTop`, so as a signal it has no such gap.
    const takeOver = () => {
      // The reader owns the position now, so stop aiming at the old one. What
      // they are looking at IS the answer from here, so it is written down at
      // once rather than waiting for a scroll event to carry it: a gesture that
      // only relinquished left a window with nothing remembered at all, and a
      // list torn down inside that window came back at its first row.
      wanted.current = null;
      offsets.set(place, scroller.scrollTop);
    };
    for (const gesture of ["wheel", "touchstart", "keydown"] as const) {
      scroller.addEventListener(gesture, takeOver, { passive: true });
    }

    // The ONE writer of this lane's offset. There are two moments worth
    // recording — every scroll, and the teardown — and each has the same three
    // ways of being wrong. Spelled twice, the teardown had none of the guards
    // and undid what the scrolls had got right.
    const remember = () => {
      // A restore still in flight: what the element holds is a clamp on the way
      // to the offset already remembered, not a place anyone chose.
      if (wanted.current !== null) {
        return;
      }
      // And only while the reader is still ON this entry. A surface that goes
      // hands its DOM node to whatever renders in its place, and React reuses it
      // rather than building another: opening a record from a list puts the
      // record's OWN table in the element the list was scrolling, still carrying
      // this listener. The record then scrolls its table to the top on arrival,
      // as every table does, and that zero was filed under the list's entry —
      // a reader who had scrolled a long way down came back to the first row.
      // The element is no longer ours to read, whatever it still reports.
      if (historyEntryId() !== entry) {
        return;
      }
      // Gone from the document, where every measurement answers zero. React
      // detaches the DOM before a passive cleanup runs when the subtree
      // unmounts, which is exactly what opening a record does to a list.
      if (!scroller.isConnected) {
        return;
      }
      const bottom = scroller.scrollHeight - scroller.clientHeight;
      const remembered = offsets.get(place);
      if (remembered !== undefined && remembered > bottom) {
        // The range can no longer HOLD the offset already remembered, so
        // whatever the element reports is the browser clamping rather than a
        // choice. This is what leaving a list looks like from in here: opening a
        // record unmounts the rows, a list 8600px tall collapses to 575 with the
        // element still in the document, and both the scroll events for the
        // collapse and the teardown that follows report the clamp. Either one
        // replaced the reader's own offset a moment before Back came to ask for
        // it. Testing for a pin to the very BOTTOM was not enough: the collapse
        // arrives in steps, and a step landing short of the new bottom looks
        // like a choice.
        //
        // A reader who takes hold clears the remembered offset, so this never
        // stands between them and a list they have deliberately narrowed.
        return;
      }
      offsets.set(place, scroller.scrollTop);
    };
    scroller.addEventListener("scroll", remember, { passive: true });

    // The CONTENT grows as rows arrive, and the content is what has to be
    // watched — not the scroller, whose own box is the same size whether it
    // holds ten rows or a thousand. A ResizeObserver on the scroller therefore
    // reports nothing at all while the very growth this retry exists for is
    // happening, and observing the children it has at THIS moment is no better:
    // a table rendered as a placeholder and then replaced by the real one takes
    // the observed element with it.
    //
    // So watch the subtree for the rows arriving, and the boxes for a height
    // that settles without any node changing (a row that grows when its logo
    // loads). Either one is another chance at an offset the browser had to clamp.
    const growth =
      target > 0 && typeof MutationObserver === "function"
        ? new MutationObserver(seek)
        : null;
    growth?.observe(scroller, { childList: true, subtree: true });
    const resize =
      target > 0 && typeof ResizeObserver === "function"
        ? new ResizeObserver(seek)
        : null;
    resize?.observe(scroller);

    return () => {
      growth?.disconnect();
      resize?.disconnect();
      scroller.removeEventListener("scroll", remember);
      for (const gesture of ["wheel", "touchstart", "keydown"] as const) {
        scroller.removeEventListener(gesture, takeOver);
      }
      // For the case this hook moves to another ENTRY while the element stays:
      // the offset belongs to the entry being left, and nothing else will write
      // it. Every guard that matters is inside `remember`.
      remember();
    };
  }, [scroller, entry, lane]);
}
