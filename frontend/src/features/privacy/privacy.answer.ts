// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useState } from "react";
import type { components } from "../../api/schema";
import type { Translator } from "../../i18n";
import { type DsrClosing, ERASURE_ANSWER_LABEL } from "../../lib/privacy.logic";

type DataSubjectRequest = components["schemas"]["DataSubjectRequest"];

// The answer someone recorded, or null when the erasure that retired the
// case left its placeholder in its place.
export function writtenAnswer(dsr: DataSubjectRequest): string | null {
  return dsr.resolution_removed ? null : (dsr.resolution ?? null);
}

// An erasure's answer is a fixed word, shown in the reader's language; the
// officer's own answer shows as written.
export function answerText(dsr: DataSubjectRequest, t: Translator): string {
  const written = writtenAnswer(dsr);
  if (written === null) return t("privacy.answerRemoved");
  const fixed = ERASURE_ANSWER_LABEL[written];
  return fixed ? t(fixed) : written;
}

// The answer box rests on the answer the server holds and on whether the subject
// is erased. When either changes underneath, the draft resting on the old one goes.
export function useClosingDraft(
  dsr: DataSubjectRequest,
): readonly [DsrClosing, (change: Partial<DsrClosing>) => void] {
  const held = writtenAnswer(dsr) ?? "";
  const erased = Boolean(dsr.subject_erased);
  const [draft, setDraft] = useState<DsrClosing>(() => requestClosing(dsr));
  const [basis, setBasis] = useState({ held, erased });
  if (basis.held !== held || basis.erased !== erased) {
    setBasis({ held, erased });
    setDraft((current) => ({ ...current, resolution: held }));
  }
  function edit(change: Partial<DsrClosing>) {
    setDraft((current) => ({ ...current, ...change }));
  }
  return [draft, edit];
}

function requestClosing(dsr: DataSubjectRequest): DsrClosing {
  return {
    resolution: writtenAnswer(dsr) ?? "",
    identityCheck: dsr.identity_verified ? "confirm_link" : "",
    requesterEmail: "",
  };
}
