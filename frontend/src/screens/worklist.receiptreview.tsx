// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "../api/client";
import { ifMatch } from "../api/version";
import { useCanWrite } from "../app/capability";
import { Button } from "../design-system/atoms";
import { useT } from "../i18n";
import { problemMessageOf, throwProblem } from "./common";
import { type Receipt, worklistKey } from "./worklist.queries";
import { ReceiptUndo } from "./worklist.receiptundo";

export function ReceiptReview({ receipt }: Readonly<{ receipt: Receipt }>) {
  const t = useT();
  const client = useQueryClient();
  const writable = useCanWrite("deal", "update");
  const decide = useMutation({
    mutationFn: async (input: {
      id: string;
      changeId: string;
      version: number;
      action: "accept" | "undo";
    }) => {
      const { error } =
        input.action === "accept"
          ? await api.POST("/deals/{id}/applied-changes/{changeId}/accept", {
              params: {
                path: { id: input.id, changeId: input.changeId },
                ...ifMatch(input.version),
              },
            })
          : await api.POST(
              "/deals/{id}/stage-progressions/{approvalId}/revert",
              {
                params: { path: { id: input.id, approvalId: input.changeId } },
              },
            );
      if (error) throwProblem(error);
    },
    onError: () => {
      client.invalidateQueries({ queryKey: worklistKey });
    },
    onSuccess: () => {
      client.invalidateQueries({ queryKey: worklistKey });
      client.invalidateQueries({ queryKey: ["brief"] });
      client.invalidateQueries({ queryKey: ["deals"] });
    },
  });
  const review = receipt.review;
  if (!review) return <ReceiptUndo receipt={receipt} />;
  if (review.reversed)
    return <span className="t-caption">{t("brief.changes.undone")}</span>;
  if (review.accepted)
    return <span className="t-caption">{t("brief.changes.accepted")}</span>;
  if (!review.can_accept && !review.can_undo)
    return <p className="t-caption">{t("brief.changes.superseded")}</p>;
  const subject = receipt.subject;
  if (!writable || !review.writable || subject?.type !== "deal") return null;
  const press = (action: "accept" | "undo") =>
    decide.mutate({
      id: subject.id,
      changeId: receipt.id,
      version: review.version,
      action,
    });
  return (
    <div className="card-actions">
      {review.can_undo &&
        (review.kind === "close_date" ? (
          <ReceiptUndo receipt={receipt} />
        ) : (
          <Button
            variant="ghost"
            pending={decide.isPending}
            onClick={() => press("undo")}
          >
            {t("common.undo")}
          </Button>
        ))}
      {review.can_accept && (
        <Button pending={decide.isPending} onClick={() => press("accept")}>
          {t("brief.changes.accept")}
        </Button>
      )}
      {decide.error && (
        <p role="alert" className="t-caption">
          {problemMessageOf(decide.error, t)}
        </p>
      )}
    </div>
  );
}
