// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";
import { jurisdictionName } from "../../format/jurisdiction";
import type { Locale, Translator } from "../../i18n";
import type { MessageKey } from "../../i18n/en";

type RefusedRecipient = components["schemas"]["RefusedRecipient"];

const SOURCE_KEY: Partial<
  Record<NonNullable<RefusedRecipient["country_source"]>, MessageKey>
> = {
  contact_address: "compose.country.fromContact",
  employer_address: "compose.country.fromEmployer",
  lead_company_address: "compose.country.fromLeadCompany",
  sender_override: "compose.country.fromSender",
};

/**
 * The country a refusal was judged under and where it came from, for the
 * review rows. Undefined when the review names none, which an older server
 * also answers.
 */
export function refusalCountryText(
  refusal: RefusedRecipient,
  t: Translator,
  locale: Locale,
): string | undefined {
  if (refusal.country_source === "sender_cleared") {
    return t("compose.country.row", {
      detail: t("compose.country.clearedBySender"),
    });
  }
  if (!refusal.country) return undefined;
  const country = jurisdictionName(refusal.country, locale);
  const key = refusal.country_source
    ? SOURCE_KEY[refusal.country_source]
    : undefined;
  return t("compose.country.row", {
    detail: key ? t(key, { country }) : country,
  });
}
