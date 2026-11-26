// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { formatNumber } from "../../format/format";
import type { Locale, PluralTranslator, Translator } from "../../i18n";
import type { Company, Company360View } from "./companies";
import { wholeCount } from "./companyrailshared";

/**
 * The question before a company is archived. It says what happens to the people
 * and the pipeline left on it. The contacts lose it as their employer, and the
 * open deals stay open. The counts come from the page's own 360 read.
 * A count it could not take whole is not guessed, and the sentence then names
 * no number.
 */
export function companyArchiveConfirm(
  t: Translator,
  plural: PluralTranslator,
  locale: Locale,
  company: Pick<Company, "display_name">,
  view: Company360View | undefined,
): string {
  const contacts = view?.contacts?.page.total ?? wholeCount(view?.contacts);
  const openDeals = view?.state_strip?.commercial?.open_count;
  const parts = [t("co.archiveConfirm.ask", { name: company.display_name })];
  if (contacts === undefined || openDeals === undefined) {
    parts.push(t("co.archiveConfirm.unknown"));
  } else {
    if (contacts > 0) {
      parts.push(
        plural("co.archiveConfirm.contacts", contacts, {
          count: formatNumber(contacts, locale),
        }),
      );
    }
    if (openDeals > 0) {
      parts.push(
        plural("co.archiveConfirm.deals", openDeals, {
          count: formatNumber(openDeals, locale),
        }),
      );
    }
  }
  parts.push(t("co.archiveConfirm.restore"));
  return parts.join(" ");
}
