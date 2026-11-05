// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Trash2 } from "lucide-react";
import { useState } from "react";
import { api } from "../../../api/client";
import type { components } from "../../../api/schema";
import { Button, Field, TextInput } from "../../../design-system/atoms";
import { Callout } from "../../../design-system/callout";
import { ConfirmModal } from "../../../design-system/confirmmodal";
import { IconAction } from "../../../design-system/iconaction";
import { MoneyInput } from "../../../design-system/moneyinput";
import {
  RecordPicker,
  type RecordPickerCandidate,
} from "../../../design-system/recordpicker";
import { formatMoneyOrAbsent } from "../../../format/format";
import { useLocale, useT } from "../../../i18n";
import type { MessageKey } from "../../../i18n/en";
import { problemMessageOf, unwrap } from "../../../lib/problem";
import { REPORTING_EVALUATION_KEY } from "../../../lib/reportingfigures";
import {
  EMPTY_LINE_BILLING,
  type LineBilling,
  lineBillingBody,
  OfferLineBillingFields,
} from "../offerlinebilling";
import { OfferCurrencyField } from "../offers.currencyfield";
import { searchProductCandidates } from "../products";

export type DealCommercial = components["schemas"]["DealCommercial"];
type DealLine = components["schemas"]["DealLine"];

export const dealLinesKey = (dealId: string) => ["deal-lines", dealId] as const;

/** The deal's lines and the value they add up to. */
export function useDealLines(dealId: string) {
  const t = useT();
  return useQuery({
    queryKey: dealLinesKey(dealId),
    queryFn: async (): Promise<DealCommercial> =>
      unwrap(
        await api.GET("/deals/{id}/lines", {
          params: { path: { id: dealId } },
        }),
        t,
      ),
  });
}

/** One row the editor holds. `id` is the saved line, absent on a new one. */
type DraftLine = {
  key: string;
  id?: string;
  productId?: string;
  description: string;
  quantity: string;
  priceMinor: number | null;
  billing: LineBilling;
};

function billingOfLine(line: DealLine): LineBilling {
  return {
    billingModel: line.billing_model ?? "",
    billingIntervalMonths: line.billing_interval_months
      ? String(line.billing_interval_months)
      : "",
    intervalCount: line.interval_count ? String(line.interval_count) : "",
  };
}

function draftOf(line: DealLine): DraftLine {
  return {
    key: line.id,
    id: line.id,
    productId: line.product_id ?? undefined,
    description: line.description,
    quantity: line.quantity == null ? "" : String(line.quantity),
    priceMinor: line.unit_price_minor ?? null,
    billing: billingOfLine(line),
  };
}

function quantityOf(text: string): number | null {
  const value = Number(text);
  return text.trim() === "" || !Number.isFinite(value) ? null : value;
}

/** The fields of a saved line that the draft changed, as a PATCH body. */
function changedFields(saved: DealLine, draft: DraftLine) {
  const body: Omit<
    components["schemas"]["UpdateDealLineRequest"],
    "expected_revision"
  > = {};
  if (draft.description.trim() !== saved.description) {
    body.description = draft.description.trim();
  }
  const quantity = quantityOf(draft.quantity);
  if (quantity !== (saved.quantity ?? null)) {
    body.quantity = quantity;
  }
  if (draft.priceMinor !== (saved.unit_price_minor ?? null)) {
    body.unit_price_minor = draft.priceMinor;
  }
  const before = billingOfLine(saved);
  if (JSON.stringify(before) !== JSON.stringify(draft.billing)) {
    const wire = lineBillingBody(draft.billing);
    body.billing_model = wire.billing_model ?? null;
    body.billing_interval_months = wire.billing_interval_months ?? null;
    body.interval_count = wire.interval_count ?? null;
  }
  return Object.keys(body).length > 0 ? body : null;
}

/**
 * Saves the draft against the server one change at a time. Each answer
 * carries the next revision, so a colleague's edit in between stops the save
 * with a reload message instead of being overwritten.
 */
async function saveDraft(
  dealId: string,
  start: DealCommercial,
  draft: readonly DraftLine[],
  replaceEstimate: boolean,
  t: (key: MessageKey) => string,
): Promise<DealCommercial> {
  let state = start;
  const kept = new Set(draft.flatMap((row) => (row.id ? [row.id] : [])));
  for (const line of start.lines.filter((row) => !kept.has(row.id))) {
    state = unwrap(
      await api.DELETE("/deals/{id}/lines/{lineId}", {
        params: {
          path: { id: dealId, lineId: line.id },
          query: { expected_revision: state.commercial_revision },
        },
      }),
      t,
    );
  }
  for (const row of draft) {
    const saved = start.lines.find((line) => line.id === row.id);
    if (saved) {
      const body = changedFields(saved, row);
      if (body) {
        state = unwrap(
          await api.PATCH("/deals/{id}/lines/{lineId}", {
            params: { path: { id: dealId, lineId: saved.id } },
            body: { ...body, expected_revision: state.commercial_revision },
          }),
          t,
        );
      }
      continue;
    }
    state = unwrap(
      await api.POST("/deals/{id}/lines", {
        params: { path: { id: dealId } },
        body: {
          expected_revision: state.commercial_revision,
          replace_estimate: replaceEstimate || undefined,
          currency: state.currency ?? undefined,
          product_id: row.productId,
          description: row.productId ? undefined : row.description.trim(),
          quantity: quantityOf(row.quantity),
          unit_price_minor: row.productId ? undefined : row.priceMinor,
          ...lineBillingBody(row.billing),
        },
      }),
      t,
    );
  }
  return state;
}

const MISSING_LABEL: Record<DealCommercial["missing"][number], MessageKey> = {
  lines: "deal.lines.missing.lines",
  quantity: "deal.lines.missing.quantity",
  unit_price: "deal.lines.missing.unitPrice",
  billing_model: "deal.lines.missing.billing",
  interval_count: "deal.lines.missing.term",
  tax_rate: "deal.lines.missing.taxRate",
};

/** What the deal's lines add up to, or why they do not yet. */
export function DealLinesTotal({ state }: Readonly<{ state: DealCommercial }>) {
  const t = useT();
  const { locale } = useLocale();
  if (state.money_withheld) {
    return <p className="t-caption">{t("deal.lines.withheld")}</p>;
  }
  if (state.complete) {
    return (
      <p className="t-body">
        {t("deal.lines.committedNet", {
          amount: formatMoneyOrAbsent(
            state.committed_net_minor,
            state.currency,
            locale,
          ),
        })}
      </p>
    );
  }
  return (
    <p className="t-caption">
      {t("deal.lines.incomplete", {
        known: formatMoneyOrAbsent(
          state.known_committed_net_minor,
          state.currency,
          locale,
        ),
        missing: state.missing
          .map((reason) => t(MISSING_LABEL[reason]))
          .join(", "),
      })}
    </p>
  );
}

function DraftLineRow({
  row,
  currency,
  onChange,
  onRemove,
}: Readonly<{
  row: DraftLine;
  currency: string | null;
  onChange: (next: DraftLine) => void;
  onRemove: () => void;
}>) {
  const t = useT();
  return (
    <li className="deal-lines-edit-row">
      <Field label={t("deal.lines.description")}>
        {(control) => (
          <TextInput
            {...control}
            value={row.description}
            onChange={(event) =>
              onChange({ ...row, description: event.target.value })
            }
          />
        )}
      </Field>
      <Field label={t("deal.lines.quantity")}>
        {(control) => (
          <TextInput
            {...control}
            type="number"
            min="0"
            step="any"
            inputMode="decimal"
            value={row.quantity}
            onChange={(event) =>
              onChange({ ...row, quantity: event.target.value })
            }
          />
        )}
      </Field>
      {currency && (
        <Field label={t("deal.lines.price")}>
          {(control) => (
            <MoneyInput
              {...control}
              currency={currency}
              valueMinor={row.priceMinor ?? 0}
              blankWhenZero={row.priceMinor === null}
              onChangeMinor={(minor) => onChange({ ...row, priceMinor: minor })}
              onClear={() => onChange({ ...row, priceMinor: null })}
            />
          )}
        </Field>
      )}
      <OfferLineBillingFields
        value={row.billing}
        onChange={(billing) => onChange({ ...row, billing })}
      />
      <IconAction
        icon={<Trash2 />}
        label={t("deal.lines.remove", { name: row.description })}
        onClick={onRemove}
      />
    </li>
  );
}

/**
 * The editor of a deal's products and services. Saving prices the deal from
 * its lines; a typed value the deal held is replaced, and the dialog says so
 * before the save.
 */
export function DealLinesEditor({
  dealId,
  state,
  typedAmount,
  onClose,
}: Readonly<{
  dealId: string;
  state: DealCommercial;
  // The value a human typed, which the first line replaces. Null when none.
  typedAmount: string | null;
  onClose: () => void;
}>) {
  const t = useT();
  const client = useQueryClient();
  const [draft, setDraft] = useState<DraftLine[]>(() =>
    state.lines.map(draftOf),
  );
  const [custom, setCustom] = useState("");
  const [currency, setCurrency] = useState(state.currency ?? "");
  const replaces = state.mode !== "lines" && typedAmount !== null;
  const save = useMutation({
    mutationFn: () =>
      saveDraft(
        dealId,
        { ...state, currency: state.currency ?? (currency || null) },
        draft,
        replaces,
        t,
      ),
    onSuccess: (saved) => {
      client.setQueryData(dealLinesKey(dealId), saved);
      void client.invalidateQueries({ queryKey: ["deal", dealId] });
      void client.invalidateQueries({ queryKey: ["deal-products", dealId] });
      void client.invalidateQueries({ queryKey: REPORTING_EVALUATION_KEY });
      onClose();
    },
  });
  const update = (key: string, next: DraftLine) =>
    setDraft((rows) => rows.map((row) => (row.key === key ? next : row)));
  const remove = (key: string) =>
    setDraft((rows) => rows.filter((row) => row.key !== key));
  const addProduct = (candidate: RecordPickerCandidate) =>
    setDraft((rows) => [
      ...rows,
      {
        key: `new-${rows.length}-${candidate.id}`,
        productId: candidate.id,
        description: candidate.name,
        quantity: "",
        priceMinor: null,
        billing: EMPTY_LINE_BILLING,
      },
    ]);
  const addCustom = () => {
    if (!custom.trim()) {
      return;
    }
    setDraft((rows) => [
      ...rows,
      {
        key: `new-${rows.length}-custom`,
        description: custom.trim(),
        quantity: "",
        priceMinor: null,
        billing: { ...EMPTY_LINE_BILLING, billingModel: "one_time" },
      },
    ]);
    setCustom("");
  };
  return (
    <ConfirmModal
      open
      intent="form"
      onClose={onClose}
      title={t("deal.lines.editTitle")}
      confirmLabel={t("deal.lines.save")}
      pending={save.isPending}
      error={save.error ? problemMessageOf(save.error, t) : null}
      onConfirm={() => save.mutate()}
    >
      <p className="t-body">{t("deal.lines.intro")}</p>
      {replaces && (
        <Callout
          tone="warning"
          title={t("deal.lines.replacesEstimate", {
            amount: typedAmount ?? "",
          })}
        />
      )}
      {!state.currency && (
        <OfferCurrencyField
          value={currency}
          lineItems={[]}
          onChange={setCurrency}
        />
      )}
      {draft.length > 0 && (
        <ul className="deal-lines-edit">
          {draft.map((row) => (
            <DraftLineRow
              key={row.key}
              row={row}
              currency={state.currency ?? (currency || null)}
              onChange={(next) => update(row.key, next)}
              onRemove={() => remove(row.key)}
            />
          ))}
        </ul>
      )}
      <RecordPicker
        label={t("deal.lines.addProduct")}
        searchTargets={searchProductCandidates}
        onPick={addProduct}
      />
      <Field label={t("deal.lines.customItem")}>
        {(control) => (
          <TextInput
            {...control}
            value={custom}
            onChange={(event) => setCustom(event.target.value)}
          />
        )}
      </Field>
      <Button onClick={addCustom} disabled={!custom.trim()}>
        {t("deal.lines.addCustom")}
      </Button>
    </ConfirmModal>
  );
}
