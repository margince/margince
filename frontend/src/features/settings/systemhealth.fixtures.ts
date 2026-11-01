// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";

type Report = components["schemas"]["SystemHealth"];
export const HEALTH_AT = "2026-10-14T09:00:00Z";
export const healthReport: Report = {
  generated_at: HEALTH_AT,
  assessment: "incomplete",
  complete: false,
  action_count: 0,
  sources: [
    {
      id: "jobs",
      permission: "job_health:read",
      scope: "workspace",
      availability: "unknown",
      condition: "unknown",
      code: "queue_checked",
      observed_at: HEALTH_AT,
    },
    {
      id: "capture",
      permission: "job_health:read",
      scope: "workspace",
      availability: "available",
      condition: "clear",
      code: "capture_checked",
      observed_at: HEALTH_AT,
    },
    {
      id: "intake",
      permission: "job_health:read",
      scope: "workspace",
      availability: "available",
      condition: "clear",
      code: "intake_no_refusals",
      observed_at: HEALTH_AT,
    },
    {
      id: "recovery",
      permission: "job_health:read",
      scope: "installation",
      availability: "unknown",
      condition: "unknown",
      code: "recovery_missing",
    },
    {
      id: "automatic",
      permission: "job_health:read",
      scope: "workspace",
      availability: "available",
      condition: "clear",
      code: "automatic_checked",
      observed_at: HEALTH_AT,
    },
    {
      id: "search",
      permission: "embedding_reindex:read",
      scope: "installation",
      availability: "available",
      condition: "clear",
      code: "search_checked",
      observed_at: HEALTH_AT,
    },
    {
      id: "updates",
      permission: "data_task:read",
      scope: "workspace",
      availability: "available",
      condition: "clear",
      code: "updates_checked",
      observed_at: HEALTH_AT,
    },
    {
      id: "providers",
      permission: "ai_diagnostics:read",
      scope: "installation",
      availability: "unknown",
      condition: "unknown",
      code: "providers_unobserved",
    },
  ],
  findings: [],
};
export const overdueFinding: components["schemas"]["HealthFinding"] = {
  id: "jobs:queue_overdue",
  source: "jobs",
  code: "queue_overdue",
  condition: "technical",
  requires_action: true,
  job_kinds: ["capture_classify"],
  urgency: 0,
  scope: "workspace",
  impact: "possible",
  responsible: "operator",
  recovery: "jobs",
  count: 12,
  unit: "runs",
  observed_at: HEALTH_AT,
  action: { destination: "diagnostics", available: true },
};
export const attentionReport: Report = {
  ...healthReport,
  assessment: "attention",
  action_count: 1,
  sources: healthReport.sources.map((source) =>
    source.id === "jobs"
      ? {
          ...source,
          availability: "available",
          condition: "technical",
          code: "queue_overdue",
        }
      : source,
  ),
  findings: [overdueFinding],
};

export const stoppedMailReport: Report = {
  ...attentionReport,
  complete: true,
  findings: [
    {
      ...overdueFinding,
      id: "jobs:queue_unverified",
      code: "queue_unverified",
      urgency: 2,
      condition: "unverified",
      scope: "shared",
    },
  ],
  sources: attentionReport.sources.map((source) => {
    if (source.id === "jobs")
      return { ...source, condition: "unverified", code: "queue_unverified" };
    if (source.id === "recovery")
      return {
        ...source,
        availability: "available",
        condition: "clear",
        code: "recovery_verified",
        observed_at: HEALTH_AT,
      };
    if (source.id === "providers")
      return {
        ...source,
        availability: "not_configured",
        code: "providers_not_configured",
      };
    return source;
  }),
};
