// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useState } from "react";
import type {
  ReportingEvaluation,
  ReportingEvidenceRef,
} from "./reporting.model";

/**
 * What a reader opened from a view, for as long as the question it was read
 * from stands. A changed selection closes it for good, so a drawer never lists
 * one question's records under another's figures, and going back to the first
 * question does not reopen it.
 */
export function useSelectionHeld<Held>(question: string) {
  const [held, setHeld] = useState<Readonly<{
    question: string;
    item: Held;
  }> | null>(null);
  if (held && held.question !== question) setHeld(null);
  const item = held?.question === question ? held.item : null;
  const open = (next: Held) => setHeld({ question, item: next });
  const close = () => setHeld(null);
  return { held: item, open, close };
}

/**
 * The evidence a reader opened, with the evaluation it was opened from: the
 * opener's own, or else `current` as it stood at the press. The drawer lists
 * the records under that receipt, so a later refetch never moves it.
 */
export function useSelectionEvidence(
  question: string,
  current?: ReportingEvaluation,
) {
  const { held, open, close } =
    useSelectionHeld<
      Readonly<{ reference: ReportingEvidenceRef; from?: ReportingEvaluation }>
    >(question);
  return {
    evidence: held?.reference ?? null,
    evidenceFrom: held?.from,
    openEvidence: (
      reference: ReportingEvidenceRef,
      from: ReportingEvaluation | undefined = current,
    ) => open({ reference, from }),
    closeEvidence: close,
  };
}
