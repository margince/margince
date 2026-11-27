// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Button } from "../../design-system/atoms";
import { useToast } from "../../design-system/toast";
import { useT } from "../../i18n";
import { InquiryTakeButton } from "./worklist.inquiryassignment";
import type { WorklistItem } from "./worklist.queries";
import { useTakeTask } from "./worklist.writes";

/**
 * Takes a task nobody holds, through the endpoint that admits a reader who may
 * only see it. Drawn only where the server offers `take`.
 */
export function TakeVerb({ item }: Readonly<{ item: WorklistItem }>) {
  if (
    !item.actions.includes("take") ||
    item.batch ||
    item.version === undefined
  ) {
    return null;
  }
  return item.kind === "website_inquiry" ? (
    <InquiryTakeButton item={item} />
  ) : (
    <TakeButton item={item} />
  );
}

// Apart from TakeVerb so a row with nothing to take never asks who is reading.
function TakeButton({ item }: Readonly<{ item: WorklistItem }>) {
  const t = useT();
  const toast = useToast();
  const take = useTakeTask();
  return (
    <Button
      pending={take.isPending}
      // mutateAsync, not per-call callbacks. The refetch after the write takes
      // the row off the Unassigned list. React Query drops the callbacks of an
      // unmounted button, but the promise still settles, so the line shows.
      onClick={() =>
        take
          .mutateAsync({
            activityId: item.id,
            version: item.version,
          })
          .then(
            () => toast.show(t("worklist.take.done")),
            () => toast.show(t("worklist.take.failed"), { tone: "danger" }),
          )
      }
    >
      {t("worklist.take.action")}
    </Button>
  );
}
