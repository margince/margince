// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";
import type { PluralTranslator, Translator } from "../../i18n";
import type { MessageKey } from "../../i18n/en";

export type DomainQuestionContext =
  components["schemas"]["DomainQuestionContext"];
type SuggestionBasis =
  components["schemas"]["DomainCompanySuggestion"]["basis"];

// One company question, as both the row and the review drawer read it.
export type DomainQuestionView = Readonly<{
  domain: string;
  reasonCode: string | undefined;
  context: DomainQuestionContext | undefined;
}>;

// The question names a person the reader may see. Failing that, it names the
// address that wrote, and failing that, the domain.
export function domainQuestionHeading(
  { domain, context }: DomainQuestionView,
  t: Translator,
  plural: PluralTranslator,
): string {
  const first = context?.contacts[0];
  if (first) {
    const others = Math.max(context.contact_count, context.contacts.length) - 1;
    return others > 0
      ? plural("worklist.domainQuestion.headingMany", others, {
          name: first.name,
          count: String(others),
        })
      : t("worklist.domainQuestion.heading", { name: first.name });
  }
  if (context?.sender_address) {
    return t("worklist.domainQuestion.headingAddress", {
      address: context.sender_address,
    });
  }
  return t("worklist.domainQuestion.headingDomain", { domain });
}

const REASON: Readonly<Record<string, MessageKey>> = {
  forwarded_identity: "worklist.domainQuestion.reason.forwarded",
  near_duplicate: "worklist.domainQuestion.reason.nearDuplicate",
  stale_evidence: "worklist.domainQuestion.reason.staleEvidence",
  vendor_verdict: "worklist.domainQuestion.reason.vendor",
};

// Why Margince is asking. An unevidenced question tells a site never read
// from one read in vain, because the two earn different trust.
export function domainQuestionReasonKey({
  reasonCode,
  context,
}: DomainQuestionView): MessageKey {
  if (reasonCode && REASON[reasonCode]) {
    return REASON[reasonCode];
  }
  return context?.site_unread
    ? "worklist.domainQuestion.reason.unread"
    : "worklist.domainQuestion.reason.noCompany";
}

export const SUGGESTION_BASIS: Readonly<Record<SuggestionBasis, MessageKey>> = {
  redirect_target: "worklist.domainQuestion.basis.redirectTarget",
  contact_employer: "worklist.domainQuestion.basis.contactEmployer",
  name_twin: "worklist.domainQuestion.basis.nameTwin",
};

// What Confirm does, said on the button's caption: the whole domain and every
// contact on it move to the company, as matching does.
export function confirmEffect(
  { domain, context }: DomainQuestionView,
  company: string,
  t: Translator,
  plural: PluralTranslator,
): string {
  const count = context?.contact_count ?? 0;
  return count > 0
    ? plural("worklist.domainQuestion.confirmEffect", count, {
        domain,
        company,
        count: String(count),
      })
    : t("worklist.domainQuestion.confirmEffectNone", { domain, company });
}
