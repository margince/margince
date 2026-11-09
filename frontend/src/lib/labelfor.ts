// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { MessageKey } from "../i18n/en";

/** The catalogue key shown for a value this build has no label for. */
export const UNKNOWN_VALUE_LABEL: MessageKey = "common.unknownValue";

/**
 * labelFor answers the catalogue key a value maps to. A response enum is open,
 * so a newer server can send a value this build does not list. That value gets
 * a neutral label rather than a blank.
 */
export function labelFor<V extends string>(
  labels: Readonly<Record<V, MessageKey>>,
  value: V,
): MessageKey {
  return Object.hasOwn(labels, value) ? labels[value] : UNKNOWN_VALUE_LABEL;
}
