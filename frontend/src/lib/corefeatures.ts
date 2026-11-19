// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMemo } from "react";
import { type AccessSnapshot, grants, type RbacObject } from "./accessgrants";
import type { CoreFeature, Withheld } from "./corefeaturepolicy";
import { useMe } from "./session";

export {
  type CoreFeature,
  recordEnabled,
  screenEnabled,
  type Withheld,
} from "./corefeaturepolicy";

const NONE: readonly CoreFeature[] = [];

export function useDisabledCoreFeatures(): readonly CoreFeature[] {
  return useMe(false).data?.settings_availability?.disabled_features ?? NONE;
}

// What the server demands before a destination's first request answers. A list
// is any-of. Home and Filters gather several reads, so no one grant decides
// them and each of their sections asks for its own.
export const READ_REQUIREMENTS: readonly (readonly [
  Withheld,
  readonly RbacObject[],
])[] = [
  ["contacts", ["contact"]],
  ["companies", ["company"]],
  ["leads", ["lead"]],
  ["deals", ["deal"]],
  ["projects", ["project"]],
  ["screen:analytics", ["forecast", "deal"]],
  ["screen:progress", ["report_definition"]],
  ["screen:goals", ["sales_target"]],
];

/** What this reader may not read, whatever the installation turned on. */
export function unreadableCoreFeatures(
  snapshot: AccessSnapshot,
): readonly Withheld[] {
  // No stated grants hides nothing: the probe has not answered, or the server is
  // too old to send them. Only a stated refusal withholds a destination.
  if (snapshot?.authorization === undefined) return [];
  return READ_REQUIREMENTS.flatMap(([withheld, objects]) =>
    objects.some((object) => grants(snapshot, object, "read"))
      ? []
      : [withheld],
  );
}

/**
 * Features that are off for this installation or unreadable to this reader.
 * Used where a destination is offered: a sidebar row, a palette jump, a
 * shortcut. No one is sent to a screen the server refuses them.
 */
export function useHiddenCoreFeatures(): readonly Withheld[] {
  const me = useMe(false).data;
  return useMemo(
    () => [
      ...(me?.settings_availability?.disabled_features ?? NONE),
      ...unreadableCoreFeatures(me),
    ],
    [me],
  );
}
