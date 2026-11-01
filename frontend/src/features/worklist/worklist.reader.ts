// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  summaryFieldsLine,
  summarySentence,
} from "../../design-system/approvalsummary";
import { INTL_LOCALE } from "../../format/format";
import { type Locale, type Translator, translatePlural } from "../../i18n";
import { summaryWords } from "../../lib/approvalpreviewfields";
import {
  actorAttribution,
  HUMAN_ACTOR_PREFIX,
  isAuditActorType,
} from "../../lib/auditactor";
import { domainReasonLabels } from "./worklist.domainreason";
import type { WorklistItem } from "./worklist.queries";

type Viewer = Readonly<{ id: string; display_name: string }>;

/** Responsibility comes from the assigned user ID; the stored promise stays verbatim. */
export function readerTask(
  item: WorklistItem,
  viewer: Viewer | undefined,
  t: Translator,
): WorklistItem {
  if (
    item.source !== "task" ||
    !viewer ||
    item.owner?.kind !== "user" ||
    item.owner.id !== viewer.id ||
    !item.title
  )
    return item;
  const prefix = `${viewer.display_name.trim()} will `;
  if (!viewer.display_name.trim() || !item.title.startsWith(prefix))
    return item;
  return {
    ...item,
    title: t("home.task.yours", { action: item.title.slice(prefix.length) }),
  };
}

/**
 * The row's own reading of an approval: the sentence as the title, because
 * noticeDetail draws the values under it. Other surfaces keep the whole title.
 */
export function sentenceTitled(item: WorklistItem): WorklistItem {
  const sentence = summarySentence({
    summary: item.title,
    summary_parts: item.summary_parts,
  });
  return item.summary_parts && sentence ? { ...item, title: sentence } : item;
}

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
    const label = domainReasonLabels[item.domain_reason_code];
    if (!label) return item.detail;
    // A vendor question also carries the site triage's own sentence.
    return [t(label), item.domain_note].filter(Boolean).join(" ");
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
