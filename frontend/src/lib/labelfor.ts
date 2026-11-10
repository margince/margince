// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { MessageKey } from "../i18n/en";

/** The catalogue key shown for a value this build has no label for. */
export const UNKNOWN_VALUE_LABEL: MessageKey = "common.unknownValue";

/**
 * valueFor answers what a map holds for a value, or `fallback` for a value it
 * does not hold. A response enum is open: a newer server can send a value this
 * build does not list, and its closed TypeScript union cannot say so.
 * A caller with its own answer for that value passes `undefined`.
 */
export function valueFor<V extends string, T, F = T>(
  map: Readonly<Record<V, T>>,
  value: V,
  fallback: F,
): T | F {
  return Object.hasOwn(map, value) ? map[value] : fallback;
}

/** labelFor is valueFor for a catalogue key: an unknown value gets a neutral
 * label rather than a blank. */
export function labelFor<V extends string>(
  labels: Readonly<Record<V, MessageKey>>,
  value: V,
): MessageKey {
  return valueFor(labels, value, UNKNOWN_VALUE_LABEL);
}
