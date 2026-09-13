// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useCanWrite } from "../app/capability";
import { navigate } from "../app/router";
import { Button } from "../design-system/atoms";
import { useT } from "../i18n";
import { TeamPlanReview } from "./brief.teamplan";
import { problemMessageOf, useMe } from "./common";
import { useSetCommitmentState } from "./weeklyplan.queries";
import type { WorklistItem } from "./worklist.queries";

export function PlanWorkActions({ item }: Readonly<{ item: WorklistItem }>) {
  const t = useT();
  const me = useMe();
  const canEdit = useCanWrite("weekly_plan", "update");
  const settle = useSetCommitmentState();
  const own = item.owner?.id === me.data?.user.id;
  if (!me.data) return null;
  return (
    <>
      {!own && item.owner?.id ? (
        <TeamPlanReview owner={item.owner.id} name={item.owner.label ?? ""} />
      ) : (
        <Button
          variant="ghost"
          small
          onClick={() =>
            navigate({ screen: "brief" }, new Map([["view", "weekly"]]))
          }
        >
          {t("brief.plan.open")}
        </Button>
      )}
      {canEdit && own && (
        <Button
          small
          pending={settle.isPending}
          onClick={() => settle.mutate({ id: item.id, state: "done" })}
        >
          {t("plan.state.done")}
        </Button>
      )}
      {settle.isError && (
        <span role="alert">{problemMessageOf(settle.error, t)}</span>
      )}
    </>
  );
}
