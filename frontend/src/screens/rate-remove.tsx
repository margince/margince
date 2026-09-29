// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { ConfirmModal } from "../design-system/confirmmodal";
import { useT } from "../i18n";
import { problemMessageOf, throwProblem } from "./common";
import { LANE_LABEL } from "./rate-manual";

type SheetRow = components["schemas"]["AiModelRate"];

/**
 * RemovePriceDialog asks before a model's entry leaves the sheet. Every date of
 * it goes, and the calls it priced read as unpriced from then on, which is why
 * the question is asked at all. A refusal stays in the dialog with the entry
 * still on the sheet behind it. The question names the lane as well as the
 * model, since one id can be priced on two lanes and only one is leaving.
 */
export function RemovePriceDialog({
  row,
  onClose,
  returnFocusTo,
}: Readonly<{
  row: SheetRow;
  onClose: () => void;
  // The row's own verb is gone once the entry is; the sheet names where focus
  // lands instead.
  returnFocusTo: () => HTMLElement | null;
}>) {
  const t = useT();
  const qc = useQueryClient();
  const remove = useMutation({
    mutationFn: async (entry: SheetRow) => {
      const { error } = await api.DELETE("/ai-model-rates", {
        params: {
          query: {
            provider: entry.provider,
            model_id: entry.model_id,
            lane: entry.lane,
          },
        },
      });
      if (error) {
        throwProblem(error);
      }
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["ai-model-rates"] });
      onClose();
    },
  });
  const named = { model: row.model_id, lane: t(LANE_LABEL[row.lane]) };
  return (
    <ConfirmModal
      open
      onClose={onClose}
      title={t("aiRates.remove.title", named)}
      confirmLabel={t("aiRates.remove.confirm")}
      confirmVariant="danger"
      pending={remove.isPending}
      error={remove.error ? problemMessageOf(remove.error, t) : null}
      returnFocusTo={returnFocusTo}
      onConfirm={() => remove.mutate(row)}
    >
      <p>{t("aiRates.remove.body", named)}</p>
    </ConfirmModal>
  );
}
