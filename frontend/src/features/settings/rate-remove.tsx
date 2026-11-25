// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { type ReactNode, useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { Button } from "../../design-system/atoms";
import { ConfirmModal } from "../../design-system/confirmmodal";
import type { DataTableColumn } from "../../design-system/datatable";
import { useT } from "../../i18n";
import { useCanWrite } from "../../lib/capability";
import { labelFor } from "../../lib/labelfor";
import { problemMessageOf, unwrap } from "../../lib/problem";
import { INSTALLATION_SETTINGS_KEY } from "../../lib/uploadlimit";
import { LANE_LABEL } from "./rate-manual";

type SheetRow = components["schemas"]["AiModelRate"];
type FxRate = components["schemas"]["FxRate"];

/**
 * The Remove column of the currency rate sheet and its confirm dialog. The
 * column is empty without the update grant on a full seat.
 */
export function useRemoveFxRate(): {
  columns: DataTableColumn<FxRate>[];
  dialog: ReactNode;
} {
  const t = useT();
  const canRemove = useCanWrite("fx_rate", "update");
  const [removing, setRemoving] = useState<FxRate | null>(null);
  return {
    columns: canRemove
      ? [
          {
            key: "verbs",
            header: t("settings.rates.fxRemoveConfirm"),
            headerHidden: true,
            render: (row) => (
              <Button onClick={() => setRemoving(row)}>
                {t("settings.rates.fxRemoveVerb", {
                  currency: row.from_currency,
                })}
              </Button>
            ),
          },
        ]
      : [],
    dialog: removing ? (
      <RemoveFxRateDialog row={removing} onClose={() => setRemoving(null)} />
    ) : null,
  };
}

/**
 * RemoveFxRateDialog asks before a currency leaves the rate sheet. Deals in it
 * read as unconverted afterwards, and the base currency lock stops counting it.
 * The server keeps the removed rows, so a figure already computed keeps its rate.
 */
export function RemoveFxRateDialog({
  row,
  onClose,
}: Readonly<{ row: FxRate; onClose: () => void }>) {
  const t = useT();
  const qc = useQueryClient();
  const remove = useMutation({
    mutationFn: async (from: string) => {
      unwrap(
        await api.DELETE("/fx-rates/{from}", { params: { path: { from } } }),
      );
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["fx-rates"] });
      qc.invalidateQueries({ queryKey: INSTALLATION_SETTINGS_KEY });
      onClose();
    },
  });
  const named = { currency: row.from_currency };
  return (
    <ConfirmModal
      open
      onClose={onClose}
      title={t("settings.rates.fxRemoveTitle", named)}
      confirmLabel={t("settings.rates.fxRemoveConfirm")}
      confirmVariant="danger"
      pending={remove.isPending}
      error={remove.error ? problemMessageOf(remove.error, t) : null}
      returnFocusTo={() => null}
      onConfirm={() => remove.mutate(row.from_currency)}
    >
      <p>{t("settings.rates.fxRemoveBody", named)}</p>
    </ConfirmModal>
  );
}

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
      unwrap(
        await api.DELETE("/ai-model-rates", {
          params: {
            query: {
              provider: entry.provider,
              model_id: entry.model_id,
              lane: entry.lane,
            },
          },
        }),
      );
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["ai-model-rates"] });
      onClose();
    },
  });
  const named = {
    model: row.model_id,
    lane: t(labelFor(LANE_LABEL, row.lane)),
  };
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
