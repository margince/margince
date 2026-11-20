// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQueryClient } from "@tanstack/react-query";
import type { components } from "../../../api/schema";
import { Button } from "../../../design-system/atoms";
import { Callout } from "../../../design-system/callout";
import { useToast } from "../../../design-system/toast";
import { formatNumber } from "../../../format/format";
import { useLocale, usePlural, useT } from "../../../i18n";
import type { MessageKey } from "../../../i18n/en";
import { dealStageMoveKeys } from "../../../lib/activitykeys";
import { useCanWrite } from "../../../lib/capability";
import { undoRefusalKey } from "../../../lib/historyundo";
import { problemMessageOf } from "../../../lib/problem";
import { useRecordHistory } from "../../../lib/recordhistory";
import { useRecordRestore } from "../../../lib/recordrestore";

type Deal = components["schemas"]["Deal"];
type AuditHistoryEntry = components["schemas"]["AuditHistoryEntry"];

/**
 * The archived deal's band: the sentence every refused verb points at, and the
 * way back beside it for a reader who may archive deals. The id sits on the
 * sentence alone, so a refused verb is not described by the button's label.
 */
export function ArchivedDealBand({
  deal,
  reason,
  reasonId,
}: Readonly<{ deal: Deal; reason: string; reasonId: string }>) {
  // The way back asks what the archive asks: delete, or edit rights for the
  // deal's owner. The history's answer refuses the rest, with its reason.
  const canDelete = useCanWrite("deal", "delete");
  const canEdit = useCanWrite("deal", "update");
  const canRestore = canDelete || canEdit;
  return (
    <Callout
      kind="standing"
      title={<span id={reasonId}>{reason}</span>}
      actions={canRestore ? <RestoreDealAction deal={deal} /> : undefined}
    />
  );
}

/**
 * Brings an archived deal back by putting its latest archive entry back, the
 * restore its history's Undo presses, refused with the server's own reason.
 */
export function RestoreDealAction({ deal }: Readonly<{ deal: Deal }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const toast = useToast();
  const queryClient = useQueryClient();
  const archives = useRecordHistory("deal", deal.id, true, "archive");
  const entry = archives.data?.pages[0]?.data?.[0];
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
      toast.show(problemMessageOf(error, t), { tone: "danger" });
    },
  });
  const refused = restoreRefusal(archives.isPending, entry, deal.version);
  return (
    <Button
      disabled={archives.isPending || restore.isPending}
      reason={refused && t(refused)}
      onClick={() => {
        if (!refused && entry && deal.version !== undefined) {
          restore.mutate({
            kind: "deal",
            id: deal.id,
            auditId: entry.id,
            version: deal.version,
          });
        }
      }}
      data-testid="restore-record"
    >
      {t("deal.restore")}
    </Button>
  );
}

// Why the deal cannot come back from here. This reader may see no archive
// entry or no version to pin, or the server may give its own reason.
function restoreRefusal(
  pending: boolean,
  entry: AuditHistoryEntry | undefined,
  version: number | undefined,
): MessageKey | undefined {
  if (pending) {
    return undefined;
  }
  if (!entry || version === undefined) {
    return "record.actionRefused";
  }
  return entry.undoable?.undoable === false
    ? (undoRefusalKey(entry.undoable.reason) ?? "record.actionRefused")
    : undefined;
}
