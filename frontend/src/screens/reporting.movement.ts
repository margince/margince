// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import type { Translator } from "../i18n";

type MovementBucket = components["schemas"]["ForecastMovementBucket"]["name"];

// A Record over the contract's union. A new bucket fails the build here until
// it has a word, so it never reaches a chart as its code key.
const BUCKETS: Record<MovementBucket, true> = {
  new: true,
  pulled_in: true,
  pushed_out: true,
  amount: true,
  category: true,
  stage_weight: true,
  won: true,
  lost: true,
  reopened_or_archived: true,
  fx: true,
  definition: true,
  model: true,
};

function isBucket(key: string): key is MovementBucket {
  return Object.hasOwn(BUCKETS, key);
}

/** The reader's word for a movement bar; a key that is no bucket stays as sent. */
export function movementLabel(
  key: string,
  label: string,
  t: Translator,
): string {
  return isBucket(key) ? t(`reporting.movement.${key}`) : label;
}
