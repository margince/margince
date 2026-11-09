// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQueryClient } from "@tanstack/react-query";
import type { components } from "../api/schema";
import { Button } from "../design-system/atoms";
import { ErrorLine } from "../design-system/errorline";
import { useToast } from "../design-system/toast";
import { useT } from "../i18n";
import { useDealDecline } from "../lib/dealdecline";

type Activity = components["schemas"]["Activity"];

/** The deal question a capture raised, keyed by the source its writer stamps. */
export const WHICH_DEAL_SOURCE = "which_deal";

/**
 * "None of these deals" on a deal question. The email stays off every deal
 * the question offered, and the question is done, so the task leaves the list.
 */
export function WhichDealDecline({ task }: Readonly<{ task: Activity }>) {
  const t = useT();
  const toast = useToast();
  const queryClient = useQueryClient();
  const decline = useDealDecline(() => {
    toast.show(t("tasks.whichDeal.noneDone"));
    return queryClient.invalidateQueries({ queryKey: ["activity", task.id] });
  });
  const mail = task.source_activity_id;
  if (task.source_system !== WHICH_DEAL_SOURCE || !mail) return null;
  return (
    <>
      <Button
        disabled={decline.isPending}
        onClick={() => decline.mutate({ activityId: mail })}
      >
        {t("tasks.whichDeal.none")}
      </Button>
      {decline.isError && <ErrorLine error={decline.error} inline />}
    </>
  );
}
