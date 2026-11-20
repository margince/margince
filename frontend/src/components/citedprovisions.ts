// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";

export type CitedProvision = components["schemas"]["CitedProvision"];

/**
 * Every law a finding cites, the recipient's first: `provisions` when the
 * server sent it, else the single `provision` an older answer carries.
 */
export function lawsOf(
  cited: Readonly<{
    provision?: CitedProvision;
    provisions?: readonly CitedProvision[];
  }>,
): readonly CitedProvision[] {
  if (cited.provisions?.length) return cited.provisions;
  return cited.provision ? [cited.provision] : [];
}
