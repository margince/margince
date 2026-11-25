// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQueryClient } from "@tanstack/react-query";
import type { components } from "../../../api/schema";
import { MergedIntoSentence } from "../../../components/historymergedref";
import { Button } from "../../../design-system/atoms";
import { Callout } from "../../../design-system/callout";
import { useToast } from "../../../design-system/toast";
import { formatNumber } from "../../../format/format";
import { useLocale, usePlural, useT } from "../../../i18n";
import { dealStageMoveKeys } from "../../../lib/activitykeys";
import { useCanWrite } from "../../../lib/capability";
import { undoRefusalKey } from "../../../lib/historyundo";
import { problemCodeOf, problemMessageOf } from "../../../lib/problem";
import { useRecordHistory } from "../../../lib/recordhistory";
import { useRecordRestore } from "../../../lib/recordrestore";

type Deal = components["schemas"]["Deal"];
type AuditHistoryEntry = components["schemas"]["AuditHistoryEntry"];

/**
 * The archived deal's band: the sentence every refused verb points at, and the
 * way back beside it. The id sits on the sentence alone, so a refused verb is
 * not described by the button's label.
 */
export function ArchivedDealBand({
  deal,
  reason,
  reasonId,
}: Readonly<{ deal: Deal; reason: string; reasonId: string }>) {
  const t = useT();
  // The way back asks what the archive asks: delete, or edit rights for the
  // deal's owner. A reader with neither is not asked about the history.
  const canDelete = useCanWrite("deal", "delete");
  const canEdit = useCanWrite("deal", "update");
  const canRestore = canDelete || canEdit;
  const archives = useRecordHistory("deal", deal.id, canRestore, "archive");
  const entry = archives.data?.pages[0]?.data?.[0];
  // Offered only when the history says the archive can be put back. When it
  // says why not, the band says so instead of drawing a dead button.
  // A merged-away deal is never restored: its offers and contracts now belong
  // to the deal it merged into, so the band says where it went instead.
  const mergedInto = deal.merged_into_id ?? undefined;
  const undoable =
    !mergedInto && canRestore && entry?.undoable?.undoable === true;
  const refusal =
    !mergedInto && canRestore && entry?.undoable?.undoable === false
      ? undoRefusalKey(entry.undoable.reason)
      : undefined;
  return (
    <Callout
      kind="standing"
      title={<span id={reasonId}>{reason}</span>}
      actions={
        undoable && deal.version !== undefined ? (
          <RestoreDealAction deal={deal} entry={entry} version={deal.version} />
        ) : undefined
      }
    >
      {mergedInto && <MergedIntoSentence kind="deal" id={mergedInto} />}
      {refusal && t(refusal)}
    </Callout>
  );
}

/**
 * Brings an archived deal back by putting its latest archive entry back, the
 * restore its history's Undo presses.
 */
function RestoreDealAction({
  deal,
  entry,
  version,
}: Readonly<{ deal: Deal; entry: AuditHistoryEntry; version: number }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const toast = useToast();
  const queryClient = useQueryClient();
  const restore = useRecordRestore({
    onSuccess: (done) => {
      for (const queryKey of [
        ...dealStageMoveKeys(deal.id),
        ["deals"],
        ["record-history", "deal", deal.id],
      ]) {
        queryClient.invalidateQueries({ queryKey });
      }
      // A link whose list, tag or other record was archived since stays out,
      // and a bare "restored" would read as the whole deal coming back.
      const left = done.left_behind?.length ?? 0;
      if (left > 0) {
        toast.show(
          plural("history.undo.leftBehind", left, {
            count: formatNumber(left, locale),
          }),
          { tone: "warning" },
        );
        return;
      }
      toast.show(t("record.restoreDone", { name: deal.name }));
    },
    onError: (error) => {
      // A refusal found at the press reads as the one shown up front.
      const key = undoRefusalKey(problemCodeOf(error));
      toast.show(key ? t(key) : problemMessageOf(error, t), { tone: "danger" });
    },
  });
  return (
    <Button
      disabled={restore.isPending}
      onClick={() =>
        restore.mutate({
          kind: "deal",
          id: deal.id,
          auditId: entry.id,
          version,
        })
      }
      data-testid="restore-record"
    >
      {t("deal.restore")}
    </Button>
  );
}
