// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";
import { formatNumber } from "../../format/format";
import type { Translator, Words } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { domainReasonLabels } from "./worklist.domainreason";

export type DomainQuestionContext =
  components["schemas"]["DomainQuestionContext"];
export type DomainSuggestion =
  components["schemas"]["DomainQuestionSuggestion"];

// The question names a sender the reader may see. It falls back to the
// sender's address, then to the domain.
export function domainQuestionHeading(
  t: Translator,
  domain: string,
  context: DomainQuestionContext | undefined,
): string {
  const first = context?.contacts[0];
  if (first) {
    const others = (context?.contact_count ?? 1) - 1;
    if (others <= 0)
      return t("worklist.domainQuestion.heading", { name: first.name });
    if (others === 1)
      return t("worklist.domainQuestion.headingPair", { name: first.name });
    return t("worklist.domainQuestion.headingMany", {
      name: first.name,
      others: String(others),
    });
  }
  if (context?.sender_address && !context.hidden_senders)
    return t("worklist.domainQuestion.heading", {
      name: context.sender_address,
    });
  return t("worklist.domainQuestion.headingDomain", { domain });
}

const REASONS: Partial<Record<string, MessageKey>> = {
  forwarded_identity: "worklist.domainQuestion.reason.forwarded",
  vendor_verdict: "worklist.domainQuestion.reason.vendor",
  near_duplicate: "worklist.domainQuestion.reason.nearDuplicate",
  stale_evidence: "worklist.domainQuestion.reason.stale",
};

// Why Margince asks, in the reader's words. An unevidenced question says
// whether the site was read at all, so a failed read is not told as an
// inspection that found nothing.
export function domainQuestionReason(
  t: Translator,
  reasonCode: string | undefined,
  context: DomainQuestionContext | undefined,
): string | undefined {
  if (!reasonCode) return undefined;
  if (reasonCode === "unevidenced") {
    return t(
      context?.site_read === "unread"
        ? "worklist.domainQuestion.reason.unread"
        : "worklist.domainQuestion.reason.read",
    );
  }
  const key = REASONS[reasonCode] ?? domainReasonLabels[reasonCode];
  return key ? t(key) : undefined;
}

const BASES: Record<DomainSuggestion["basis"], MessageKey> = {
  name_twin: "worklist.domainQuestion.basis.name_twin",
  redirect_target: "worklist.domainQuestion.basis.redirect_target",
  contact_employer: "worklist.domainQuestion.basis.contact_employer",
};

export function suggestionBasis(
  t: Translator,
  suggestion: DomainSuggestion,
): string {
  return t(BASES[suggestion.basis]);
}

// What confirming a company does: the domain and every captured contact on it
// are linked, so the sentence names both.
export function confirmEffect(
  { t, plural, locale }: Words,
  domain: string,
  company: string,
  context: DomainQuestionContext | undefined,
): string {
  const count = context?.contact_count ?? 0;
  if (count === 0)
    return t("worklist.domainQuestion.confirmEffectNone", { domain, company });
  return plural("worklist.domainQuestion.confirmEffect", count, {
    domain,
    company,
    count: formatNumber(count, locale),
  });
}
