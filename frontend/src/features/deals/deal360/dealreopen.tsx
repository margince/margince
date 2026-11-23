// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../../../api/client";
import type { components } from "../../../api/schema";
import { ifMatch, requireVersion } from "../../../api/version";
import { Button } from "../../../design-system/atoms";
import { useToast } from "../../../design-system/toast";
import { useLocale, useT } from "../../../i18n";
import { dealStageMoveKeys } from "../../../lib/activitykeys";
import { unwrap } from "../../../lib/problem";
import { advanceNote } from "../dealprobability";
import { ReopenModal } from "./reopenmodal";

type Stage = components["schemas"]["Stage"];

// Reopens a won or lost deal into an open stage. It uses the advance mutation
// the board drag uses, with status "open" forced. The header line and the
// overflow menu both offer it.
export function ReopenAction({
  dealId,
  dealVersion,
  openStages,
  pipelineRetired,
  disabledReasonId,
}: Readonly<{
  dealId: string;
  // The version the caller was rendered from, so the reopen pins the deal the
  // reader was looking at. The caller states it because this action holds no
  // query to read one from, and a fresh one would be the wrong answer.
  dealVersion: number | undefined;
  openStages: Stage[];
  // A retired pipeline takes no stage, so a closed deal on it cannot reopen.
  pipelineRetired?: boolean;
  // The id of the sentence saying why this reopen is refused, when it is.
  // A control blocked by the record's state stays visible and says why.
  // Hiding it would hide a fact the reader needs.
  disabledReasonId?: string;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const toast = useToast();
  const queryClient = useQueryClient();
  const [open, setOpen] = useState(false);
  const reopen = useMutation({
    mutationKey: ["deal-edit", dealId],
    // Stage and version both ride the variables. A version read from the
    // closure would be the one from before this dialog opened. A reopen that
    // pins the wrong version fails for no visible reason or lands on a deal
    // somebody else moved.
    mutationFn: async (input: {
      toStage: Stage;
      version: number | undefined;
    }) => {
      return unwrap(
        await api.POST("/deals/{id}/advance", {
          params: {
            path: { id: dealId },
            ...ifMatch(requireVersion(input.version)),
          },
          body: { to_stage_id: input.toStage.id, status: "open" },
        }),
        t,
      );
    },
    onSuccess: (advanced, input) => {
      setOpen(false);
      toast.show(
        advanceNote(
          advanced,
          { toStage: input.toStage, reopens: true },
          t,
          locale,
        ),
      );
      for (const queryKey of dealStageMoveKeys(dealId)) {
        queryClient.invalidateQueries({ queryKey });
      }
      queryClient.invalidateQueries({ queryKey: ["deals"] });
    },
  });
  if (pipelineRetired) {
    return null;
  }
  return (
    <>
      <Button
        reasonId={disabledReasonId}
        data-testid="reopen-open"
        onClick={() => setOpen(true)}
      >
        {t("deal.reopen")}
      </Button>
      <ReopenModal
        open={open}
        onClose={() => setOpen(false)}
        openStages={openStages}
        pending={reopen.isPending}
        error={reopen.error}
        onConfirm={(toStage) =>
          reopen.mutate({ toStage, version: dealVersion })
        }
      />
    </>
  );
}
