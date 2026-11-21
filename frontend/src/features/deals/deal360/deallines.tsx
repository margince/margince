// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Trash2 } from "lucide-react";
import { useState } from "react";
import { api } from "../../../api/client";
import type { components } from "../../../api/schema";
import type { ProductCandidate } from "../../../components/productsearch";
import { Button, Field, TextInput } from "../../../design-system/atoms";
import { Callout } from "../../../design-system/callout";
import { ConfirmModal } from "../../../design-system/confirmmodal";
import { IconAction } from "../../../design-system/iconaction";
import { MoneyInput } from "../../../design-system/moneyinput";
import { RecordPicker } from "../../../design-system/recordpicker";
import {
  typedNumberFault,
  typedNumberOrNaN,
  writtenDecimal,
} from "../../../format/decimalinput";
import { formatNumber } from "../../../format/format";
import { type Locale, type Translator, useLocale, useT } from "../../../i18n";
import type { MessageKey } from "../../../i18n/en";
import { problemMessageOf, unwrap } from "../../../lib/problem";
import { REPORTING_EVALUATION_KEY } from "../../../lib/reportingfigures";
import {
  EMPTY_LINE_BILLING,
  type LineBilling,
  lineBillingBody,
  lineBillingOf,
  OfferLineBillingFields,
} from "../offerlinebilling";
import {
  discountFault,
  type LineRates,
  NewLineRates,
  taxRateFault,
} from "../offerlinerates";
import { OfferCurrencyField } from "../offers.currencyfield";
import { useProductCandidateSearch } from "../products";

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
type DraftLine = LineRates & {
  key: string;
  id?: string;
  productId?: string;
  description: string;
  quantity: string;
  priceMinor: number | null;
  billing: LineBilling;
  // The product's name and price as the picker named them, for a line not saved yet.
  catalogName?: string;
  catalogPriceMinor?: number;
};

// A rate card price is one in the lines' currency, so a new currency drops it.
function withoutCatalogPrices(rows: DraftLine[]): DraftLine[] {
  return rows.map((row) =>
    row.priceMinor === row.catalogPriceMinor
      ? { ...row, priceMinor: null, catalogPriceMinor: undefined }
      : row,
  );
}

function draftOf(line: DealLine, locale: Locale): DraftLine {
  return {
    key: line.id,
    id: line.id,
    productId: line.product_id ?? undefined,
    description: line.description,
    quantity: typedText(line.quantity, locale),
    priceMinor: line.unit_price_minor ?? null,
    billing: lineBillingOf(line),
    discount_pct: typedText(line.discount_pct, locale),
    tax_rate: typedText(line.tax_rate, locale),
  };
}

function typedText(value: number | null | undefined, locale: Locale): string {
  return value == null ? "" : writtenDecimal(String(value), locale);
}

// Empty means the stored default, so an emptied rate is no change to send.
function typedRate(text: string, locale: Locale): number | undefined {
  return text.trim() === "" ? undefined : typedNumberOrNaN(text, locale);
}

const NO_RATES: LineRates = { discount_pct: "", tax_rate: "" };

function lineFault(row: DraftLine, locale: Locale, t: Translator) {
  return (
    typedNumberFault(row.quantity, locale, t) ??
    discountFault(row.discount_pct, locale, t) ??
    taxRateFault(row.tax_rate, locale, t)
  );
}

function quantityOf(text: string, locale: Locale): number | null {
  const value = typedNumberOrNaN(text, locale);
  return Number.isNaN(value) ? null : value;
}

/** The fields of a saved line that the draft changed, as a PATCH body. */
function changedFields(saved: DealLine, draft: DraftLine, locale: Locale) {
  const body: Omit<
    components["schemas"]["UpdateDealLineRequest"],
    "expected_revision"
  > = {};
  if (draft.description.trim() !== saved.description) {
    body.description = draft.description.trim();
  }
  const quantity = quantityOf(draft.quantity, locale);
  if (quantity !== (saved.quantity ?? null)) {
    body.quantity = quantity;
  }
  if (draft.priceMinor !== (saved.unit_price_minor ?? null)) {
    body.unit_price_minor = draft.priceMinor;
  }
  // A typed tax rate drops the line's tax category, so only a changed one is sent.
  const discount = typedRate(draft.discount_pct, locale);
  if (discount !== undefined && discount !== saved.discount_pct) {
    body.discount_pct = discount;
  }
  const taxRate = typedRate(draft.tax_rate, locale);
  if (taxRate !== undefined && taxRate !== saved.tax_rate) {
    body.tax_rate = taxRate;
  }
  const before = lineBillingOf(saved);
  if (JSON.stringify(before) !== JSON.stringify(draft.billing)) {
    const wire = lineBillingBody(draft.billing);
    body.billing_model = wire.billing_model ?? null;
    body.billing_interval_months = wire.billing_interval_months ?? null;
    body.interval_count = wire.interval_count ?? null;
  }
  return Object.keys(body).length > 0 ? body : null;
}

/** Where a save stopped: the server's state after the last change it took. */
type SaveOutcome = {
  state: DealCommercial;
  saved: number;
  total: number;
  error?: unknown;
};

type SaveStep = (state: DealCommercial) => Promise<DealCommercial>;

function newLineBody(
  row: DraftLine,
  state: DealCommercial,
  replace: boolean,
  locale: Locale,
) {
  // A product line sends only what the seller changed from the catalogue.
  const renamed = row.productId && row.description.trim() !== row.catalogName;
  return {
    expected_revision: state.commercial_revision,
    replace_estimate: replace || undefined,
    currency: state.currency ?? undefined,
    product_id: row.productId,
    description: !row.productId || renamed ? row.description.trim() : undefined,
    quantity: quantityOf(row.quantity, locale),
    // The price the seller saw is sent; none takes the product's own.
    unit_price_minor:
      row.priceMinor === null && row.productId ? undefined : row.priceMinor,
    discount_pct: typedRate(row.discount_pct, locale),
    tax_rate: typedRate(row.tax_rate, locale),
    ...lineBillingBody(row.billing),
  };
}

function saveSteps(
  dealId: string,
  start: DealCommercial,
  draft: readonly DraftLine[],
  replaceEstimate: boolean,
  t: (key: MessageKey) => string,
  locale: Locale,
): SaveStep[] {
  const kept = new Set(draft.flatMap((row) => (row.id ? [row.id] : [])));
  const steps: SaveStep[] = start.lines
    .filter((row) => !kept.has(row.id))
    .map(
      (line) => async (state) =>
        unwrap(
          await api.DELETE("/deals/{id}/lines/{lineId}", {
            params: {
              path: { id: dealId, lineId: line.id },
              query: { expected_revision: state.commercial_revision },
            },
          }),
          t,
        ),
    );
  for (const row of draft) {
    const saved = start.lines.find((line) => line.id === row.id);
    const body = saved ? changedFields(saved, row, locale) : null;
    if (saved && body) {
      steps.push(async (state) =>
        unwrap(
          await api.PATCH("/deals/{id}/lines/{lineId}", {
            params: { path: { id: dealId, lineId: saved.id } },
            body: { ...body, expected_revision: state.commercial_revision },
          }),
          t,
        ),
      );
    }
    if (!saved) {
      steps.push(async (state) =>
        unwrap(
          await api.POST("/deals/{id}/lines", {
            params: { path: { id: dealId } },
            body: newLineBody(row, state, replaceEstimate, locale),
          }),
          t,
        ),
      );
    }
  }
  return steps;
}

/** Saves one change at a time, each answering the next revision. A failure
 * stops the save, which reports how far it got and the server's state then. */
async function saveDraft(steps: readonly SaveStep[], start: DealCommercial) {
  let state = start;
  for (const [index, step] of steps.entries()) {
    try {
      state = await step(state);
    } catch (error) {
      return { state, saved: index, total: steps.length, error } as SaveOutcome;
    }
  }
  return { state, saved: steps.length, total: steps.length } as SaveOutcome;
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
  const { locale } = useLocale();
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
      <Field
        label={t("deal.lines.quantity")}
        error={typedNumberFault(row.quantity, locale, t)}
      >
        {(control) => (
          <TextInput
            {...control}
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
      <NewLineRates
        value={row}
        testIdPrefix="deal-line"
        onChange={(update) => onChange(update(row))}
      />
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

/** The editor of a deal's products and services. Saving prices the deal from
 * its lines and replaces a typed value, which the dialog names before the save. */
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
  const searchProducts = useProductCandidateSearch();
  const { locale } = useLocale();
  const client = useQueryClient();
  const [draft, setDraft] = useState<DraftLine[]>(() =>
    state.lines.map((line) => draftOf(line, locale)),
  );
  const [custom, setCustom] = useState("");
  const [currency, setCurrency] = useState(state.currency ?? "");
  const replaces = state.mode !== "lines" && typedAmount !== null;
  const [partial, setPartial] = useState<string | null>(null);
  const save = useMutation({
    mutationFn: () => {
      const start = {
        ...state,
        currency: state.currency ?? (currency || null),
      };
      return saveDraft(
        saveSteps(dealId, start, draft, replaces, t, locale),
        start,
      );
    },
    onSuccess: (outcome) => {
      client.setQueryData(dealLinesKey(dealId), outcome.state);
      void client.invalidateQueries({ queryKey: ["deal", dealId] });
      void client.invalidateQueries({ queryKey: ["deal-products", dealId] });
      void client.invalidateQueries({ queryKey: REPORTING_EVALUATION_KEY });
      if (outcome.error === undefined) {
        onClose();
        return;
      }
      // What did save stays saved; the editor starts again from it.
      setDraft(outcome.state.lines.map((line) => draftOf(line, locale)));
      setPartial(
        t("deal.lines.partlySaved", {
          saved: formatNumber(outcome.saved, locale),
          total: formatNumber(outcome.total, locale),
          reason: problemMessageOf(outcome.error, t),
        }),
      );
    },
  });
  const update = (key: string, next: DraftLine) =>
    setDraft((rows) => rows.map((row) => (row.key === key ? next : row)));
  const remove = (key: string) =>
    setDraft((rows) => rows.filter((row) => row.key !== key));
  const linesCurrency = state.currency ?? (currency || null);
  // The rate card price is the line's start only in the lines' own currency.
  const addProduct = (candidate: ProductCandidate) => {
    const catalogPriceMinor =
      candidate.currency === linesCurrency
        ? candidate.unitPriceMinor
        : undefined;
    setDraft((rows) => [
      ...rows,
      {
        key: `new-${rows.length}-${candidate.id}`,
        productId: candidate.id,
        description: candidate.name,
        catalogName: candidate.name,
        catalogPriceMinor,
        quantity: "",
        priceMinor: catalogPriceMinor ?? null,
        billing: EMPTY_LINE_BILLING,
        ...NO_RATES,
      },
    ]);
  };
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
        ...NO_RATES,
      },
    ]);
    setCustom("");
  };
  return (
    <ConfirmModal
      open
      intent="form"
      // Enter in the custom item field would save the lines without that item.
      enterConfirms={false}
      onClose={onClose}
      title={t("deal.lines.editTitle")}
      confirmLabel={t("deal.lines.save")}
      pending={save.isPending}
      // A refused figure would travel as no figure, so the save waits.
      confirmDisabled={draft.some(
        (row) => lineFault(row, locale, t) !== undefined,
      )}
      error={save.error ? problemMessageOf(save.error, t) : partial}
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
          onChange={(next) => {
            setCurrency(next);
            setDraft(withoutCatalogPrices);
          }}
        />
      )}
      {draft.length > 0 && (
        <ul className="deal-lines-edit">
          {draft.map((row) => (
            <DraftLineRow
              key={row.key}
              row={row}
              currency={linesCurrency}
              onChange={(next) => update(row.key, next)}
              onRemove={() => remove(row.key)}
            />
          ))}
        </ul>
      )}
      <RecordPicker
        label={t("deal.lines.addProduct")}
        searchTargets={searchProducts}
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
