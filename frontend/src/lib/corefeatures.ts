// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMemo } from "react";
import { type AccessSnapshot, grants, type RbacObject } from "./accessgrants";
import type { CoreFeature } from "./corefeaturepolicy";
import { useMe } from "./session";

export {
  type CoreFeature,
  recordEnabled,
  screenEnabled,
} from "./corefeaturepolicy";

const NONE: readonly CoreFeature[] = [];

export function useDisabledCoreFeatures(): readonly CoreFeature[] {
  return useMe(false).data?.settings_availability?.disabled_features ?? NONE;
}

// The object whose read the server demands before it lists a feature's records.
// The other features gather several objects (Home, Filters, Insights), so no one
// grant decides them and each of their sections asks for its own.
export const FEATURE_READ_OBJECT: readonly (readonly [
  CoreFeature,
  RbacObject,
])[] = [
  ["contacts", "contact"],
  ["companies", "company"],
  ["leads", "lead"],
  ["deals", "deal"],
  ["projects", "project"],
];

/** The features this reader may not read, whatever the installation turned on. */
export function unreadableCoreFeatures(
  snapshot: AccessSnapshot,
): readonly CoreFeature[] {
  if (snapshot === undefined) return NONE;
  return FEATURE_READ_OBJECT.flatMap(([feature, object]) =>
    grants(snapshot, object, "read") ? [] : [feature],
  );
}

/**
 * Features that are off for this installation or unreadable to this reader.
 * Used where a destination is offered: a sidebar row, a palette jump, a
 * shortcut. No one is sent to a screen the server refuses them.
 */
export function useHiddenCoreFeatures(): readonly CoreFeature[] {
  const me = useMe(false).data;
  return useMemo(
    () => [
      ...(me?.settings_availability?.disabled_features ?? NONE),
      ...unreadableCoreFeatures(me),
    ],
    [me],
  );
}
