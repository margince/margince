// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { unwrap } from "../../lib/problem";

type Finding = components["schemas"]["HealthFinding"];

export async function readJobReport(finding: Finding, signal: AbortSignal) {
  const kind =
    finding.job_kinds?.length === 1 ? finding.job_kinds[0] : undefined;
  const [healthResponse, historyResponse] = await Promise.all([
    api.GET("/admin/job-health", { signal }),
    api.GET("/admin/job-health/history", {
      signal,
      params: {
        query: {
          days: 7,
          kind,
          state: finding.code === "queue_unverified" ? "discarded" : undefined,
        },
      },
    }),
  ]);
  const health = unwrap(healthResponse);
  const history = unwrap(historyResponse);
  return JSON.stringify(
    {
      source: "jobs",
      condition_code: finding.code,
      observed_at: health.generated_at,
      queues: health.kinds
        .filter((row) => !kind || row.kind === kind)
        .map((row) => ({
          kind: row.kind,
          queue: row.queue,
          fleet_wide: row.fleet_wide,
          condition: row.condition,
          condition_code: row.condition_code,
          waiting: row.waiting,
          running: row.running,
          retrying: row.retrying,
          discarded: row.discarded,
          cancelled: row.cancelled,
          oldest_waiting_age_seconds: row.oldest_waiting_age_seconds,
          expectation_seconds: row.expectation_seconds,
        })),
      history: {
        coverage: history.coverage,
        window_start: history.window_start,
        cutoff: history.cutoff,
        has_more: history.has_more,
        groups: history.groups.map((group) => ({
          kind: group.kind,
          state: group.state,
          fleet_wide: group.fleet_wide,
          count: group.count,
          first_at: group.first_at,
          latest_at: group.latest_at,
        })),
        runs: history.runs.map(jobRunEvidence),
      },
    },
    null,
    2,
  );
}

export async function readHealthReport(
  finding: Finding,
  signal: AbortSignal,
): Promise<string> {
  if (finding.source === "jobs") return readJobReport(finding, signal);
  switch (finding.source) {
    case "capture": {
      const data = unwrap(await api.GET("/admin/capture-health", { signal }));
      return JSON.stringify(
        {
          generated_at: data.generated_at,
          classifier: {
            pending: data.classifier.pending,
            unsure: data.classifier.unsure,
            exhausted: data.classifier.exhausted,
            oldest_pending_age_seconds:
              data.classifier.oldest_pending_age_seconds,
          },
          held_meetings: {
            count: data.held_meetings.count,
            oldest_age_seconds: data.held_meetings.oldest_age_seconds,
          },
          sweeps: data.sweeps.map((sweep) => ({
            sweep: sweep.sweep,
            cadence_seconds: sweep.cadence_seconds,
            last_succeeded_at: sweep.last_succeeded_at,
            last_run: sweep.last_run && {
              outcome: sweep.last_run.outcome,
              finished_at: sweep.last_run.finished_at,
              processed: sweep.last_run.processed,
              cap_hit: sweep.last_run.cap_hit,
              error_class: sweep.last_run.error_class,
            },
          })),
        },
        null,
        2,
      );
    }
    case "intake": {
      const data = unwrap(
        await api.GET("/admin/extension-ingest-health", { signal }),
      );
      return JSON.stringify(
        {
          generated_at: data.generated_at,
          window_days: data.window_days,
          units: data.units.map((unit) => ({
            unit: unit.unit,
            refused: unit.refused,
            last_refused_at: unit.last_refused_at,
            refusals: unit.refusals.map((row) => ({
              refusal: row.refusal,
              refused: row.refused,
              last_refused_at: row.last_refused_at,
            })),
          })),
        },
        null,
        2,
      );
    }
    case "providers": {
      const data = unwrap(await api.GET("/ai/provider-health", { signal }));
      return JSON.stringify(
        {
          configured: data.configured,
          providers: data.providers.map((row) => ({
            provider: row.provider,
            health: row.health,
            since: row.since,
            retry_after: row.retry_after,
          })),
        },
        null,
        2,
      );
    }
    case "search": {
      const data = unwrap(
        await api.GET("/embeddings/reindex/status", { signal }),
      );
      return JSON.stringify(
        {
          configured_identity: data.configured_identity,
          populated_identity: data.populated_identity,
          status: data.status,
          updated_at: data.updated_at,
          reindex_needed: data.reindex_needed,
          entities_pending: data.entities_pending,
        },
        null,
        2,
      );
    }
    case "recovery": {
      const data = unwrap(await api.GET("/admin/recovery-health", { signal }));
      const drill = data.last_drill;
      return JSON.stringify(
        {
          generated_at: data.generated_at,
          recovery_target_seconds: data.recovery_target_seconds,
          data_loss_target_seconds: data.data_loss_target_seconds,
          last_drill: drill && {
            started_at: drill.started_at,
            finished_at: drill.finished_at,
            outcome: drill.outcome,
            recovery_seconds: drill.recovery_seconds,
            data_loss_seconds: drill.data_loss_seconds,
            recovery_within_target: drill.recovery_within_target,
            data_loss_within_target: drill.data_loss_within_target,
          },
        },
        null,
        2,
      );
    }
    case "automatic": {
      const data = unwrap(
        await api.GET("/admin/automatic-work-health", { signal }),
      );
      const flagged = data.colleagues.filter((row) =>
        row.tallies.some((tally) => tally.above_threshold),
      );
      return JSON.stringify(
        {
          generated_at: data.generated_at,
          days: data.days,
          first_day: data.first_day,
          last_day: data.last_day,
          share_threshold: data.share_threshold,
          small_sample_below: data.small_sample_below,
          kinds: data.kinds.map((row) => ({
            kind: row.kind,
            records_not_mine: row.records_not_mine,
          })),
          unattributed: data.unattributed,
          colleagues: flagged.slice(0, 50).map((row) => ({
            user_id: row.user_id,
            status: row.status,
            tallies: row.tallies
              .filter((tally) => tally.above_threshold)
              .map((tally) => ({
                kind: tally.kind,
                not_mine_share: tally.not_mine_share,
                small_sample: tally.small_sample,
                above_threshold: tally.above_threshold,
                counts: {
                  created: tally.counts.created,
                  completed: tally.counts.completed,
                  open: tally.counts.open,
                  not_mine: tally.counts.not_mine,
                  rerouted: tally.counts.rerouted,
                  dismissed: tally.counts.dismissed,
                  rejected: tally.counts.rejected,
                  expired: tally.counts.expired,
                },
              })),
          })),
          colleague_sample_limit: 50,
          total_flagged_colleagues: flagged.length,
        },
        null,
        2,
      );
    }
    case "updates": {
      const data = unwrap(await api.GET("/admin/data-tasks", { signal }));
      return JSON.stringify(
        data.data.map((row) => ({
          task_id: row.task_id,
          run_id: row.run_id,
          task_version: row.task_version,
          status: row.status,
          last_error_class: row.last_error_class,
          error_sentence: row.error_sentence,
          error_remedy: row.error_remedy,
          step: row.step,
          version: row.version,
          scanned: row.scanned,
          changed: row.changed,
          blocked_reason: row.blocked_reason,
          next_attempt_at: row.next_attempt_at,
        })),
        null,
        2,
      );
    }
    default:
      throw new Error("Unknown health source");
  }
}

export function jobRunEvidence(run: components["schemas"]["JobFailure"]) {
  return {
    job_id: run.job_id,
    kind: run.kind,
    state: run.state,
    attempt: run.attempt,
    max_attempts: run.max_attempts,
    failed_at: run.failed_at,
    first_failed_at: run.first_failed_at,
    failure_class: run.failure_class,
    reason: run.reason,
    remedy: run.remedy,
  };
}
