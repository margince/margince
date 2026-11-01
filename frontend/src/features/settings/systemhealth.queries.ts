// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useRef } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { useHealthScope, useHealthVisible } from "../../lib/healthscope";
import { unwrap } from "../../lib/problem";
import { useMe } from "../../lib/session";
import {
  healthAvailability,
  healthConditions,
  healthMeasures,
  healthResponsible,
  healthScopes,
} from "./systemhealth.copy";

const detailPrefixes: Readonly<Record<string, readonly string[]>> = {
  jobs: ["job-health", "job-health-history"],
  capture: ["capture-health"],
  intake: ["extension-ingest-health"],
  recovery: ["recovery-health"],
  automatic: ["automatic-work-health"],
  providers: ["ai-provider-health"],
  search: [
    "embed-reindex-status",
    "embed-reindex-identities",
    "embed-reindex-preview",
  ],
  updates: [],
};

export function useSystemHealth(enabled: boolean) {
  const me = useMe();
  const scope = useHealthScope();
  const visible = useHealthVisible();
  const client = useQueryClient();
  const reconciledDenial = useRef<string | undefined>(undefined);
  useEffect(() => {
    if (!visible && scope)
      client.cancelQueries({
        predicate: (query) => query.queryKey.includes(scope),
      });
  }, [client, scope, visible]);
  const query = useQuery({
    queryKey: ["system-health", scope],
    enabled: enabled && visible && scope !== undefined,
    staleTime: 60_000,
    refetchInterval: visible && enabled ? 60_000 : false,
    refetchIntervalInBackground: false,
    retry: 2,
    retryDelay: (attempt) => Math.min(1000 * 2 ** attempt, 10_000),
    queryFn: async ({ signal }) => {
      const report = unwrap(await api.GET("/admin/system-health", { signal }));
      if (
        !report ||
        !Array.isArray(report.sources) ||
        !Array.isArray(report.findings) ||
        !validHealthEvidence(report) ||
        !Number.isSafeInteger(report.action_count) ||
        report.action_count < 0 ||
        report.action_count !==
          report.findings.filter((finding) => finding.requires_action).length ||
        typeof report.complete !== "boolean" ||
        !["checked", "attention", "incomplete"].includes(report.assessment)
      ) {
        throw new Error("Invalid System health report");
      }
      return report;
    },
  });
  useEffect(() => {
    const denied =
      query.data?.sources.filter((s) => s.availability === "withheld") ?? [];
    const prefixes = denied.flatMap(
      (source) => detailPrefixes[source.id] ?? [],
    );
    if (denied.some((source) => source.id === "updates"))
      client.removeQueries({ queryKey: ["admin", "data-tasks"] });
    if (prefixes.length)
      client.removeQueries({
        predicate: (entry) =>
          typeof entry.queryKey[0] === "string" &&
          prefixes.includes(entry.queryKey[0]),
      });
    const mismatches = denied
      .filter(
        (source) =>
          me.data?.authorization?.objects[source.permission.split(":")[0] ?? ""]
            ?.read,
      )
      .map((source) => source.permission)
      .sort();
    const denialKey = JSON.stringify([scope, mismatches]);
    if (reconciledDenial.current !== denialKey) {
      reconciledDenial.current = denialKey;
      if (mismatches.length) client.invalidateQueries({ queryKey: ["me"] });
    }
  }, [client, query.data, me.data, scope]);
  return query;
}

export function useJobHistory(
  filters: {
    kind?: string;
    state?: "retryable" | "discarded" | "cancelled";
    days: number;
  },
  cursor?: string,
) {
  const scope = useHealthScope();
  const visible = useHealthVisible();
  return useQuery({
    queryKey: ["job-health-history", scope, filters, cursor],
    enabled: visible && scope !== undefined,
    retry: 2,
    queryFn: async ({ signal }) => {
      const report = unwrap(
        await api.GET("/admin/job-health/history", {
          signal,
          params: { query: { ...filters, cursor } },
        }),
      );
      if (
        !report ||
        !Array.isArray(report.groups) ||
        !Array.isArray(report.runs) ||
        report.coverage !== "retained_only" ||
        typeof report.has_more !== "boolean" ||
        typeof report.population_may_have_changed !== "boolean" ||
        (report.has_more &&
          (typeof report.next_cursor !== "string" ||
            report.next_cursor.length === 0)) ||
        !validHealthTimestamp(report.observed_at) ||
        !validHealthTimestamp(report.cutoff) ||
        !validHealthTimestamp(report.window_start) ||
        !report.groups.every(
          (group) =>
            group !== null &&
            typeof group === "object" &&
            typeof group.kind === "string" &&
            typeof group.fleet_wide === "boolean" &&
            ["retryable", "discarded", "cancelled"].includes(group.state) &&
            Number.isSafeInteger(group.count) &&
            group.count >= 0 &&
            validHealthTimestamp(group.first_at) &&
            validHealthTimestamp(group.latest_at),
        ) ||
        !report.runs.every(
          (run) =>
            (run.job_id === undefined ||
              (typeof run.job_id === "number" &&
                Number.isSafeInteger(run.job_id) &&
                run.job_id > 0)) &&
            typeof run.kind === "string" &&
            run.kind.length > 0 &&
            ["retryable", "discarded", "cancelled"].includes(run.state) &&
            validHealthTimestamp(run.failed_at) &&
            (run.first_failed_at == null ||
              validHealthTimestamp(run.first_failed_at)) &&
            [run.attempt, run.max_attempts].every(
              (value) => Number.isSafeInteger(value) && value >= 0,
            ) &&
            typeof run.reason === "string" &&
            [run.failure_class, run.remedy].every(
              (value) => value == null || typeof value === "string",
            ),
        )
      )
        throw new Error("Invalid job history");
      return report;
    },
  });
}

function validHealthEvidence(
  report: components["schemas"]["SystemHealth"],
): boolean {
  const at = (value: string | undefined) =>
    value === undefined || validHealthTimestamp(value);
  const sources = report.sources.every(
    (source) =>
      source !== null &&
      typeof source === "object" &&
      typeof source.id === "string" &&
      typeof source.permission === "string" &&
      typeof source.code === "string" &&
      Object.hasOwn(healthScopes, source.scope) &&
      validHealthMeasure(source.count, source.unit) &&
      Object.hasOwn(healthAvailability, source.availability) &&
      Object.hasOwn(healthConditions, source.condition) &&
      at(source.observed_at) &&
      at(source.window_start) &&
      at(source.window_end) &&
      (source.availability !== "available" || source.observed_at !== undefined),
  );
  const findings = report.findings.every(
    (finding) =>
      finding !== null &&
      typeof finding === "object" &&
      typeof finding.id === "string" &&
      typeof finding.source === "string" &&
      typeof finding.code === "string" &&
      typeof finding.requires_action === "boolean" &&
      Object.hasOwn(healthScopes, finding.scope) &&
      validHealthMeasure(finding.count, finding.unit) &&
      Object.hasOwn(healthConditions, finding.condition) &&
      Object.hasOwn(healthResponsible, finding.responsible) &&
      typeof finding.recovery === "string" &&
      Number.isInteger(finding.urgency) &&
      finding.urgency >= 0 &&
      finding.urgency <= 3 &&
      finding.action !== null &&
      typeof finding.action === "object" &&
      typeof finding.action.available === "boolean" &&
      at(finding.observed_at) &&
      at(finding.first_observed_at) &&
      at(finding.window_start) &&
      at(finding.window_end) &&
      at(finding.next_attempt_at),
  );
  return (
    sources &&
    findings &&
    typeof report.generated_at === "string" &&
    at(report.generated_at)
  );
}

function validHealthMeasure(
  count: number | undefined,
  unit: string | undefined,
) {
  if (count === undefined) return unit === undefined;
  return (
    Number.isSafeInteger(count) &&
    count >= 0 &&
    unit !== undefined &&
    Object.hasOwn(healthMeasures, unit)
  );
}

function validHealthTimestamp(value: unknown): boolean {
  return typeof value === "string" && Number.isFinite(Date.parse(value));
}
