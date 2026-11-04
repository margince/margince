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

export function ReceiptReview({ receipt }: Readonly<{ receipt: Receipt }>) {
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
      void invalidateHandledChange(client);
    },
  });
  const review = receipt.review;
  if (!review) return <ReceiptUndo receipt={receipt} />;
  if (review.reversed) return <span>{t("brief.changes.undone")}</span>;
  if (review.accepted) return <span>{t("brief.changes.accepted")}</span>;
  if (!isReviewableReceipt(receipt))
    return <p>{t("brief.changes.superseded")}</p>;
  const subject = receipt.subject;
  if (!writable || !review.writable || subject?.type !== "deal") return null;
  const accept = () =>
    decide.mutate({
      id: subject.id,
      changeId: receipt.id,
      version: review.version,
    });
  return (
    <div className="card-actions">
      {review.can_undo && <ReceiptUndo receipt={receipt} />}
      {review.can_accept && (
        <Button pending={decide.isPending} onClick={accept}>
          {t(
            review.kind === "close_date"
              ? "brief.changes.keepDate"
              : "brief.changes.accept",
          )}
        </Button>
      )}
      <ErrorLine error={decide.error} />
    </div>
  );
}
