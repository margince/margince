// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useT } from "../i18n";
import type { Receipt } from "../lib/handled";
import { isOpenReceipt } from "../lib/receiptstate";
import { ReceiptUndo } from "./receiptundo";

/**
 * The verb of one applied change: its Undo, for as long as the server offers
 * one. Margince never asks the reader to approve a change it already made,
 * because the Undo is the answer. `onDone` hears the Undo.
 */
export function ReceiptVerbs({
  receipt,
  onDone,
}: Readonly<{ receipt: Receipt; onDone?: () => void }>) {
  const t = useT();
  const review = receipt.review;
  if (!review) return <ReceiptUndo receipt={receipt} onDone={onDone} />;
  if (review.reversed) return <span>{t("brief.changes.undone")}</span>;
  if (!isOpenReceipt(receipt)) return <p>{t("brief.changes.superseded")}</p>;
  return <ReceiptUndo receipt={receipt} onDone={onDone} />;
}
