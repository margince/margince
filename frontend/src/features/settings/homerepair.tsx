// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { ActionRow } from "../../design-system/actionrow";
import { Button } from "../../design-system/atoms";
import { ErrorLine } from "../../design-system/errorline";
import { SettingList, SettingRow } from "../../design-system/settingrow";
import { formatNumber } from "../../format/format";
import { viewerZone } from "../../format/timezone";
import { useLocale, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { useActivity } from "../../lib/activityread";
import { unwrap } from "../../lib/problem";
import { OpenActivityDrawer } from "../voice-knowledge/activitydrawer";

type Review = components["schemas"]["HomeRepairReview"];
type Task = components["schemas"]["HomeRepairTask"];
const actionLabels: Record<Task["action"], MessageKey> = {
  archive_automatic_reminder: "homeRepair.archive",
  human_review: "homeRepair.keepReview",
  leave_current: "homeRepair.keepCurrent",
};
const reasonLabels: Record<string, MessageKey> = {
  human_adoption_not_excluded: "homeRepair.reason.adopted",
  source_rule_not_verified: "homeRepair.reason.unverified",
  source_unavailable: "homeRepair.reason.unavailable",
  historical_without_current_deadline: "homeRepair.reason.historical",
  before_mailbox_history_baseline: "homeRepair.reason.baseline",
  meeting_cancelled: "homeRepair.reason.cancelled",
  request_past_waiting_horizon: "homeRepair.reason.requestAge",
  current_work: "homeRepair.reason.current",
};

function RepairTask({ task }: Readonly<{ task: Task }>) {
  const t = useT();
  const activity = useActivity(task.id);
  const [open, setOpen] = useState(false);
  return (
    <>
      <SettingRow
        label={activity.data?.subject || t("homeRepair.unnamed")}
        description={t(
          reasonLabels[task.reason] ?? "homeRepair.reason.unverified",
        )}
        control={
          <Button
            variant="ghost"
            disabled={!activity.data}
            onClick={() => setOpen(true)}
          >
            {t("homeRepair.inspect")}
          </Button>
        }
        value={t(actionLabels[task.action] ?? "homeRepair.keepReview")}
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

export function HomeRepairControls({
  version,
  canActivate,
}: Readonly<{ version: number; canActivate: boolean }>) {
  const t = useT();
  const { locale } = useLocale();
  const client = useQueryClient();
  const preview = useMutation({
    mutationFn: async (after?: string) =>
      unwrap(
        await api.POST("/admin/home-repair/preview", {
          body: { limit: 25, after },
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
  const review = preview.data;
  const archiveCount =
    review?.tasks.filter((task) => task.action === "archive_automatic_reminder")
      .length ?? 0;
  return (
    <>
      <p>{t("homeRepair.explain")}</p>
      {version < 1 && <p>{t("dataTasks.blocked.waiting_for_worker")}</p>}
      <ActionRow>
        <Button
          variant="ghost"
          pending={preview.isPending}
          disabled={activate.isPending}
          onClick={() => {
            activate.reset();
            preview.mutate(undefined);
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
              preview.mutate(review.tasks.at(-1)?.id);
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
              count: formatNumber(archiveCount, locale),
            })}
          </p>
          <SettingList>
            {review.tasks.map((task) => (
              <RepairTask key={task.id} task={task} />
            ))}
          </SettingList>
          {canActivate && archiveCount > 0 && (
            <ActionRow>
              <Button
                pending={activate.isPending}
                disabled={preview.isPending || version < 1}
                onClick={() => activate.mutate(review)}
              >
                {t("homeRepair.apply", {
                  count: formatNumber(archiveCount, locale),
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
