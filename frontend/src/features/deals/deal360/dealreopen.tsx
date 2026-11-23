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

// Reopens a won/lost deal back to an open-semantic stage — the same advance
// mutation shape the board drag uses, with status:"open" forced. Split out
// of DealActions for the same readability reason as the other header actions.
export function ReopenAction({
  dealId,
  dealVersion,
  openStages,
  pipelineRetired,
  disabledReasonId,
}: Readonly<{
  dealId: string;
  // The version the header this button sits in was rendered from, so the reopen
  // pins the deal the reader was looking at. Stated by the caller rather than
  // read here: this action holds no query of its own to read a fresh one from,
  // and a fresh one would be the wrong answer anyway.
  dealVersion: number | undefined;
  openStages: Stage[];
  // A retired pipeline takes no stage, so a closed deal on it cannot reopen.
  pipelineRetired?: boolean;
  // The id of the sentence saying why this reopen is refused, when it is.
  // STATE-4a: a control blocked by the record's STATE rather than by a
  // permission stays visible and says why, because the reason is the
  // information and hiding the control hides a fact the reader needs.
  disabledReasonId?: string;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const toast = useToast();
  const queryClient = useQueryClient();
  const [open, setOpen] = useState(false);
  const reopen = useMutation({
    mutationKey: ["deal-edit", dealId],
    // Stage and version both ride the variables: a version read out of the
    // closure would be the one from the render before this dialog opened, and a
    // reopen that pins the wrong version either fails for no reason the reader
    // can see or lands on a deal somebody else has since moved.
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
