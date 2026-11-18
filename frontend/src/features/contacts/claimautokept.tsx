// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "../../api/client";
import { Button } from "../../design-system/atoms";
import { ErrorLine } from "../../design-system/errorline";
import { useT } from "../../i18n";
import { CLAIM_SETTLED_KEYS } from "../../lib/activitykeys";
import { useCanWrite } from "../../lib/capability";
import { unwrap } from "../../lib/problem";

/**
 * A promise Margince closed on its own evidence says so and offers the undo.
 * The undo opens the promise again, and Margince does not close it on the same
 * evidence a second time.
 */
export function AutoKeptLine({ id }: Readonly<{ id: string }>) {
  const t = useT();
  const queryClient = useQueryClient();
  const reopen = useMutation({
    mutationFn: async (claimId: string) =>
      unwrap(
        await api.POST("/claims/{id}/reopen", {
          params: { path: { id: claimId } },
        }),
        t,
      ),
    onSettled: () => {
      for (const queryKey of CLAIM_SETTLED_KEYS) {
        queryClient.invalidateQueries({ queryKey });
      }
    },
  });
  // The undo writes the contact's record, so a reader who may not update
  // contacts is not offered a verb the server would refuse.
  const canUndo = useCanWrite("contact", "update");
  return (
    <span className="pe-loop-quote t-caption">
      {t("commitment.autoKept")}{" "}
      {canUndo && (
        <Button
          variant="link"
          pending={reopen.isPending}
          onClick={() => reopen.mutate(id)}
        >
          {t("commitment.undoKept")}
        </Button>
      )}
      <ErrorLine error={reopen.error} inline />
    </span>
  );
}
