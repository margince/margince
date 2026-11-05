// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The words for what an Undo of a Margince change did. A close keeps its own
// sentence: Margince closes the deal again unless somebody writes why it is
// alive. Every other kind names the field it will now leave alone.

import type { Translator } from "../i18n";
import type { MessageKey } from "../i18n/en";
import type { UndoMarginceChangeResult } from "./marginchanges";

const FIELD_OF: Readonly<Record<string, MessageKey>> = {
  deal_close_date: "changes.field.closeDate",
  close_date_correction: "changes.field.closeDate",
  deal_amount: "changes.field.amount",
  deal_probability: "changes.field.probability",
  stage_move: "changes.field.stage",
};

/** The field a change of this kind sets, or null for a close or another kind. */
export function changeFieldKey(kind: string | undefined): MessageKey | null {
  return (kind && FIELD_OF[kind]) || null;
}

function fieldList(kinds: readonly (string | undefined)[], t: Translator) {
  const names = [
    ...new Set(
      kinds.map((kind) =>
        kind === "close_lost"
          ? t("changes.field.status")
          : t(changeFieldKey(kind) ?? "changes.field.status"),
      ),
    ),
  ];
  return names.reduce((first, second) =>
    t("changes.fieldJoin", { first, second }),
  );
}

/** One deal's Undo answer as the sentence the reader sees. */
export function undoResultText(
  result: UndoMarginceChangeResult,
  t: Translator,
): string {
  if (result.outcome === "refused") {
    return result.kept.some((kept) => kept.reason === "accepted")
      ? t("changes.alreadyAccepted")
      : t("changes.refused");
  }
  if (result.outcome === "partly_undone") {
    return t("changes.partly", {
      field: fieldList(
        result.kept.map((kept) => kept.kind),
        t,
      ),
    });
  }
  if (result.undone.includes("deal_open")) {
    return t("changes.undo.archived");
  }
  if (result.undone.includes("deal_revive")) {
    return t("changes.undo.relost");
  }
  if (result.undone.includes("close_lost")) {
    return t("worklist.handled.closeLostUndone");
  }
  return t("worklist.handled.verdictUndone", {
    field: fieldList(result.undone, t),
  });
}
