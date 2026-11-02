// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";
import type { MessageKey } from "../../i18n/en";

import { healthImpacts } from "./systemhealth.copy";

type Finding = components["schemas"]["HealthFinding"];
const effects: Readonly<Record<string, MessageKey>> = {
  queue_overdue: "health.effect.jobs",
  queue_unverified: "health.effect.jobs",
  capture_uncertain: "health.effect.senderReview",
  capture_exhausted: "health.effect.senderReview",
  capture_owner_review: "health.effect.senderReview",
  capture_thread_uncertain: "health.effect.heldMail",
  capture_unread: "health.effect.unreadMail",
  capture_repair_unverified: "health.effect.captureRepair",
  intake_refused: "health.effect.intake",
  providers_blocked: "health.effect.providers",
  search_reindex: "health.effect.search",
  updates_failed: "health.effect.updates",
  updates_review: "health.effect.updates",
  recovery_unverified: "health.effect.recovery",
  automatic_routing: "health.effect.automatic",
  read_failed: "health.effect.readFailed",
};

export function findingEffectKey(finding: Finding): MessageKey {
  if (
    finding.source === "jobs" &&
    !finding.job_kinds_truncated &&
    finding.job_kinds?.length === 1 &&
    finding.job_kinds[0] === "capture_classify"
  )
    return "health.effect.mailSorting";
  return effects[finding.code] ?? "health.effect.unknown";
}

export function findingNextStepKey(finding: Finding): MessageKey {
  if (finding.code === "read_failed") return "health.readFailedNext";
  if (finding.responsible === "mailbox_owner") return "health.ownerHandoff";
  if (finding.code === "queue_unverified") return "health.stoppedNext";
  return healthImpacts[finding.source] ?? "health.operatorHandoff";
}
