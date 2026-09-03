// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
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

// A paged section's rows are its count only when the page is the whole set.
// `PageInfo` carries `has_more` and no total, so a cut page is a floor and not
// a count — "25" beside sixty contacts is not a smaller truth but a wrong one.
// Past the cut the count is absent, and a reader who sees no number goes and
// looks, which is the outcome a wrong number prevents. The tab strip's badges
// (companyTabCounts) and the rail's summaries and "All" verbs all read this
// one rule.
export function wholeCount(section?: {
  data: readonly unknown[];
  page: components["schemas"]["PageInfo"];
}): number | undefined {
  if (!section || section.page.has_more) {
    return undefined;
  }
  return section.data.length;
}

// A section has answered, one way or the other, only once it is ready or
// empty; withheld, unavailable, loading and failed are all still "we don't
// know yet" from the reader's side of the count badge.
export function sectionAnswered(state: SectionState): boolean {
  return state === "ready" || state === "empty";
}
