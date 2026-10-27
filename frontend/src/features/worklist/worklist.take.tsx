// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Button } from "../../design-system/atoms";
import { useToast } from "../../design-system/toast";
import { useT } from "../../i18n";
import { useMe } from "../../lib/session";
import type { WorklistItem } from "./worklist.queries";
import { useReassignTask } from "./worklist.writes";

/**
 * Takes a task nobody holds: the hand-off's own write, with the reader as the
 * assignee. Drawn only where the server offers `take`.
 */
export function TakeVerb({ item }: Readonly<{ item: WorklistItem }>) {
  if (!item.actions.includes("take") || item.batch) {
    return null;
  }
  return <TakeButton item={item} />;
}

// Apart from TakeVerb so a row with nothing to take never asks who is reading.
function TakeButton({ item }: Readonly<{ item: WorklistItem }>) {
  const t = useT();
  const toast = useToast();
  const reader = useMe().data?.user?.id;
  const take = useReassignTask();
  if (reader === undefined) {
    return null;
  }
  return (
    <Button
      pending={take.isPending}
      onClick={() =>
        take.mutate(
          { activityId: item.id, version: item.version, assigneeId: reader },
          {
            onSuccess: () => toast.show(t("worklist.take.done")),
            onError: () =>
              toast.show(t("worklist.take.failed"), { tone: "danger" }),
          },
        )
      }
    >
      {t("worklist.take.action")}
    </Button>
  );
}
