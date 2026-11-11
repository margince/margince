// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  type DecisionCardLabels,
  DecisionProposal,
} from "../../design-system/decisioncard";
import type { Translator } from "../../i18n";
import { useLocale, useT } from "../../i18n";
import { summaryWords } from "../../lib/approvalpreviewfields";
import type { Approval } from "../../lib/approvals.queries";
import { useViewerZone } from "../../lib/viewerzone";
import { resolveDisplay, stagedDayFormatter } from "./approvaldisplay";
import { useApprovalNames } from "./approvalresponsibility";

/**
 * The words a decision card needs on a queue surface. No `skip`: a queue is
 * worked to the end. No `showMore`: the row's detail dialog shows the whole
 * draft.
 */
export function rowLabels(t: Translator): DecisionCardLabels {
  return {
    accept: t("trust.accept"),
    edit: t("trust.edit"),
    reject: t("decision.reject"),
    expired: t("decision.expired"),
    draftSubject: t("decision.draftSubject"),
    draftBody: t("decision.draftBody"),
    noContent: t("common.empty"),
    loading: t("decision.detailLoading"),
    summaryWords: summaryWords(t),
  };
}

/** What a kind shows of its proposal, in the reader's words. */
export function useApprovalDisplay(
  approval: Approval,
  preview: Approval["preview"],
) {
  const t = useT();
  const { locale } = useLocale();
  const names = useApprovalNames([approval]);
  const zone = useViewerZone();
  return resolveDisplay(
    approval.kind,
    approval.proposed_change ?? {},
    t,
    stagedDayFormatter(locale, zone),
    names,
    approval.requested_decider_id,
    approval.assignee_inactive,
    preview,
    locale,
    zone,
  );
}

/**
 * What a staged proposal would do, read the way its card reads it. Who is asked
 * to decide is the card's question, not part of what the proposal does.
 */
export function ApprovalProposal({
  approval,
  className,
}: Readonly<{ approval: Approval; className?: string }>) {
  const t = useT();
  const display = useApprovalDisplay(approval, approval.preview).filter(
    (entry) => entry.field !== "requested_decider",
  );
  return (
    <DecisionProposal
      approval={approval}
      labels={rowLabels(t)}
      display={display}
      className={className}
    />
  );
}
