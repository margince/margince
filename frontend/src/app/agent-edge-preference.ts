// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { deviceSwitch, useDeviceSwitch } from "../lib/devicepreference";
import { STORAGE_KEYS } from "../lib/storage";

/**
 * Whether this reader wants the margins lit at all.
 *
 * The edge (`agent-edge.tsx`) is the one thing on a workspace screen that moves
 * without being asked for, and it moves in the periphery of the whole window —
 * which is exactly where movement is hardest to work beside for some contacts,
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

const edgeLight = deviceSwitch(STORAGE_KEYS.edgeLight);

/** The current answer, for a consumer that is not a component. */
export const edgeLightShown = edgeLight.read;

/** Say whether the margins may light, and tell everything already drawn. */
export const setEdgeLightShown = edgeLight.write;

export function useEdgeLightShown(): boolean {
  return useDeviceSwitch(edgeLight);
}
