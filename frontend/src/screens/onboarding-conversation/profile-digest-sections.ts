// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { MessageKey } from "../../i18n/en";
import type { CompanyFieldName } from "../onboarding";
import { type ReviewGroupKey, reviewGroups } from "./company-review-state";

// The article's words for the record's four groups: a reader skimming their
// own record reads "How they write" where a reader filling in the form reads
// "Positioning and sales context". Only the words are the article's; which
// field sits under which heading is `reviewGroups()`'s, so the article and
// the form can never file one field in two places.
const SECTION_LABELS: Readonly<Record<ReviewGroupKey, MessageKey>> = {
  identity: "ob.digest.section.identity",
  offer: "ob.digest.section.offer",
  customer: "ob.digest.section.customer",
  sales: "ob.digest.section.sales",
};

export function articleSections(): readonly Readonly<{
  key: ReviewGroupKey;
  labelKey: MessageKey;
  fields: readonly CompanyFieldName[];
}>[] {
  return reviewGroups().map((group) => ({
    ...group,
    labelKey: SECTION_LABELS[group.key],
  }));
}
