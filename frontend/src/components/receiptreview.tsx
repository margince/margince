// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "../api/client";
import { ifMatch } from "../api/version";
import { Button } from "../design-system/atoms";
import { ErrorLine } from "../design-system/errorline";
import { useT } from "../i18n";
import { useCanWrite } from "../lib/capability";
import type { Receipt } from "../lib/handled";
import { throwProblem } from "../lib/problem";
import { isReviewableReceipt } from "../lib/receiptreview";
import { invalidateHandledChange, worklistKey } from "../lib/workkeys";
import { ReceiptUndo } from "./receiptundo";

/**
 * The verbs of one applied change: Mark reviewed, and Undo. Marking it
 * reviewed only says the reader has seen it, so a reviewed change keeps its
 * Undo for as long as the server offers one. `onDone` hears either answer.
 */
export function ReceiptReview({
  receipt,
  onDone,
}: Readonly<{ receipt: Receipt; onDone?: () => void }>) {
  const t = useT();
  const client = useQueryClient();
  const writable = useCanWrite("deal", "update");
  const decide = useMutation({
    mutationFn: async (input: {
      id: string;
      changeId: string;
      version: number;
    }) => {
      const { error } = await api.POST(
        "/deals/{id}/applied-changes/{changeId}/accept",
        {
          params: {
            path: { id: input.id, changeId: input.changeId },
            ...ifMatch(input.version),
          },
        },
      );
      if (error) throwProblem(error);
    },
    onError: () => {
      client.invalidateQueries({ queryKey: worklistKey });
    },
    onSuccess: () => {
      onDone?.();
      void invalidateHandledChange(client);
    },
  });
  const review = receipt.review;
  if (!review) return <ReceiptUndo receipt={receipt} onDone={onDone} />;
  if (review.reversed) return <span>{t("brief.changes.undone")}</span>;
  const undo = review.can_undo && (
    <ReceiptUndo receipt={receipt} onDone={onDone} />
  );
  if (review.accepted) {
    return (
      <div className="card-actions">
        <span role="status">{t("brief.changes.reviewed")}</span>
        {undo}
      </div>
    );
  }
  if (!isReviewableReceipt(receipt))
    return <p>{t("brief.changes.superseded")}</p>;
  const subject = receipt.subject;
  if (!writable || !review.writable || subject?.type !== "deal") return null;
  const markReviewed = () =>
    decide.mutate({
      id: subject.id,
      changeId: receipt.id,
      version: review.version,
    });
  return (
    <div className="card-actions">
      {review.can_accept && (
        <Button pending={decide.isPending} onClick={markReviewed}>
          {t("brief.changes.markReviewed")}
        </Button>
      )}
      {undo}
      <ErrorLine error={decide.error} />
    </div>
  );
}
