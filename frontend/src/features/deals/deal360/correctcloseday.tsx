// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../../../api/client";
import type { components } from "../../../api/schema";
import { ifMatch, requireVersion } from "../../../api/version";
import { Button, Field } from "../../../design-system/atoms";
import { ConfirmModal } from "../../../design-system/confirmmodal";
import { DateInput, isISODate } from "../../../design-system/dateinput";
import { ErrorLine } from "../../../design-system/errorline";
import { calendarDay } from "../../../format/calendarday";
import { useT } from "../../../i18n";
import { dealRecordKeys } from "../../../lib/activitykeys";
import { unwrap } from "../../../lib/problem";
import { useRecordZone } from "../../../lib/recordzone";

type Deal = components["schemas"]["Deal"];

/**
 * The person's correction of the day a won or lost deal closed. The server
 * moves `closed_at`, freezes the exchange rate again for the new day, and
 * refuses an agent, so this verb is a human's alone.
 */
export function CorrectCloseDayAction({
  deal,
  disabledReasonId,
}: Readonly<{ deal: Deal; disabledReasonId?: string }>) {
  const t = useT();
  const zone = useRecordZone();
  const queryClient = useQueryClient();
  const [open, setOpen] = useState(false);
  const current = deal.closed_at
    ? calendarDay(new Date(deal.closed_at), zone)
    : "";
  const [day, setDay] = useState(current);
  const correct = useMutation({
    mutationKey: ["deal-edit", deal.id],
    mutationFn: async (closedOn: string) =>
      unwrap(
        await api.POST("/deals/{id}/close-day", {
          params: {
            path: { id: deal.id },
            ...ifMatch(requireVersion(deal.version)),
          },
          body: { closed_on: closedOn },
        }),
        t,
      ),
    onSuccess: () => {
      setOpen(false);
      for (const queryKey of dealRecordKeys(deal.id)) {
        queryClient.invalidateQueries({ queryKey });
      }
      queryClient.invalidateQueries({ queryKey: ["deals"] });
    },
  });
  return (
    <>
      <Button
        reasonId={disabledReasonId}
        data-testid="correct-close-day-open"
        onClick={() => {
          setDay(current);
          setOpen(true);
        }}
      >
        {t("deal.correctCloseDay")}
      </Button>
      <ConfirmModal
        open={open}
        onClose={() => setOpen(false)}
        title={t("deal.correctCloseDayTitle")}
        confirmLabel={t("deal.correctCloseDayConfirm")}
        confirmDisabled={!isISODate(day) || day === current}
        pending={correct.isPending}
        onConfirm={() => correct.mutate(day)}
      >
        <Field
          label={t("deal.correctCloseDayLabel")}
          hint={t("deal.correctCloseDayHint")}
        >
          {(control) => (
            <DateInput
              {...control}
              data-testid="correct-close-day-input"
              value={isISODate(day) ? day : ""}
              max={calendarDay(new Date(), zone)}
              onChange={(event) => setDay(event.target.value)}
            />
          )}
        </Field>
        <ErrorLine error={correct.error} />
      </ConfirmModal>
    </>
  );
}
