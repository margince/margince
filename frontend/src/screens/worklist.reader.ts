// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Translator } from "../i18n";
import { actorAttribution, HUMAN_ACTOR_PREFIX } from "./audit";
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

export function noticeDetail(
  item: WorklistItem,
  viewer: Viewer | undefined,
  t: Translator,
): string | undefined {
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
  if (
    kind !== "human" &&
    kind !== "agent" &&
    kind !== "connector" &&
    kind !== "system" &&
    kind !== "buyer"
  )
    return t("home.change.unidentified");
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
