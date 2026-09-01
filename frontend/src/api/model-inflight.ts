// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useSyncExternalStore } from "react";

/**
 * How many model calls THIS TAB is waiting on, right now.
 *
 * The server's projection is the record of what the agent did, and it is the
 * only thing that can say what a run was. It cannot say the one thing a reader
 * pressing a button needs first: that their own ask left the building. The
 * projection is a read on a poll, so between the click and the next read the
 * chrome has nothing to report and reports nothing, which is how "Draft with
 * AI" came to light no orb at all.
 *
 * This is the other end of the same fact, observed where it is already known:
 * the client is holding a request open to a route whose handler calls a model
 * and waits. It is not a second opinion about what the agent is doing. It
 * carries no kind, no state and no sentence, because it knows none of those;
 * everything the rail SAYS still comes from the feed.
 *
 * The same shape as `agent-edge-signal.ts`: one module-level value, a publish
 * on every change, and a snapshot whose identity is stable between changes so
 * `useSyncExternalStore` does not re-render forever. EVERY change, not only the
 * crossings of zero: the rail reads the count as a boolean, but the feed is
 * refetched on each edge of a call, and two calls overlapping would otherwise
 * have their inner edges — the second leaving, the first answering — pass
 * without a read.
 */

let open = 0;
const listeners = new Set<() => void>();

function notify(): void {
  for (const listener of listeners) {
    listener();
  }
}

/** A request to a model route has left. */
export function beginModelCall(): void {
  open += 1;
  notify();
}

/**
 * A request to a model route has answered, or failed, or timed out.
 *
 * Called from a `finally`, so every ending counts: a refused draft is the agent
 * no longer working just as much as a delivered one, and a count that only came
 * down on success would leave the orb lit for the rest of the session.
 */
export function endModelCall(): void {
  if (open === 0) {
    // Nothing began, so nothing can end. Guarded rather than allowed to go
    // negative: a negative count reads as false in the consumer and would hide
    // the NEXT real call instead of the bug that caused it.
    return;
  }
  open -= 1;
  notify();
}

/** The current count, for a consumer that is not a component. */
export function modelCallsInFlight(): number {
  return open;
}

function subscribe(onChange: () => void): () => void {
  listeners.add(onChange);
  return () => {
    listeners.delete(onChange);
  };
}

/** The count, as a component sees it. */
export function useModelCallsInFlight(): number {
  return useSyncExternalStore(
    subscribe,
    modelCallsInFlight,
    modelCallsInFlight,
  );
}
