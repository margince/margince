// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "../../api/client";
import { Button } from "../../design-system/atoms";
import { ErrorLine } from "../../design-system/errorline";
import { formatDayMonth } from "../../format/format";
import { useLocale, useT } from "../../i18n";
import { CLAIM_SETTLED_KEYS, taskWriteKeys } from "../../lib/activitykeys";
import { useCanWrite } from "../../lib/capability";
import { unwrap } from "../../lib/problem";
import { useRecordZone } from "../../lib/recordzone";
import { worklistKey } from "../../lib/workkeys";

/**
 * Taking over a promise one of us made that nobody was asked to keep. The
 * press makes it the reader's task; a colleague who pressed first is named in
 * the refusal, and the card then shows them.
 */
export function TakeOverPromiseButton({
  id,
  contactId,
}: Readonly<{ id: string; contactId: string }>) {
  const t = useT();
  const queryClient = useQueryClient();
  const take = useMutation({
    mutationFn: async (input: { id: string; contactId: string }) => {
      unwrap(
        await api.POST("/claims/{id}/take-over", {
          params: { path: { id: input.id } },
        }),
        t,
      );
      return input.contactId;
    },
    // Settled either way: a refusal means somebody else holds it now, and the
    // card should show who.
    onSettled: (_data, _error, input) => {
      for (const queryKey of [
        ...CLAIM_SETTLED_KEYS,
        ...taskWriteKeys("contact", input.contactId),
        worklistKey,
      ]) {
        queryClient.invalidateQueries({ queryKey });
      }
    },
  });
  // The press writes a task the reader holds, so a reader who may not create
  // one is not offered a verb the server would refuse.
  const canHoldTasks = useCanWrite("activity", "create");
  if (!canHoldTasks) {
    return null;
  }
  return (
    <>
      <Button
        variant="link"
        pending={take.isPending}
        onClick={() => take.mutate({ id, contactId })}
      >
        {t("commitment.takeOver")}
      </Button>
      <ErrorLine error={take.error} inline />
    </>
  );
}

/** Who took a promise over, and when. A deleted colleague's name is gone. */
export type TakenOver = { name: string | null; at: string };

export function TakenOverLine({ takenBy }: Readonly<{ takenBy: TakenOver }>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = useRecordZone();
  const date = formatDayMonth(takenBy.at, locale, zone);
  return (
    <span className="pe-loop-quote t-caption">
      {takenBy.name
        ? t("commitment.takenOver", { name: takenBy.name, date })
        : t("commitment.takenOverUnnamed", { date })}
    </span>
  );
}
