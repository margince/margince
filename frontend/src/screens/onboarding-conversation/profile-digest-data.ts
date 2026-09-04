// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";
import { webUrl } from "../../format/weburl";
import type { MessageKey } from "../../i18n/en";
import { FACT_CATEGORY_ORDER } from "../factview";
import type { ReviewRow } from "./company-review-state";

// The citation and grouping arithmetic shared by the article and the sidebar:
// which page a value cites, and which fact category it falls under. Kept apart
// from the components that render them so the two rendering files stay about
// layout rather than about renumbering the same list twice.

export type Fact = components["schemas"]["CompanySiteReadFact"];
export type Person = components["schemas"]["CompanySiteReadPerson"];
export type LegalEntity = components["schemas"]["CompanySiteReadLegalEntity"];
export type Page = components["schemas"]["CompanySiteReadPage"];

/** One page the profile cites, and the number it is cited by. */
export type Citation = Readonly<{ url: string; n: number }>;

/**
 * Number the pages in the order the profile first cites them.
 *
 * BY URL, so a page backing four fields is one entry cited four times rather
 * than four entries for one page. First-cited order rather than the read's own
 * order, because the numbers are read down the article and a list that jumped
 * would look like pages were missing.
 *
 * `extra` carries the citing URLs of everything the record rows do not cover
 * — a legal entity, a fact, a person — in the order the article renders them,
 * so a page backing both a profile line and one of these still earns exactly
 * one number.
 */
export function citationsOf(
  rows: readonly ReviewRow[],
  extra: readonly string[] = [],
): Citation[] {
  const seen = new Map<string, number>();
  const cite = (url: string | undefined) => {
    if (url === undefined || url === "" || seen.has(url)) {
      return;
    }
    seen.set(url, seen.size + 1);
  };
  for (const row of rows) {
    cite(row.evidence?.source);
  }
  for (const url of extra) {
    cite(url);
  }
  return [...seen].map(([url, n]) => ({ url, n }));
}

/**
 * The number a record line prints, or none: a row typed by a person or carried
 * in from an existing profile has no page behind it. One spelling, because the
 * companion, the article and the sidebar all print the same superscript.
 */
export function citationOf(
  row: ReviewRow,
  number: ReadonlyMap<string, number>,
): number | undefined {
  return row.evidence === null ? undefined : number.get(row.evidence.source);
}

/**
 * The address a reference entry prints: host and path, no scheme. The
 * references list is the one place the host is worth repeating per row —
 * unlike a page strip read one crawl at a time, this is a list a reader
 * copies an address out of, and a bare path is not one.
 */
export function referenceAddressOf(url: string): string {
  const parsed = webUrl(url);
  if (parsed === null) {
    return url;
  }
  return `${parsed.host}${parsed.pathname === "/" ? "" : parsed.pathname}`;
}

/** The host a page is read from, for the header's own sentence. A value that
 * is not a web address prints as it came, which is honest rather than blank. */
export function hostOf(url: string): string {
  return webUrl(url)?.host ?? url;
}

type PageKind = NonNullable<
  components["schemas"]["CompanySiteReadPage"]["kind"]
>;

/** What a reference's page is, in the reader's words — the read's own closed
 * vocabulary of page kinds, never a guessed title. A page the crawl kept no
 * kind for reads as "Page", which is honest rather than blank. */
const PAGE_KIND_LABELS: Readonly<Record<PageKind, MessageKey>> = {
  home: "ob.digest.pageKind.home",
  impressum: "ob.digest.pageKind.impressum",
  about: "ob.digest.pageKind.about",
  team: "ob.digest.pageKind.team",
  services: "ob.digest.pageKind.services",
  products: "ob.digest.pageKind.products",
  contact: "ob.digest.pageKind.contact",
  other: "ob.digest.pageKind.other",
};

export function pageKindLabelKey(
  kind: PageKind | null | undefined,
): MessageKey {
  return kind ? PAGE_KIND_LABELS[kind] : PAGE_KIND_LABELS.other;
}

/** The read's own record of a cited page, when it walked that exact URL — a
 * citation can also name a page the read never fetched (a candidate's own
 * imprint page, reached through the entity census rather than the crawl), and
 * that one carries no kind to report. */
export function pageOf(
  pages: readonly components["schemas"]["CompanySiteReadPage"][],
  url: string,
): components["schemas"]["CompanySiteReadPage"] | undefined {
  return pages.find((page) => page.url === url);
}

/** The facts grouped by category, in reading order, empty groups dropped. */
export function factsByCategory(
  facts: readonly Fact[],
): ReadonlyArray<{ category: Fact["category"]; facts: readonly Fact[] }> {
  return FACT_CATEGORY_ORDER.map((category) => ({
    category,
    facts: facts.filter((fact) => fact.category === category),
  })).filter((group) => group.facts.length > 0);
}

/** The single best fact behind a sidebar figure: highest confidence first,
 * the read's own order breaking a tie. Never a blend of several — a sidebar
 * line carries one citation, on the same rule every other line here does. */
export function bestFact(
  facts: readonly Fact[],
  field: Fact["field"],
): Fact | undefined {
  return facts
    .filter((fact) => fact.field === field)
    .sort((a, b) => b.confidence - a.confidence)
    .at(0);
}
