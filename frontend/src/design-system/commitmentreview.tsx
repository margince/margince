// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import { formatDateTime } from "../format/format";
import { useLocale, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import "./commitmentreview.css";
import { Disclosure } from "./disclosure";
import { Heading } from "./heading";
import { Panel, PanelBody, PanelRow } from "./panel";

type Admission = components["schemas"]["CommitmentAdmission"];
const REASONS: Readonly<Record<string, MessageKey>> = {
  historical_without_current_deadline: "commitment.review.historical",
  before_mailbox_history_baseline: "commitment.review.beforeBaseline",
  later_context_not_assessed: "commitment.review.incomplete",
  later_context_not_read: "commitment.review.incomplete",
  source_text_incomplete: "commitment.review.sourceIncomplete",
  source_text_unassessed: "commitment.review.sourceUnassessed",
  source_time_unknown: "commitment.review.unknownDate",
  source_time_contradicted: "commitment.review.conflictingDates",
  meeting_cancelled: "commitment.review.canceledMeeting",
  meeting_not_yet_held: "commitment.review.pendingMeeting",
  source_unavailable: "commitment.review.unavailable",
  semantic_completed: "commitment.review.completed",
  semantic_conditional: "commitment.review.conditional",
  semantic_optional: "commitment.review.optional",
  semantic_request: "commitment.review.request",
  semantic_unclear: "commitment.review.unclear",
  semantic_unassessed: "commitment.review.unclear",
  promiser_not_assignable: "commitment.review.noPromiser",
};

export function CommitmentReview({
  decisions,
  zone,
}: Readonly<{ decisions?: readonly Admission[]; zone: string }>) {
  const t = useT();
  const { locale } = useLocale();
  const withheld =
    decisions?.filter((decision) => !decision.current_work) ?? [];
  if (withheld.length === 0) return null;
  return (
    <Disclosure
      className="commitment-review"
      summary={t("commitment.review.title")}
    >
      <Panel>
        <PanelBody>
          <p>{t("commitment.review.detail")}</p>
        </PanelBody>
        {withheld.map((decision) => (
          <PanelRow key={decision.locator}>
            <Heading as="h4" size="small">
              {decision.summary}
            </Heading>
            <p>
              {t(REASONS[decision.outcome] ?? "commitment.review.unavailable")}
            </p>
            {decision.source_quote && (
              <blockquote>{decision.source_quote}</blockquote>
            )}
            {decision.meaning?.condition && (
              <p>
                {t("commitment.review.conditionWords", {
                  words: decision.meaning.condition,
                })}
              </p>
            )}
            {decision.meaning?.deadline_quote && (
              <p>
                {t("commitment.review.deadlineWords", {
                  words: decision.meaning.deadline_quote,
                })}
              </p>
            )}
            <p className="t-caption">
              {t("commitment.review.evaluated", {
                when: formatDateTime(decision.evaluated_at, locale, zone),
              })}
            </p>
          </PanelRow>
        ))}
      </Panel>
    </Disclosure>
  );
}
