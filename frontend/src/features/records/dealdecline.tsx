// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";
import { Badge, Button } from "../../design-system/atoms";
import { undoAction, useToast } from "../../design-system/toast";
import { useLocale, useT } from "../../i18n";
import { useCanWriteRecord } from "../../lib/capability";
import {
  type DealRelevanceInput,
  useDealRelevance,
} from "../../lib/dealdecline";
import { unitWords } from "../../lib/extensions";
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
 * Sets a mark on the row's deal link, with an Undo toast that sets the other
 * mark through the same send. A refusal lands in a toast, because the press
 * came from a menu, a row or a toast.
 */
function useDealMark() {
  const t = useT();
  const toast = useToast();
  const refuse = (error: Error) =>
    toast.show(problemMessageOf(error, t), { tone: "danger", sticky: true });
  const mark = useDealRelevance();
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
  return { send, pending: mark.isPending };
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
  const writable = useCanWriteRecord("activity", activity);
  const mark = useDealMark();
  const unrelated = dealRelevanceOf(activity, dealId) === "unrelated";
  return (
    <Button
      onClick={() =>
        mark.send({
          activityId: activity.id,
          dealId,
          relevance: unrelated ? "relevant" : "unrelated",
        })
      }
      pending={mark.pending}
      reason={writable ? undefined : t("activity.notYoursToChange")}
    >
      {t(unrelated ? "dealRelevance.about" : "dealDecline.action")}
    </Button>
  );
}

/**
 * Which extension filed the row on this deal and why, while the filing holds.
 * Its Undo marks the row "Not about this deal", so no automatic pass files it
 * there again.
 */
export function DealFiledBy({
  activity,
  dealId,
}: Readonly<{ activity: Activity; dealId: string }>) {
  const t = useT();
  const { locale } = useLocale();
  const writable = useCanWriteRecord("activity", activity);
  const mark = useDealMark();
  const link = activity.links?.find(
    (candidate) =>
      candidate.entity_type === "deal" && candidate.entity_id === dealId,
  );
  const filer = link?.filed_by;
  if (!filer || filer.actor === "core" || link.deal_relevance === "unrelated") {
    return null;
  }
  const unit = unitWords(locale, filer.actor, "title") || filer.actor;
  const reason = filer.reason
    ? unitWords(locale, filer.actor, `captureReason.${filer.reason}`) ||
      filer.reason
    : "";
  return (
    <>
      <Badge>
        {reason
          ? t("dealFiling.byUnit", { unit, reason })
          : t("dealFiling.byUnitNoReason", { unit })}
      </Badge>
      <Button
        variant="link"
        onClick={() =>
          mark.send({ activityId: activity.id, dealId, relevance: "unrelated" })
        }
        pending={mark.pending}
        reason={writable ? undefined : t("activity.notYoursToChange")}
      >
        {t("common.undo")}
      </Button>
    </>
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
