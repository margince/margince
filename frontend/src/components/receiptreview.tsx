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
import { isReviewableReceipt, receiptAuditId } from "../lib/receiptreview";
import { invalidateHandledChange, worklistKey } from "../lib/workkeys";
import { ReceiptUndo } from "./receiptundo";

/**
 * Marks applied changes reviewed, one request each in order, so a failure
 * names the change it stopped at. `onDone` hears a run that answered.
 */
export function useAcceptChanges(onDone?: () => void) {
  const client = useQueryClient();
  return useMutation({
    mutationFn: async (receipts: readonly Receipt[]) => {
      for (const receipt of receipts) {
        const subject = receipt.subject;
        const changeId = receiptAuditId(receipt);
        const version = receipt.review?.version;
        if (subject?.type !== "deal" || !changeId || version === undefined) {
          continue;
        }
        const { error } = await api.POST(
          "/deals/{id}/applied-changes/{changeId}/accept",
          {
            params: {
              path: { id: subject.id, changeId },
              ...ifMatch(version),
            },
          },
        );
        if (error) throwProblem(error);
      }
    },
    onError: () => {
      client.invalidateQueries({ queryKey: worklistKey });
    },
    onSuccess: () => {
      onDone?.();
      void invalidateHandledChange(client);
    },
  });
}

/** Whether the reader may still mark the receipt reviewed. */
export function acceptable(receipt: Receipt): boolean {
  const review = receipt.review;
  return Boolean(
    review?.can_accept &&
      review.writable &&
      !review.accepted &&
      !review.reversed &&
      isReviewableReceipt(receipt) &&
      receipt.subject?.type === "deal" &&
      receiptAuditId(receipt),
  );
}

/**
 * The verbs of one applied change: Mark reviewed, and Undo. Marking it
 * reviewed only says the reader has seen it, so a reviewed change keeps its
 * Undo for as long as the server offers one. `onDone` hears either answer.
 * Without `offerAccept`, the review is asked for elsewhere, once for many.
 */
export function ReceiptReview({
  receipt,
  onDone,
  offerAccept = true,
}: Readonly<{ receipt: Receipt; onDone?: () => void; offerAccept?: boolean }>) {
  const t = useT();
  const writable = useCanWrite("deal", "update");
  const decide = useAcceptChanges(onDone);
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
  if (
    !writable ||
    !review.writable ||
    receipt.subject?.type !== "deal" ||
    !receiptAuditId(receipt)
  )
    return null;
  if (!offerAccept || !acceptable(receipt)) return undo || null;
  return (
    <div className="card-actions">
      <Button
        pending={decide.isPending}
        onClick={() => decide.mutate([receipt])}
      >
        {t("brief.changes.markReviewed")}
      </Button>
      {undo}
      <ErrorLine error={decide.error} />
    </div>
  );
}
