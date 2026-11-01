// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { ActionRow } from "../../design-system/actionrow";
import { Badge, Button, EmptyState } from "../../design-system/atoms";
import { ErrorLine } from "../../design-system/errorline";
import { type Fact, FactList } from "../../design-system/factlist";
import { PanelBody } from "../../design-system/panel";
import { SettingList, SettingRow } from "../../design-system/settingrow";
import { formatDateTime, formatNumber } from "../../format/format";
import { viewerZone } from "../../format/timezone";
import { useLocale, usePlural, useT, type Words } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { useCan, useCanWrite } from "../../lib/capability";
import { useHealthScope, useHealthVisible } from "../../lib/healthscope";
import { problemCodeOf, unwrap } from "../../lib/problem";
import { HealthCard } from "./healthcard";
import { HomeRepairControls } from "./homerepair";

type Progress = components["schemas"]["DataTaskProgress"];
type Control = components["schemas"]["DataTaskControl"];
const queryKey = ["admin", "data-tasks"];
const statusLabels: Record<Progress["status"], MessageKey> = {
  pending: "dataTasks.status.pending",
  running: "dataTasks.status.running",
  paused: "dataTasks.status.paused",
  done: "dataTasks.status.done",
  failed: "dataTasks.status.failed",
};
const blockLabels: Record<
  NonNullable<Progress["blocked_reason"]>,
  MessageKey
> = {
  review_required: "dataTasks.blocked.review_required",
  waiting_for_worker: "dataTasks.blocked.waiting_for_worker",
  newer_release_required: "dataTasks.blocked.newer_release_required",
  paused: "dataTasks.blocked.paused",
  failed: "dataTasks.blocked.failed",
  retry_wait: "dataTasks.blocked.retry_wait",
  earlier_step: "dataTasks.blocked.earlier_step",
  missing_prerequisite: "dataTasks.blocked.missing_prerequisite",
};

function validProgress(row: Progress): boolean {
  return (
    row !== null &&
    typeof row === "object" &&
    typeof row.task_id === "string" &&
    typeof row.title === "string" &&
    typeof row.step === "string" &&
    typeof row.status === "string" &&
    [row.version, row.scanned, row.changed, row.batches, row.batch_limit].every(
      (value) => Number.isSafeInteger(value) && value >= 0,
    ) &&
    [
      row.run_id,
      row.first_release,
      row.blocked_by,
      row.error_sentence,
      row.error_remedy,
    ].every((value) => value === undefined || typeof value === "string") &&
    [row.estimated_at, row.next_attempt_at, row.last_progress_at].every(
      (value) =>
        value === undefined ||
        (typeof value === "string" && Number.isFinite(Date.parse(value))),
    ) &&
    (row.blocked_reason === undefined ||
      typeof row.blocked_reason === "string") &&
    typeof row.estimate_capped === "boolean" &&
    (!row.estimate_capped || row.estimate !== undefined) &&
    (row.estimate === undefined ||
      (Number.isSafeInteger(row.estimate) && row.estimate >= 0)) &&
    Array.isArray(row.missing_prerequisites) &&
    row.missing_prerequisites.every((value) => typeof value === "string")
  );
}

function useDataTasks() {
  const canSee = useCan("data_task", "read");
  const scope = useHealthScope();
  const visible = useHealthVisible();
  const query = useQuery({
    queryKey: [...queryKey, scope],
    enabled: canSee && visible && scope !== undefined,
    queryFn: async ({ signal }) => {
      const result = unwrap(await api.GET("/admin/data-tasks", { signal }));
      if (!Array.isArray(result?.data) || !result.data.every(validProgress)) {
        throw new Error("Invalid data task report");
      }
      return result.data;
    },
    refetchInterval: ({ state }) =>
      state.data?.some((row) => row.status === "running") ? 2_500 : 60_000,
  });
  return { canSee, query };
}

function taskFacts(row: Progress, { t, plural, locale }: Words): Fact[] {
  const number = (value: number) => formatNumber(value, locale);
  const date = (value: string) => formatDateTime(value, locale, viewerZone());
  const estimate =
    row.estimate === undefined
      ? t("dataTasks.unknown")
      : row.estimate_capped
        ? plural("dataTasks.atLeast", row.estimate, {
            count: number(row.estimate),
          })
        : number(row.estimate);
  const facts: Fact[] = [
    {
      key: "scanned",
      term: t("dataTasks.scanned"),
      value: number(row.scanned),
    },
    {
      key: "changed",
      term: t("dataTasks.changed"),
      value: number(row.changed),
    },
    {
      key: "estimate",
      term: t("dataTasks.estimate"),
      value: estimate,
      note: row.estimated_at ? date(row.estimated_at) : undefined,
    },
    {
      key: "batch",
      term: t("dataTasks.batchLimit"),
      value: number(row.batch_limit),
    },
    {
      key: "batches",
      term: t("dataTasks.batches"),
      value: number(row.batches),
    },
    {
      key: "progress",
      term: t("dataTasks.lastProgress"),
      value: row.last_progress_at
        ? date(row.last_progress_at)
        : t("dataTasks.notStarted"),
    },
  ];
  if (row.next_attempt_at)
    facts.push({
      key: "retry",
      term: t("dataTasks.nextAttempt"),
      value: date(row.next_attempt_at),
    });
  if (row.review_kind === "home_attention")
    facts.push({
      key: "preserved",
      term: t("homeRepair.preserved"),
      value: number(row.scanned - row.changed),
      note: t("homeRepair.preservedNote"),
    });
  if (row.first_release)
    facts.push({
      key: "release",
      term: t("dataTasks.release"),
      value: row.first_release,
    });
  if (row.run_id)
    facts.push({ key: "run", term: t("dataTasks.run"), value: row.run_id });
  return facts;
}

function taskActions(row: Progress, canChange: boolean): Control["action"][] {
  const actions: Control["action"][] = [];
  const knownState =
    Object.hasOwn(statusLabels, row.status) &&
    (row.blocked_reason === undefined ||
      Object.hasOwn(blockLabels, row.blocked_reason));
  if (
    canChange &&
    row.version > 0 &&
    knownState &&
    row.blocked_reason !== "newer_release_required" &&
    row.blocked_reason !== "review_required"
  ) {
    if (row.status === "pending" || row.status === "running")
      actions.push("pause");
    if (row.status === "paused") actions.push("resume");
    if (row.status === "failed") actions.push("retry");
    if (row.missing_prerequisites.length === 0) actions.push("estimate");
  }
  return actions;
}

function TaskRow({ row }: Readonly<{ row: Progress }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const canChange = useCanWrite("data_task", "update");
  const client = useQueryClient();
  const mutation = useMutation({
    mutationFn: async (body: Control) =>
      unwrap(await api.POST("/admin/data-tasks/control", { body })),
    onSettled: () =>
      Promise.all([
        client.invalidateQueries({ queryKey }),
        client.invalidateQueries({ queryKey: ["system-health"] }),
      ]),
  });
  const actions = taskActions(row, canChange);
  return (
    <SettingRow
      label={row.title}
      description={
        <>
          <p>
            {t("dataTasks.step", { step: row.step })}{" "}
            <Badge tone={row.status === "failed" ? "danger" : undefined}>
              {t(
                Object.hasOwn(statusLabels, row.status)
                  ? statusLabels[row.status]
                  : "dataTasks.unknownState",
              )}
            </Badge>
          </p>
          {row.blocked_reason && (
            <p>
              {t(
                Object.hasOwn(blockLabels, row.blocked_reason)
                  ? blockLabels[row.blocked_reason]
                  : "dataTasks.unknownState",
              )}
            </p>
          )}
          {row.blocked_by && (
            <p>{t("dataTasks.blockedBy", { name: row.blocked_by })}</p>
          )}
          {row.missing_prerequisites.length > 0 && (
            <p>
              {t("dataTasks.prerequisites", {
                names: row.missing_prerequisites.join(", "),
              })}
            </p>
          )}
          {row.error_sentence && (
            <ErrorLine standing>
              {row.error_sentence} {row.error_remedy}
            </ErrorLine>
          )}
        </>
      }
      layout="stack"
      control={
        <>
          <FactList facts={taskFacts(row, { t, plural, locale })} />
          {row.review_kind === "home_attention" && (
            <HomeRepairControls
              version={row.version}
              canActivate={
                canChange &&
                (row.blocked_reason === "review_required" ||
                  row.status === "done" ||
                  row.status === "paused" ||
                  row.status === "failed")
              }
            />
          )}
          {actions.length > 0 && (
            <ActionRow>
              {actions.map((action) => (
                <Button
                  key={action === "estimate" ? "estimate" : "state"}
                  variant="ghost"
                  pending={
                    mutation.isPending && mutation.variables?.action === action
                  }
                  disabled={mutation.isPending}
                  onClick={() =>
                    mutation.mutate({
                      task_id: row.task_id,
                      step: row.step,
                      version: row.version,
                      action,
                    })
                  }
                >
                  {t(`dataTasks.action.${action}`)}
                </Button>
              ))}
            </ActionRow>
          )}
          <ErrorLine
            error={
              problemCodeOf(mutation.error) === "conflict" &&
              mutation.variables?.version !== row.version
                ? null
                : mutation.error
            }
          />
        </>
      }
    />
  );
}

export function DataTasksHealthCard() {
  const t = useT();
  const { canSee, query } = useDataTasks();
  return (
    <HealthCard
      title={t("dataTasks.title")}
      sub={t("dataTasks.sub")}
      withheld={t("dataTasks.withheld")}
      canSee={canSee}
      query={query}
    >
      {(rows) =>
        rows.length === 0 ? (
          <PanelBody>
            <EmptyState>{t("dataTasks.empty")}</EmptyState>
          </PanelBody>
        ) : (
          <SettingList bleed="settings">
            {rows.map((row) => (
              <TaskRow key={`${row.task_id}/${row.step}`} row={row} />
            ))}
          </SettingList>
        )
      }
    </HealthCard>
  );
}
