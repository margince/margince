// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import { formatNumber, INTL_LOCALE } from "../format/format";
import type { Locale, Translator } from "../i18n";
import { translatePlural } from "../i18n";

// Whose weekly plans a team read looked at.
//
// "Nothing due" and "not looked at" differ: a teammate whose plan could not be
// read adds no rows, and without this sentence a lead reads their silence as a
// week with nothing owed.

type PlanCoverage = components["schemas"]["WorklistPlanCoverage"];

/** True when some teammate's commitments are unknown rather than absent. */
export function planCoverageGap(coverage: PlanCoverage | undefined): boolean {
  return (
    coverage !== undefined &&
    (coverage.truncated || coverage.members.some((member) => !member.read))
  );
}

/**
 * The coverage as one sentence, or null when there is nothing to say: no
 * team read was made, or its roster was empty.
 *
 * Unread teammates are NAMED, because "4 of 5" sends a lead looking without
 * saying where.
 */
export function planCoverageText(
  coverage: PlanCoverage | undefined,
  locale: Locale,
  t: Translator,
): string | null {
  if (!coverage || coverage.members.length === 0) {
    return null;
  }
  const total = coverage.members.length;
  const unread = coverage.members
    .filter((member) => !member.read)
    .map((member) => member.display_name);
  const count = formatNumber(total, locale);
  // "All" would contradict a roster cut short, so a truncated read states its
  // figures as a share even when every plan it asked for was read.
  const head =
    unread.length === 0 && !coverage.truncated
      ? translatePlural(locale, "worklist.planCoverage.all", total, { count })
      : translatePlural(locale, "worklist.planCoverage.some", total, {
          count,
          read: formatNumber(total - unread.length, locale),
        });
  const names = new Intl.ListFormat(INTL_LOCALE[locale], {
    style: "long",
    type: "conjunction",
  }).format(unread);
  return [
    head,
    unread.length > 0 && t("worklist.planCoverage.unread", { names }),
    coverage.truncated && t("worklist.planCoverage.truncated"),
  ]
    .filter(Boolean)
    .join(" ");
}
