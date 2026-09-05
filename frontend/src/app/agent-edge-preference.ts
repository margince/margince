// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useSyncExternalStore } from "react";

/**
 * Whether this reader wants the margins lit at all.
 *
 * The edge (`agent-edge.tsx`) is the one thing on a workspace screen that moves
 * without being asked for, and it moves in the periphery of the whole window —
 * which is exactly where movement is hardest to work beside for some people,
 * and merely unwanted for others. `prefers-reduced-motion` already reaches the
 * loop and calms the waves; this is the other half of the same courtesy, for
 * the reader whose machine states no preference and who wants the frame still.
 *
 * ON is the default and stays it: the lit rim is how the product says the agent
 * is working, so an installation nobody has told otherwise shows it. Turning it
 * off costs a reading, not a fact — everything the margin carries is written in
 * words in the rail beside it, which is why it could be `aria-hidden` in the
 * first place.
 *
 * Per browser rather than per seat, and stored the way the theme is: it is a
 * choice about the screen somebody is sitting in front of, not about their
 * place in the workspace, and it has to hold before any read of `/me` answers.
 */

/** Where the answer is kept. The theme's namespace, not its key. */
export const EDGE_LIGHT_KEY = "margince.edgeLight";

/** The two words storage carries. Anything else is an install that never chose. */
const OFF = "off";
const ON = "on";

const listeners = new Set<() => void>();

/** Storage is unavailable in some embedded contexts; a preference nobody can
 *  read is an unstated one, never an error. */
function readStored(): boolean {
  try {
    return window.localStorage.getItem(EDGE_LIGHT_KEY) !== OFF;
  } catch {
    return true;
  }
}

/**
 * What this page is showing, resolved on first read.
 *
 * Held rather than re-read per render for the reason the theme holds its own:
 * the answer is asked for on every render of the margins, and a store that went
 * to storage each time would also have nowhere to keep a flip a browser refused
 * to persist — the reader would press the switch and watch nothing happen.
 */
let shown: boolean | null = null;

function ensureLoaded(): boolean {
  if (shown === null) {
    shown = readStored();
  }
  return shown;
}

/** The current answer, for a consumer that is not a component. */
export function edgeLightShown(): boolean {
  return ensureLoaded();
}

/**
 * Say whether the margins may light, and tell everything already drawn.
 *
 * The write is best-effort and the flip is not: a browser refusing storage
 * costs the reader their choice on the NEXT page, not on this one.
 */
export function setEdgeLightShown(next: boolean): void {
  shown = next;
  try {
    window.localStorage.setItem(EDGE_LIGHT_KEY, next ? ON : OFF);
  } catch {
    // Persisting is the enhancement; honouring the flip is the feature.
  }
  for (const listener of listeners) {
    listener();
  }
}

function subscribe(listener: () => void): () => void {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}

/**
 * The preference, as a component sees it.
 *
 * A boolean snapshot, so `useSyncExternalStore` compares it by value and no
 * identity has to be kept stable between reads.
 */
export function useEdgeLightShown(): boolean {
  return useSyncExternalStore(subscribe, edgeLightShown, edgeLightShown);
}
