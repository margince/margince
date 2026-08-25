// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useSyncExternalStore } from "react";

/**
 * What the screen's own margins are saying about the agent, ONE signal, for the
 * whole document.
 *
 * The agent's state is derived in the rail (`agentrail.tsx`), from a stack that
 * only that component has: the reviewer's state switcher, the scripted run, and
 * the reads it makes itself. A second consumer cannot re-derive it, calling
 * those hooks again produces a SEPARATE run and a separate override, so the
 * panel's switcher would drive the rail and leave the margins dead. So the rail
 * publishes here and everything else reads.
 *
 * The same shape as `window-focus.ts`, deliberately: one module-level state, a
 * publish that only notifies on a real change, and a subscribe that hands over
 * the current answer before it returns.
 */

/**
 * Two independent facts, not one state.
 *
 * They can both be true, an agent reading right now while an approval from ten
 * minutes ago still waits, and they are read by different parts of the margin,
 * so collapsing them into one enum would force a choice the screen does not have
 * to make.
 */
export type AgentEdgeReading = Readonly<{
  /** Work is in flight: the agent is reading or acting THIS moment. */
  reading: boolean;
  /** Something is staged and cannot move until a person says so. */
  waiting: boolean;
}>;

/** Nothing is happening, which is the honest answer whenever no rail is mounted. */
export const AGENT_EDGE_STILL: AgentEdgeReading = {
  reading: false,
  waiting: false,
};

let shown: AgentEdgeReading = AGENT_EDGE_STILL;
const listeners = new Set<() => void>();

/**
 * Say what the agent is doing.
 *
 * Only a CHANGE is published. The rail re-renders on every cache notification -
 * several times a second while a page settles, and a store that notified on
 * each of them would re-render the margins for readings identical to the one
 * already on screen.
 */
export function publishAgentEdge(next: AgentEdgeReading): void {
  if (next.reading === shown.reading && next.waiting === shown.waiting) {
    return;
  }
  shown = { reading: next.reading, waiting: next.waiting };
  for (const listener of listeners) {
    listener();
  }
}

/**
 * Go quiet.
 *
 * The rail calls this as it unmounts. Without it a reading would outlive the
 * component that made it: sign out while the agent is mid-read and the login
 * screen would inherit a lit margin belonging to a session that has ended.
 */
export function clearAgentEdge(): void {
  publishAgentEdge(AGENT_EDGE_STILL);
}

/** The current answer, for a consumer that is not a component. */
export function currentAgentEdge(): AgentEdgeReading {
  return shown;
}

function subscribe(onChange: () => void): () => void {
  listeners.add(onChange);
  return () => {
    listeners.delete(onChange);
  };
}

/**
 * The reading, as a component sees it.
 *
 * `shown` is replaced only when a value actually changed, so its identity is
 * stable between changes, which is exactly what `useSyncExternalStore` demands
 * of a snapshot: a fresh object every call would re-render forever.
 */
export function useAgentEdge(): AgentEdgeReading {
  return useSyncExternalStore(subscribe, currentAgentEdge, currentAgentEdge);
}
