// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useState } from "react";
import type {
  ReportingEvaluation,
  ReportingEvidenceRef,
} from "./reporting.model";

/**
 * What a reader opened from a view, for as long as the question it was read
 * from stands. A changed selection closes it, so a drawer never lists one
 * question's records under another's figures.
 */
export function useSelectionHeld<Held>(question: string) {
  const [held, setHeld] = useState<Readonly<{
    question: string;
    item: Held;
  }> | null>(null);
  const item = held?.question === question ? held.item : null;
  const open = (next: Held) => setHeld({ question, item: next });
  const close = () => setHeld(null);
  return { held: item, open, close };
}

/**
 * The evidence a reader opened. `from` is the evaluation the reference came
 * from, when a view reads its charts apart from its headline figures.
 */
export function useSelectionEvidence(question: string) {
  const { held, open, close } =
    useSelectionHeld<
      Readonly<{ reference: ReportingEvidenceRef; from?: ReportingEvaluation }>
    >(question);
  return {
    evidence: held?.reference ?? null,
    evidenceFrom: held?.from,
    openEvidence: (
      reference: ReportingEvidenceRef,
      from?: ReportingEvaluation,
    ) => open({ reference, from }),
    closeEvidence: close,
  };
}
