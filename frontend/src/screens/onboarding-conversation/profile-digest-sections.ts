// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { MessageKey } from "../../i18n/en";
import type { CompanyFieldName } from "../onboarding";
import {
  CUSTOMER_FIELDS,
  LEGAL_IDENTITY_FIELDS,
  OFFER_FIELDS,
  SALES_FIELDS,
} from "../onboarding";

// The article's own grouping of the record's fields, read as prose sections
// rather than as the form's four clusters. It is the SAME four groups
// `reviewFields()` already flattens and confirm-card.tsx's `reviewGroups()`
// already names for the form — this is not a second idea of what belongs
// where, only a second set of words for it, because a reader skimming their
// own record reads "How they write" where a reader filling in a form reads
// "Positioning and sales context". confirm-card.tsx keeps its own map rather
// than this one importing it: it sits on the same `../onboarding` import
// cycle this file does, and a cross-import between two modules already on
// that cycle is exactly the crash risk company-review-state.ts's own comment
// warns against.
//
// A function rather than a module-level constant for the same reason
// `reviewGroups()` is one: this module and `../onboarding` sit on an import
// cycle (`../onboarding` → `onboarding-conversation/index` → `company-act` →
// `profile-digest` → this file → `../onboarding`), so the field arrays only
// exist by the time a render calls this, never at module load.
export function articleSections(): readonly Readonly<{
  key: string;
  labelKey: MessageKey;
  fields: readonly CompanyFieldName[];
}>[] {
  return [
    {
      key: "identity",
      labelKey: "ob.digest.section.identity",
      fields: LEGAL_IDENTITY_FIELDS,
    },
    {
      key: "offer",
      labelKey: "ob.digest.section.offer",
      fields: OFFER_FIELDS,
    },
    {
      key: "customer",
      labelKey: "ob.digest.section.customer",
      fields: CUSTOMER_FIELDS,
    },
    {
      key: "sales",
      labelKey: "ob.digest.section.sales",
      fields: SALES_FIELDS,
    },
  ];
}
