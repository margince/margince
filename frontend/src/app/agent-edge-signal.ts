// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useSyncExternalStore } from "react";

/**
 * What the screen's own margins are saying about the agent, ONE signal, for the
 * whole document.
 *
 * The agent's state is derived in the rail (`agentrail.tsx`), from the reads it
 * makes itself. A second consumer cannot re-derive it: calling those hooks again
 * makes a SEPARATE set of reads, so the margins would report on a second agent
 * whose answers arrive at their own times. So the rail publishes here and
 * everything else reads.
 *
 * The same shape as `window-focus.ts`, deliberately: one module-level state, a
 * publish that only notifies on a real change, and a subscribe that hands over
 * the current answer before it returns.
 */

/**
 * Which register the margins speak in.
 *
 * Both are work in flight and the margin lights for both, but they are not the
 * same kind of work and were not being read as the same kind. An agent run is
 * seconds to a minute: a rim that swells and sends a head of light round the
 * frame says NOW, and is gone before it can become wallpaper. A mailbox import
 * is minutes to hours, and the same rim over that span is a lamp left on in the
 * corner of every screen a person works in. So the import takes a thinner,
 * calmer register — still lit, still moving, still saying mail is arriving —
 * and the agent's own register is a little thicker and livelier, so the two
 * read as two things rather than as one thing at two volumes. What each draws
 * at is in `agent-edge-gl.ts`.
 */
export type AgentEdgeRegister = "agent" | "capture";

/**
 * The one fact the margins draw: whether work is in flight, and in which
 * register.
 *
 * A STAGED DECISION IS NOT PUBLISHED HERE. It used to be, as a second boolean
 * closing the margin into a still contour, and on any installation with an
 * unanswered queue that drew a ring around the whole window for as long as the
 * queue stood. The rail states it in words, with its count. This signal carries
 * only what the margin actually paints, so there is no published fact nothing
 * reads.
 */
export type AgentEdgeReading = Readonly<{
  /** Work is in flight: the agent is reading or acting THIS moment. */
  reading: boolean;
  /**
   * Which register the light is in. It means nothing while dark, so a dark
   * reading always wears the agent's: one spelling for rest, whatever the light
   * was doing before it went out.
   */
  register: AgentEdgeRegister;
}>;

/** Nothing is happening, which is the honest answer whenever no rail is mounted. */
export const AGENT_EDGE_STILL: AgentEdgeReading = {
  reading: false,
  register: "agent",
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
  const settled: AgentEdgeReading = next.reading
    ? { reading: true, register: next.register }
    : AGENT_EDGE_STILL;
  if (
    settled.reading === shown.reading &&
    settled.register === shown.register
  ) {
    return;
  }
  shown = settled;
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
