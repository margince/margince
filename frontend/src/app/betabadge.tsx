// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Badge } from "../design-system/atoms";
import { useT } from "../i18n";

/**
 * TEMPORARY. The rail head's stage marker, here to tell somebody meeting the
 * product for the first time that it is not finished — and gone the day that
 * stops being true.
 *
 * DELETING IT IS ONE MOVE. Remove, together:
 *   - this file;
 *   - its call site in `app/shell.tsx` — the `<BetaBadge />` that `BrandBlock`
 *     seats on the head's second line, and the import above it;
 *   - `shell.beta` from `i18n/en.ts`, `i18n/de.ts` and `i18n/vi.ts`, and its
 *     entry in `i18n.test.ts`'s `KEPT_IN_ENGLISH`, which fails on an allowlisted
 *     key no catalog still carries;
 *   - `betabadge.stories.tsx`.
 * Nothing else may reach for it. A second caller is a second place to find on
 * the day it goes, and the marker is one fact about the build that belongs in
 * the one place a reader meets the product: the rail head, beside the mark.
 *
 * WHAT IS TEMPORARY IS THE WRAPPING, NOT THE PILL. The badge itself is the
 * house `Badge` in the `discovery` tone, because a hand-rolled pill is a second
 * spelling of something the design system already draws and a temporary defect
 * is still a defect (`badge-spelling.test.ts` fails one).
 *
 * ONE WORD AT BOTH RAIL WIDTHS. Collapsed, the rail drops every label it has
 * and keeps this one: a build is hardest to identify exactly where there is
 * least room to say so. Four letters fit the 56px column, which the version
 * string this replaces did not — so there is no narrow spelling here, and no
 * second string that could come to name a different stage from this one.
 */
export function BetaBadge() {
  const t = useT();
  return <Badge tone="discovery">{t("shell.beta")}</Badge>;
}
