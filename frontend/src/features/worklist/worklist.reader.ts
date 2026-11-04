// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { summaryFieldsLine } from "../../design-system/approvalsummary";
import { INTL_LOCALE } from "../../format/format";
import { type Locale, type Translator, translatePlural } from "../../i18n";
import { summaryWords } from "../../lib/approvalpreviewfields";
import {
  actorAttribution,
  HUMAN_ACTOR_PREFIX,
  isAuditActorType,
} from "../../lib/auditactor";
import type { Viewer } from "../../lib/worklistcopy";
import {
  domainQuestionHeading,
  domainQuestionReason,
} from "./worklist.domaincopy";
import type { WorklistItem } from "./worklist.queries";

export function noticeDetail(
  item: WorklistItem,
  viewer: Viewer | undefined,
  t: Translator,
): string | undefined {
  if (item.summary_parts) {
    return (
      summaryFieldsLine(item.summary_parts, summaryWords(t)) ?? item.detail
    );
  }
  if (item.source === "domain_question" && item.domain_reason_code) {
    const reason = domainQuestionReason(
      t,
      item.domain_reason_code,
      item.domain_context,
    );
    if (!reason) return item.detail;
    // The question names the contact; a vendor question also carries the
    // site triage's own sentence.
    return [
      domainQuestionHeading(t, item.id, item.domain_context),
      reason,
      item.domain_note,
    ]
      .filter(Boolean)
      .join(" ");
  }
  if (item.source !== "notice") return item.detail;
  const origin = item.notice_origin;
  if (!origin)
    return [
      item.detail,
      item.kind === "automation" ? t("home.change.unknown") : undefined,
    ]
      .filter(Boolean)
      .join(" ");
  const actor = noticeActor(origin, viewer, t);
  const change = origin.stage_change;
  const detail = change
    ? `${change.from_name || t("home.change.stageUnknown")} → ${change.to_name || t("home.change.stageUnknown")}`
    : item.detail;
  return [detail, t("home.change.by", { actor })].filter(Boolean).join(" · ");
}

function noticeActor(
  origin: NonNullable<WorklistItem["notice_origin"]>,
  viewer: Viewer | undefined,
  t: Translator,
): string {
  const kind = origin.actor_type;
  if (!isAuditActorType(kind)) return t("home.change.unidentified");
  const attribution = actorAttribution(
    {
      ...origin,
      actor_type: kind,
      actor_id:
        kind === "human" && !origin.actor_id.startsWith(HUMAN_ACTOR_PREFIX)
          ? `${HUMAN_ACTOR_PREFIX}${origin.actor_id}`
          : origin.actor_id,
    },
    viewer?.id,
  );
  const label =
    attribution.name ??
    (attribution.labelKey ? t(attribution.labelKey) : attribution.identifier);
  const qualifier =
    attribution.qualifierName ??
    (attribution.qualifierKey ? t(attribution.qualifierKey) : undefined);
  return [label, qualifier].filter(Boolean).join(" · ");
}

export function sharedResponsibility(
  item: WorklistItem,
  locale: Locale,
  t: Translator,
): string | null {
  const owner = item.owner;
  if (owner?.kind !== "shared" || !owner.users?.length) return null;
  const names = owner.users.flatMap((user) =>
    user.label?.trim() ? [user.label.trim()] : [],
  );
  if (names.length === owner.users.length) {
    return t("worklist.owner.shared", {
      names: new Intl.ListFormat(INTL_LOCALE[locale], {
        style: "long",
        type: "conjunction",
      }).format(names),
    });
  }
  return translatePlural(
    locale,
    "worklist.owner.sharedCount",
    owner.users.length,
    { count: String(owner.users.length) },
  );
}
