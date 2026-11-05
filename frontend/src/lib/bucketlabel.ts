// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import type { Translator } from "../i18n";

/**
 * The word for a relationship bucket, on every screen that prints one. A
 * contact nobody has written to has no signal yet, which is not "dormant".
 */
export function bucketLabel(
  bucket: components["schemas"]["RelationshipStrength"]["bucket"],
  t: Translator,
): string {
  return bucket === "none"
    ? t("standing.ring.noInteractions")
    : t(`contact.bandBadge.${bucket}`);
}
