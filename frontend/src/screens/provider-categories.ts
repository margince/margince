// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { INTL_LOCALE } from "../format/format";
import type { Locale } from "../i18n";
import type { MessageKey } from "../i18n/en";

/** The categories a provider names, as words a reader knows.
 *
 *  The wire carries the provider's own vocabulary (`professional_email`,
 *  `linkedin_profile`), which is a key rather than a phrase — printed raw it
 *  puts `linkedin_profile, current_employment` in front of a rep.
 *
 *  A category this build has no word for keeps the provider's own: the
 *  vocabulary belongs to the provider, so an installation can declare one this
 *  frontend has never seen, and showing their word beats showing nothing.
 *
 *  Shared by the two screens that name a category — the settings card, where an
 *  admin decides what may be bought, and the contact panel, where somebody buys
 *  it. Two copies would drift, and the drift a reader would see is the settings
 *  switch and the buy button calling one purchase two different things.
 */
const CATEGORY_LABELS: Record<string, MessageKey> = {
  professional_email: "provider.category.professionalEmail",
  personal_email: "provider.category.personalEmail",
  mobile: "provider.category.mobile",
  linkedin_profile: "provider.category.linkedin",
  current_employment: "provider.category.currentEmployment",
  job_history: "provider.category.jobHistory",
};

/** One category's name, or the provider's own word when this build has none. */
export function categoryName(
  category: string,
  t: (key: MessageKey) => string,
): string {
  const label = CATEGORY_LABELS[category];
  return label ? t(label) : category;
}

/** Several categories, as one comma-joined phrase. For a LIST of categories —
 *  what a run asked for, what came back empty. */
export function categoryNames(
  categories: readonly string[],
  t: (key: MessageKey) => string,
): string {
  return categories.map((category) => categoryName(category, t)).join(", ");
}

/** Several categories a single press buys TOGETHER, joined by the locale's own
 *  conjunction: "work email and mobile number".
 *
 *  A comma is what categoryNames gives, and on a button it reads as a list the
 *  press might pick from rather than a pair it buys as one. A reader who
 *  scanned "Buy work email, mobile number · 2 credits" as the work-email
 *  button and got a mobile number too is the case this exists for — the price
 *  beside it was already the pair's, so nobody was overcharged, but nobody was
 *  told either.
 *
 *  Intl.ListFormat rather than a conjunction per language: German's "und" and
 *  Vietnamese's "và" are the platform's to know, and an i18n key would ask a
 *  translator to re-derive what the runtime already has right.
 */
export function categoryNamesTogether(
  categories: readonly string[],
  t: (key: MessageKey) => string,
  locale: Locale,
): string {
  const names = categories.map((category) => categoryName(category, t));
  return new Intl.ListFormat(INTL_LOCALE[locale], {
    style: "long",
    type: "conjunction",
  }).format(names);
}
