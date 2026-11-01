// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";
import type { PluralBase } from "../../i18n";
import type { MessageKey } from "../../i18n/en";

type Source = components["schemas"]["HealthSource"];
type Finding = components["schemas"]["HealthFinding"];

export const healthScopes: Readonly<Record<Finding["scope"], MessageKey>> = {
  workspace: "health.workspaceScope",
  shared: "health.sharedScope",
  installation: "settings.scope.installation",
};

export const healthSourceLabels: Readonly<Record<string, MessageKey>> = {
  jobs: "health.source.jobs",
  capture: "health.source.capture",
  intake: "health.source.intake",
  providers: "health.source.providers",
  search: "health.source.search",
  updates: "health.source.updates",
  recovery: "health.source.recovery",
  automatic: "health.source.automatic",
};
export const healthCodes: Readonly<Record<string, MessageKey>> = {
  queue_checked: "health.code.queue_checked",
  queue_processing: "health.code.queue_processing",
  queue_overdue: "health.code.queue_overdue",
  queue_unverified: "health.code.queue_unverified",
  queue_retrying: "health.code.queue_retrying",
  queue_expectation_unknown: "health.code.queue_expectation_unknown",
  capture_thread_processing: "health.code.capture_thread_processing",
  capture_thread_uncertain: "health.code.capture_thread_uncertain",
  capture_checked: "health.code.capture_checked",
  capture_processing: "health.code.capture_processing",
  capture_uncertain: "health.code.capture_uncertain",
  capture_exhausted: "health.code.capture_exhausted",
  capture_owner_review: "health.code.capture_owner_review",
  capture_unread: "health.code.capture_unread",
  capture_held: "health.code.capture_held",
  capture_repair_unverified: "health.code.capture_repair_unverified",
  intake_no_refusals: "health.code.intake_no_refusals",
  intake_refused: "health.code.intake_refused",
  providers_unobserved: "health.code.providers_unobserved",
  providers_not_configured: "health.code.providers_not_configured",
  providers_blocked: "health.code.providers_blocked",
  search_checked: "health.code.search_checked",
  search_unverified: "health.code.search_unverified",
  search_processing: "health.code.search_processing",
  search_reindex: "health.code.search_reindex",
  search_not_configured: "health.code.search_not_configured",
  updates_checked: "health.code.updates_checked",
  updates_processing: "health.code.updates_processing",
  updates_failed: "health.code.updates_failed",
  updates_retrying: "health.code.updates_retrying",
  updates_review: "health.code.updates_review",
  recovery_missing: "health.code.recovery_missing",
  recovery_verified: "health.code.recovery_verified",
  recovery_unverified: "health.code.recovery_unverified",
  automatic_checked: "health.code.automatic_checked",
  automatic_routing: "health.code.automatic_routing",
  withheld: "health.withheld",
  read_failed: "health.failed",
  unknown: "health.unknown",
};
export const healthAvailability: Record<Source["availability"], MessageKey> = {
  available: "health.availability.available",
  unknown: "health.unknown",
  stale: "health.stale",
  failed: "health.failed",
  withheld: "health.withheld",
  not_configured: "systemHealth.notSetUp",
  not_monitored: "health.notMonitored",
};
export const healthConditions: Record<Source["condition"], MessageKey> = {
  clear: "health.condition.clear",
  processing: "health.condition.processing",
  retrying: "health.condition.retrying",
  waiting_human: "health.condition.waiting_human",
  technical: "health.condition.technical",
  unverified: "health.condition.unverified",
  recovered: "health.condition.recovered",
  unknown: "health.unknown",
};
export const healthResponsible: Record<Finding["responsible"], MessageKey> = {
  you: "health.responsible.you",
  mailbox_owner: "health.responsible.mailbox_owner",
  administrator: "health.responsible.administrator",
  operator: "health.responsible.operator",
};
export const healthRecovery: Readonly<Record<string, MessageKey>> = {
  jobs: "health.recovery.jobs",
  capture: "health.recovery.capture",
  intake: "health.recovery.intake",
  providers: "health.recovery.providers",
  search: "health.recovery.search",
  updates: "health.recovery.updates",
  recovery: "health.recovery.recovery",
  automatic: "health.recovery.automatic",
};

export const healthImpacts: Readonly<Record<string, MessageKey>> = {
  jobs: "health.impact.jobs",
  capture: "health.impact.capture",
  intake: "health.impact.intake",
  providers: "health.impact.providers",
  search: "health.impact.search",
  updates: "health.impact.updates",
  recovery: "health.impact.recovery",
  automatic: "health.impact.automatic",
};

export function diagnosticSummary(
  capability: string,
  code: string,
  observedAt: string | undefined,
  runIDs: readonly number[] = [],
): string {
  return JSON.stringify({
    capability,
    condition_code: code,
    observed_at: observedAt,
    run_ids: runIDs,
  });
}

export const healthMeasures: Readonly<
  Record<NonNullable<Finding["unit"]>, PluralBase>
> = {
  runs: "health.measure.runs",
  threads: "health.measure.threads",
  mailboxes: "health.measure.mailboxes",
  records: "health.measure.records",
  updates: "health.measure.updates",
  items: "health.measure.items",
};
