// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  attentionReport,
  HEALTH_AT,
  stoppedMailReport,
} from "../features/settings/systemhealth.fixtures";
import { defaultOperations } from "./operationsettings.fixtures";
import { jsonResponse, meRoute, type RouteMap } from "./story-utils";

export type HealthSessionAccess = "configure" | "diagnose" | "denied";

export function healthSessionRoutes(access: HealthSessionAccess): RouteMap {
  const admin = access === "configure";
  const jobs = access !== "denied";
  const routes: RouteMap = {
    "GET /me": meRoute(
      admin
        ? {
            job_health: ["read"],
            installation_settings: ["read", "update"],
            embedding_reindex: ["read", "update"],
            data_task: ["read"],
            ai_diagnostics: ["read"],
          }
        : jobs
          ? { job_health: ["read"] }
          : {},
      {
        roles: [admin ? "admin" : jobs ? "manager" : "rep"],
        seat: admin ? "full" : "read",
      },
    ),
    "GET /admin/system-health": () =>
      jsonResponse({
        ...stoppedMailReport,
        findings: jobs ? stoppedMailReport.findings : [],
        action_count: jobs ? stoppedMailReport.action_count : 0,
        assessment: jobs ? stoppedMailReport.assessment : "incomplete",
        complete: jobs && stoppedMailReport.complete,
        sources: stoppedMailReport.sources.map((source) =>
          admin || (jobs && source.permission === "job_health:read")
            ? source
            : {
                id: source.id,
                permission: source.permission,
                scope: source.scope,
                availability: "withheld",
                condition: "unknown",
                code: "permission_required",
              },
        ),
      }),
    "GET /admin/job-health": () =>
      jsonResponse({
        generated_at: attentionReport.generated_at,
        dead_window_hours: 24,
        kinds: [
          {
            kind: "capture_classify",
            queue: "default",
            fleet_wide: true,
            waiting: 0,
            running: 0,
            retrying: 0,
            dead: 12,
            discarded: 12,
            cancelled: 0,
            dead_recent: 12,

            condition: "unverified",
            condition_code: "queue_unverified",
            expectation_seconds: 3600,
          },
        ],
        recent_failures: [],
      }),
    "GET /admin/job-health/history": () =>
      jsonResponse({
        observed_at: attentionReport.generated_at,
        cutoff: attentionReport.generated_at,
        window_start: "2026-10-08T00:00:00Z",
        groups: [
          {
            kind: "capture_classify",
            state: "discarded",
            fleet_wide: true,
            count: 12,
            first_at: attentionReport.generated_at,
            latest_at: attentionReport.generated_at,
          },
        ],
        runs: Array.from({ length: 12 }, (_, i) => ({
          job_id: 4711 + i,
          kind: "capture_classify",
          state: "discarded",
          attempt: 3,
          max_attempts: 3,
          failed_at: HEALTH_AT,
          reason: "The AI provider is temporarily unavailable.",
          remedy:
            "Check provider access and availability before restarting the affected work.",
          failure_class: "provider_unavailable",
        })),
        has_more: false,
        population_may_have_changed: false,
        coverage: "retained_only",
      }),
    "GET /admin/capture-health": () =>
      jsonResponse({
        generated_at: HEALTH_AT,
        mailboxes: [],
        classifier: { pending: 0, unsure: 0, exhausted: 0 },
        held_meetings: { count: 0 },
        sweeps: [
          ...[
            "settled_thread_verdicts",
            "stranded_contacts",
            "filed_meeting_holds",
          ].map((sweep) => ({
            sweep,
            cadence_seconds: sweep === "settled_thread_verdicts" ? 600 : 86400,
            last_succeeded_at: HEALTH_AT,
            last_run: {
              outcome: "ok",
              finished_at: HEALTH_AT,
              processed: 4,
              cap_hit: false,
            },
          })),
        ],
      }),
    "GET /admin/extension-ingest-health": () =>
      jsonResponse({ generated_at: HEALTH_AT, window_days: 7, units: [] }),
    "GET /admin/automatic-work-health": () =>
      jsonResponse({
        generated_at: HEALTH_AT,
        days: 7,
        first_day: "2026-10-08",
        last_day: "2026-10-14",
        share_threshold: 0.1,
        small_sample_below: 10,
        kinds: [],
        colleagues: [],
        unattributed: 0,
      }),
    "GET /admin/recovery-health": () =>
      jsonResponse({
        generated_at: HEALTH_AT,
        recovery_target_seconds: 14400,
        data_loss_target_seconds: 3600,
        last_drill: {
          started_at: "2026-10-14T06:45:00Z",
          finished_at: HEALTH_AT,
          restored_to: "2026-10-14T06:05:00Z",
          outcome: "passed",
          operator: "Synthetic Operator",
          notes: "Demo restore verified.",
          recovery_seconds: 8100,
          data_loss_seconds: 2400,
          recovery_within_target: true,
          data_loss_within_target: true,
        },
      }),
    "GET /ai/provider-health": () =>
      jsonResponse({ providers: [], configured: [] }),
    "GET /admin/data-tasks": () => jsonResponse({ data: [], controls: [] }),
    "GET /embeddings/reindex/status": () =>
      jsonResponse({
        configured_identity: "demo/model@1024",
        populated_identity: "demo/model@1024",
        status: "idle",
        reindex_needed: false,
        entities_pending: 0,
        updated_at: HEALTH_AT,
      }),
    "GET /embeddings/reindex/identities": () =>
      jsonResponse({
        configured_identity: "demo/model@1024",
        populated_identity: "demo/model@1024",
        reindex_needed: false,
      }),
    "GET /installation/settings": () =>
      jsonResponse({ operations: defaultOperations }),
  };
  for (const key of Object.keys(routes)) {
    if (key === "GET /me") continue;
    const jobRead =
      key.startsWith("GET /admin/") && key !== "GET /admin/data-tasks";
    if (!admin && !(jobs && jobRead)) {
      routes[key] = () =>
        jsonResponse({ title: "Forbidden", status: 403 }, 403);
    }
  }
  return routes;
}
