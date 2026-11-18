// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQueryClient } from "@tanstack/react-query";
import type { components } from "../../../api/schema";
import { Button } from "../../../design-system/atoms";
import { useToast } from "../../../design-system/toast";
import { useT } from "../../../i18n";
import type { MessageKey } from "../../../i18n/en";
import { dealStageMoveKeys } from "../../../lib/activitykeys";
import { undoRefusalKey } from "../../../lib/historyundo";
import { problemMessageOf } from "../../../lib/problem";
import { useRecordRestore } from "../../../lib/recordrestore";
import { useRecordHistory } from "../../records/historyentries";

type Deal = components["schemas"]["Deal"];
type AuditHistoryEntry = components["schemas"]["AuditHistoryEntry"];

/**
 * Brings an archived deal back by putting its latest archive entry back, the
 * restore its history's Undo presses, refused with the server's own reason.
 */
export function RestoreDealAction({ deal }: Readonly<{ deal: Deal }>) {
  const t = useT();
  const toast = useToast();
  const queryClient = useQueryClient();
  const archives = useRecordHistory("deal", deal.id, true, "archive");
  const entry = archives.data?.pages[0]?.data?.[0];
  const restore = useRecordRestore({
    onSuccess: () => {
      for (const queryKey of [
        ...dealStageMoveKeys(deal.id),
        ["deals"],
        ["record-history", "deal", deal.id],
      ]) {
        queryClient.invalidateQueries({ queryKey });
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
