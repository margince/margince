// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";
import { Badge, Button } from "../../design-system/atoms";
import { undoAction, useToast } from "../../design-system/toast";
import { useT } from "../../i18n";
import { useCanWriteRecord } from "../../lib/capability";
import {
  type DealRelevanceInput,
  useDealRelevance,
} from "../../lib/dealdecline";
import { problemMessageOf } from "../../lib/problem";

type Activity = components["schemas"]["Activity"];
type Relevance = DealRelevanceInput["relevance"];

/** The mark the activity's link to the deal carries; none when nobody judged it. */
export function dealRelevanceOf(activity: Activity, dealId: string) {
  return activity.links?.find(
    (link) => link.entity_type === "deal" && link.entity_id === dealId,
  )?.deal_relevance;
}

/**
 * "Not about this deal" and "About this deal" on a row of a deal's timeline.
 * The row stays on the timeline with its mark, the deal's activity date stops
 * counting it, and the toast's Undo sets the other mark.
 */
export function DealRelevanceAction({
  activity,
  dealId,
}: Readonly<{ activity: Activity; dealId: string }>) {
  const t = useT();
  const toast = useToast();
  const writable = useCanWriteRecord("activity", activity);
  // Pressed from the menu or from an Undo toast, so a refusal lands in a toast.
  const refuse = (error: Error) =>
    toast.show(problemMessageOf(error, t), { tone: "danger", sticky: true });
  const mark = useDealRelevance();
  // Each mark offers an Undo that sets the other mark, through the same send.
  function send(input: DealRelevanceInput) {
    mark.mutate(input, {
      onError: refuse,
      onSuccess: (answer) => {
        if (!answer.changed) return;
        const back: Relevance =
          input.relevance === "unrelated" ? "relevant" : "unrelated";
        toast.show(t(markedMessage(input.relevance, answer.project_kept)), {
          action: undoAction(t("common.undo"), () =>
            send({ ...input, relevance: back }),
          ),
        });
      },
    });
  }
  const unrelated = dealRelevanceOf(activity, dealId) === "unrelated";
  return (
    <Button
      onClick={() =>
        send({
          activityId: activity.id,
          dealId,
          relevance: unrelated ? "relevant" : "unrelated",
        })
      }
      pending={mark.isPending}
      reason={writable ? undefined : t("activity.notYoursToChange")}
    >
      {t(unrelated ? "dealRelevance.about" : "dealDecline.action")}
    </Button>
  );
}

function markedMessage(relevance: Relevance, projectKept: boolean) {
  if (relevance === "relevant") return "dealRelevance.restored";
  return projectKept ? "dealDecline.doneProjectKept" : "dealDecline.done";
}

/** The row's mark when it does not count for the deal. */
export function DealRelevanceBadge({
  activity,
  dealId,
}: Readonly<{ activity: Activity; dealId: string }>) {
  const t = useT();
  switch (dealRelevanceOf(activity, dealId)) {
    case "unrelated":
      return <Badge tone="warning">{t("dealRelevance.unrelated")}</Badge>;
    case "uncertain":
      return <Badge>{t("dealRelevance.uncertain")}</Badge>;
    default:
      return null;
  }
}
