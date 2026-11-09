// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import type { components } from "../../api/schema";
import { Button } from "../../design-system/atoms";
import { ConfirmModal } from "../../design-system/confirmmodal";
import { useToast } from "../../design-system/toast";
import { useT } from "../../i18n";
import { entityTimelineKeys } from "../../lib/activitykeys";
import { useCanWriteRecord } from "../../lib/capability";
import { useDealDecline } from "../../lib/dealdecline";
import { problemMessageOf } from "../../lib/problem";

type Activity = components["schemas"]["Activity"];

/**
 * "Not about this deal" on an email of a deal's timeline. The email comes off
 * the deal and no automatic pass files it there again. A project the email got
 * from the deal comes off too, when the server's filing rules allow it.
 */
export function DealDeclineAction({
  activity,
  dealId,
}: Readonly<{ activity: Activity; dealId: string }>) {
  const t = useT();
  const toast = useToast();
  const queryClient = useQueryClient();
  const writable = useCanWriteRecord("activity", activity);
  const [open, setOpen] = useState(false);
  const decline = useDealDecline(async (answer) => {
    setOpen(false);
    toast.show(
      t(
        answer.project_kept
          ? "dealDecline.doneProjectKept"
          : "dealDecline.done",
      ),
    );
    await Promise.all(
      entityTimelineKeys("deal", dealId).map((queryKey) =>
        queryClient.invalidateQueries({ queryKey }),
      ),
    );
  });
  return (
    <>
      <Button
        onClick={() => setOpen(true)}
        reason={writable ? undefined : t("activity.notYoursToChange")}
      >
        {t("dealDecline.action")}
      </Button>
      <ConfirmModal
        open={open}
        onClose={() => {
          decline.reset();
          setOpen(false);
        }}
        title={t("dealDecline.title")}
        confirmLabel={t("dealDecline.confirm")}
        onConfirm={() => decline.mutate({ activityId: activity.id, dealId })}
        pending={decline.isPending}
        error={decline.isError ? problemMessageOf(decline.error, t) : null}
      >
        <p className="t-body">{t("dealDecline.body")}</p>
      </ConfirmModal>
    </>
  );
}
