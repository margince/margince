// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Badge } from "../design-system/atoms";
import type { SectionState } from "../design-system/surfacestate";
import { formatNumber } from "../format/format";
import { useLocale } from "../i18n";

// Small pieces the rail's own sections draw off — a leaf so companyrail.tsx
// and companyrailtags.tsx do not import each other, the same no-cycle shape
// companylookups.ts already keeps for organizations.tsx/company360.tsx.

// A collapsible section's summary: the name, plus how many rows it carries.
// Still drawn by the signals disclosure, the one section left inside a
// `Disclosure` rather than its own `Panel`.
//
// `count` is absent, not zero, while the section is withheld, unavailable or
// still loading: a "0" badge above the restricted notice below it reads as an
// empty account rather than a permission boundary, the exact conflation
// sectionState exists to keep apart.
export function SectionSummary({
  title,
  count,
}: Readonly<{ title: string; count?: number }>) {
  const { locale } = useLocale();
  return (
    <span className="co-sect-summary">
      {/* A heading, so the section is reachable by name in the outline: the
          column is one pane of named slices, and each slice's name is a
          heading under the pane rather than a card title of its own. */}
      <h3 className="co-sect-title">{title}</h3>
      {count != null && <Badge>{formatNumber(count, locale)}</Badge>}
    </span>
  );
}

// A section has answered, one way or the other, only once it is ready or
// empty; withheld, unavailable, loading and failed are all still "we don't
// know yet" from the reader's side of the count badge.
export function sectionAnswered(state: SectionState): boolean {
  return state === "ready" || state === "empty";
}
