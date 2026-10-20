// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useId, useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { ActionRow } from "../../design-system/actionrow";
import { Button, Modal } from "../../design-system/atoms";
import { ErrorLine } from "../../design-system/errorline";
import { FilterPills } from "../../design-system/filterpills";
import { Heading } from "../../design-system/heading";
import { SettingList, SettingRow } from "../../design-system/settingrow";
import { formatNumber } from "../../format/format";
import { viewerZone } from "../../format/timezone";
import { useLocale, usePlural, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { useActivity } from "../../lib/activityread";
import { useCanWrite } from "../../lib/capability";
import { unwrap } from "../../lib/problem";
import { ApprovalDetailModal } from "../automations/approvaleditor";
import { DealSuggestionCard, suggestedName } from "../deals/dealsuggestion";
import { useDealSuggestion } from "../deals/dealsuggestions.queries";
import { OpenActivityDrawer } from "../voice-knowledge/activitydrawer";

type Review = components["schemas"]["HomeRepairReview"];
type Task = components["schemas"]["HomeRepairTask"];
const actionLabels: Record<Task["action"], MessageKey> = {
  reclassify_message: "homeRepair.reclassify",
  supersede_suggestion: "homeRepair.supersede",
  withdraw_proposal: "homeRepair.withdraw",
  archive_automatic_reminder: "homeRepair.archive",
  human_review: "homeRepair.keepReview",
  leave_current: "homeRepair.keepCurrent",
};
export const HOME_REPAIR_REASON_LABELS: Record<string, MessageKey> = {
  classification_not_attempted: "homeRepair.reason.classificationNotAttempted",
  classification_uncertain: "homeRepair.reason.classificationUncertain",
  human_changed_or_provenance_missing:
    "homeRepair.reason.classificationAdopted",
  classification_context_withheld: "homeRepair.reason.classificationWithheld",
  classification_current: "homeRepair.reason.classificationCurrent",
  classification_changed: "homeRepair.reason.classificationChanged",
  company_has_deal: "homeRepair.reason.dealExists",
  suggestion_source_changed: "homeRepair.reason.dealSource",
  buyer_no_longer_verified: "homeRepair.reason.buyer",
  human_adoption_not_excluded: "homeRepair.reason.adopted",
  source_rule_not_verified: "homeRepair.reason.unverified",
  source_unavailable: "homeRepair.reason.unavailable",
  historical_without_current_deadline: "homeRepair.reason.historical",
  before_mailbox_history_baseline: "homeRepair.reason.baseline",
  meeting_cancelled: "homeRepair.reason.cancelled",
  request_past_waiting_horizon: "homeRepair.reason.requestAge",
  continuing_engagement: "homeRepair.reason.engagement",
  ending_is_not_account_wide: "homeRepair.reason.workstream",
  lifecycle_evidence_changed: "homeRepair.reason.evidenceChanged",
  duplicate_pending_proposal: "homeRepair.reason.duplicate",
  later_context_not_read: "homeRepair.reason.contextUnread",
  later_context_not_assessed: "homeRepair.reason.contextUnassessed",
  source_time_unknown: "homeRepair.reason.timeUnknown",
  source_time_contradicted: "homeRepair.reason.timeContradicted",
  meeting_not_yet_held: "homeRepair.reason.meetingPending",
  current_work: "homeRepair.reason.current",
};

function RepairTask({ task }: Readonly<{ task: Task }>) {
  const t = useT();
  const activity = useActivity(task.id);
  const [open, setOpen] = useState(false);
  return (
    <>
      <RepairItemRow
        task={task}
        label={activity.data?.subject || t("homeRepair.unnamed")}
        disabled={!activity.data}
        onInspect={() => setOpen(true)}
      />
      {open && (
        <OpenActivityDrawer
          activityId={task.id}
          activity={activity.data}
          zone={viewerZone()}
          onClose={() => setOpen(false)}
        />
      )}
    </>
  );
}

function RepairItemRow({
  task,
  label,
  disabled,
  onInspect,
}: Readonly<{
  task: Task;
  label: string;
  disabled: boolean;
  onInspect: () => void;
}>) {
  const t = useT();
  return (
    <SettingRow
      label={label}
      description={
        task.before_verdict &&
        task.after_verdict &&
        task.before_verdict !== task.after_verdict
          ? t("homeRepair.classificationChange", {
              before: t(
                task.before_verdict === "asks_us"
                  ? "homeRepair.asks"
                  : "homeRepair.informs",
              ),
              after: t(
                task.after_verdict === "asks_us"
                  ? "homeRepair.asks"
                  : "homeRepair.informs",
              ),
            })
          : t(
              HOME_REPAIR_REASON_LABELS[task.reason] ??
                "homeRepair.reason.unverified",
            )
      }
      control={
        <Button variant="ghost" disabled={disabled} onClick={onInspect}>
          {t("homeRepair.inspect")}
        </Button>
      }
      value={t(actionLabels[task.action] ?? "homeRepair.keepReview")}
    />
  );
}

function RepairApproval({ task }: Readonly<{ task: Task }>) {
  const t = useT();
  const [open, setOpen] = useState(false);
  const detail = useQuery({
    queryKey: ["approval", task.id],
    queryFn: async () =>
      unwrap(
        await api.GET("/approvals/{id}", { params: { path: { id: task.id } } }),
      ),
  });
  return (
    <>
      <RepairItemRow
        task={task}
        label={detail.data?.summary || t("homeRepair.unnamed")}
        disabled={!detail.data}
        onInspect={() => setOpen(true)}
      />
      <ApprovalDetailModal
        approvalId={task.id}
        open={open}
        onClose={() => setOpen(false)}
      />
    </>
  );
}

function RepairSuggestion({ task }: Readonly<{ task: Task }>) {
  const t = useT();
  const heading = useId();
  const [open, setOpen] = useState(false);
  const detail = useDealSuggestion(task.id);
  const suggestion = detail.error ? undefined : detail.data;
  const label = suggestion
    ? suggestedName(suggestion, t)
    : t("homeRepair.unnamed");
  return (
    <>
      <RepairItemRow
        task={task}
        label={label}
        disabled={!suggestion}
        onInspect={() => setOpen(true)}
      />
      <ErrorLine error={detail.error} />
      {open && suggestion && (
        <Modal
          open
          onClose={() => setOpen(false)}
          labelledBy={heading}
          intent="form"
        >
          <Heading size="large" id={heading} className="modal-title">
            {label}
          </Heading>
          <DealSuggestionCard suggestion={suggestion} />
        </Modal>
      )}
    </>
  );
}

export function HomeRepairControls({
  version,
  canActivate,
}: Readonly<{ version: number; canActivate: boolean }>) {
  const t = useT();
  const { locale } = useLocale();
  const client = useQueryClient();
  const plural = usePlural();
  const mayRetireSuggestions = useCanWrite("deal", "create");
  const [source, setSource] = useState<
    "task" | "approval" | "suggestion" | "verdict"
  >("task");
  const preview = useMutation({
    mutationFn: async ({
      after,
      source,
    }: {
      after?: string;
      source: "task" | "approval" | "suggestion" | "verdict";
    }) =>
      unwrap(
        await api.POST("/admin/home-repair/preview", {
          body: { limit: 25, after, source },
        }),
      ),
  });
  const activate = useMutation({
    mutationFn: async (review: Review) =>
      unwrap(
        await api.POST("/admin/home-repair/activate", {
          body: { version, review },
        }),
      ),
    onSuccess: async () => {
      preview.reset();
      await client.invalidateQueries({ queryKey: ["admin", "data-tasks"] });
    },
  });
  const review =
    preview.variables?.source === source ? preview.data : undefined;
  const changeCount = review
    ? [
        review.counts.archive,
        review.counts.withdraw,
        review.counts.supersede,
        review.counts.reclassify,
      ].reduce((total, count) => total + (count ?? 0), 0)
    : 0;
  return (
    <>
      <p>{t("homeRepair.explain")}</p>
      <FilterPills
        value={source}
        pills={[
          { value: "task", label: t("homeRepair.tasks") },
          { value: "approval", label: t("homeRepair.proposals") },
          { value: "suggestion", label: t("homeRepair.suggestions") },
          { value: "verdict", label: t("homeRepair.classifications") },
        ]}
        label={t("homeRepair.kind")}
        onChange={(next) => {
          setSource(next);
          preview.reset();
          activate.reset();
        }}
      />
      {version < 1 && <p>{t("dataTasks.blocked.waiting_for_worker")}</p>}
      <ActionRow>
        <Button
          variant="ghost"
          pending={preview.isPending}
          disabled={activate.isPending}
          onClick={() => {
            activate.reset();
            preview.mutate({ source });
          }}
        >
          {t("homeRepair.preview")}
        </Button>
        {review && review.tasks.length === 25 && (
          <Button
            variant="ghost"
            disabled={preview.isPending || activate.isPending}
            onClick={() => {
              activate.reset();
              preview.mutate({ source, after: review.tasks.at(-1)?.id });
            }}
          >
            {t("homeRepair.next")}
          </Button>
        )}
      </ActionRow>
      <ErrorLine error={preview.error || activate.error} />
      {review && (
        <>
          <p>
            {t("homeRepair.summary", {
              reclassify: formatNumber(review.counts.reclassify ?? 0, locale),
              archive: formatNumber(review.counts.archive, locale),
              withdraw: formatNumber(review.counts.withdraw, locale),
              supersede: formatNumber(review.counts.supersede ?? 0, locale),
              review: formatNumber(review.counts.review, locale),
              keep: formatNumber(review.counts.keep, locale),
            })}
          </p>
          <SettingList>
            {review.tasks.map((task) =>
              task.source === "suggestion" ? (
                <RepairSuggestion key={task.id} task={task} />
              ) : task.source === "approval" ? (
                <RepairApproval key={task.id} task={task} />
              ) : (
                <RepairTask key={task.id} task={task} />
              ),
            )}
          </SettingList>
          {canActivate && source === "suggestion" && !mayRetireSuggestions && (
            <p>{t("homeRepair.suggestionPermission")}</p>
          )}
          {canActivate &&
            (source !== "suggestion" || mayRetireSuggestions) &&
            changeCount > 0 && (
              <ActionRow>
                <Button
                  pending={activate.isPending}
                  disabled={preview.isPending || version < 1}
                  onClick={() => activate.mutate(review)}
                >
                  {plural("homeRepair.apply", changeCount, {
                    count: formatNumber(changeCount, locale),
                  })}
                </Button>
              </ActionRow>
            )}
        </>
      )}
      {activate.isSuccess && <p>{t("homeRepair.started")}</p>}
    </>
  );
}
