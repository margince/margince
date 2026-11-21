import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../../api/client";
import { Button, Field, TextInput } from "../../design-system/atoms";
import { DataTable } from "../../design-system/datatable";
import { ErrorLine } from "../../design-system/errorline";
import { MoneyInput } from "../../design-system/moneyinput";
import { Panel, PanelBody } from "../../design-system/panel";
import {
  RecordPicker,
  type RecordPickerCandidate,
} from "../../design-system/recordpicker";
import { useFieldFaults } from "../../design-system/savepage";
import { typedNumberFault, typedNumberOrNaN } from "../../format/decimalinput";
import { formatMoney } from "../../format/format";
import { type Locale, type Translator, useLocale, useT } from "../../i18n";
import { labelFor } from "../../lib/labelfor";
import {
  type FieldLabels,
  moneyLimit,
  percentLimit,
  problemCodeOf,
  problemMessageOf,
  refusalSentences,
  unwrap,
} from "../../lib/problem";
import {
  EMPTY_LINE_BILLING,
  type LineBilling,
  lineBillingBody,
  OfferLineBillingFields,
} from "./offerlinebilling";
import { discountFault, NewLineRates, taxRateFault } from "./offerlinerates";
import type {
  Offer,
  OfferLineItem,
  OfferLineItemInput,
  UpdateOfferLineItemRequest,
} from "./offers";
import {
  DescriptionCell,
  NumericCell,
  UnitCell,
  UnitPriceCell,
} from "./offers.cells";
import { searchProductCandidates } from "./products";
import { isUnnamedOtherUnit, storedUnitOf, UnitPicker } from "./productunit";
import { TAX_CATEGORY_LABEL } from "./taxcategory";

const EMPTY_NEW_LINE = {
  description: "",
  unit: { choice: "", other: "" },
  quantity: "",
  unit_price_minor: 0,
  discount_pct: "",
  tax_rate: "",
};

type NewLineState = typeof EMPTY_NEW_LINE;

// An ungrounded (price_grounded === false) line has no price input anywhere
// below: grounding a price is a server/AI concern, not something a human
// free-types over. It stays ungrounded until a regenerate
// re-grounds it, or the human removes it and re-adds it with a price.
function UnpricedCaption({ label }: Readonly<{ label: string }>) {
  return <span className="t-caption">{label}</span>;
}

// The line-item editor. Server-driven totals (P11) govern every mutation:
// add, edit and remove all return the full updated Offer, with line_items and
// net, tax and gross recomputed. This component only passes that response to
// queryClient.setQueryData(["offer", ...]). It never derives a money figure.
export function OfferLineEditor({ offer }: Readonly<{ offer: Offer }>) {
  const t = useT();
  const { locale } = useLocale();
  const queryClient = useQueryClient();
  const [newLine, setNewLine] = useState<NewLineState>(EMPTY_NEW_LINE);
  const [billing, setBilling] = useState<LineBilling>(EMPTY_LINE_BILLING);
  const [priceTouched, setPriceTouched] = useState(false);
  const [priceRefused, setPriceRefused] = useState(false);
  // Set by an Add that found "Other" with no unit typed; any unit change clears it.
  const [unitRefused, setUnitRefused] = useState(false);
  const [product, setProduct] = useState<RecordPickerCandidate | null>(null);
  const [faults, faultsRoot] = useFieldFaults<HTMLDivElement>();
  const labels = lineLabels(offer.currency, locale, t);
  const edit = (field: string, change: Partial<NewLineState>) => {
    setNewLine((prev) => ({ ...prev, ...change }));
    faults.edited(field);
  };

  const applyOffer = (updated: Offer) => {
    queryClient.setQueryData(["offer", offer.id], updated);
  };

  // One scope per offer: the line writes carry no version, so the server
  // keeps whichever request lands last. Running them in send order keeps the
  // newest draft the last write and each answer newer than the one before.
  const lineWrites = { id: `offer-lines:${offer.id}` };
  const addMutation = useMutation({
    scope: lineWrites,
    mutationFn: async (input: OfferLineItemInput) => {
      return unwrap(
        await api.POST("/offers/{id}/line-items", {
          params: { path: { id: offer.id } },
          body: input,
        }),
      );
    },
    onSuccess: (data) => {
      applyOffer(data);
      setNewLine(EMPTY_NEW_LINE);
      setPriceTouched(false);
      setProduct(null);
    },
  });

  // The contract declares no If-Match on updateOfferLineItem. Unlike the
  // header-level Offer PATCH, a line item's own `version` is not a
  // precondition the API accepts, and the generated client refuses one.
  const updateMutation = useMutation({
    scope: lineWrites,
    mutationFn: async (variables: {
      lineItemId: string;
      patch: UpdateOfferLineItemRequest;
    }) => {
      return unwrap(
        await api.PATCH("/offers/{id}/line-items/{lineItemId}", {
          params: { path: { id: offer.id, lineItemId: variables.lineItemId } },
          body: variables.patch,
        }),
      );
    },
    onSuccess: (data) => {
      applyOffer(data);
    },
  });

  const removeMutation = useMutation({
    scope: lineWrites,
    mutationFn: async (lineItemId: string) => {
      return unwrap(
        await api.DELETE("/offers/{id}/line-items/{lineItemId}", {
          params: { path: { id: offer.id, lineItemId } },
        }),
      );
    },
    onSuccess: (data) => {
      applyOffer(data);
    },
  });

  // Only the last write speaks: an earlier refusal is about a different try.
  const writes = [addMutation, updateMutation, removeMutation];
  const attempt = (write: (typeof writes)[number]) => {
    for (const other of writes) if (other !== write) other.reset();
  };
  const errorMessage = addMutation.isError
    ? refusalSentences(addMutation.error, t, labels).rest
    : updateMutation.isError
      ? problemCodeOf(updateMutation.error) === "conflict"
        ? t("offer.positionTaken")
        : problemMessageOf(updateMutation.error, t, undefined, labels)
      : removeMutation.isError
        ? problemMessageOf(removeMutation.error, t)
        : null;

  const saveLine =
    (lineItemId: string) => (patch: UpdateOfferLineItemRequest) => {
      attempt(updateMutation);
      return updateMutation.mutateAsync({ lineItemId, patch });
    };

  const columns = [
    {
      key: "position",
      header: t("offer.position"),
      render: (line: OfferLineItem) => (
        <NumericCell
          value={line.position}
          width={60}
          testId={`line-position-${line.id}`}
          onSave={(num) => saveLine(line.id)({ position: num })}
        />
      ),
    },
    {
      key: "description",
      header: t("offer.description"),
      render: (line: OfferLineItem) => (
        <DescriptionCell line={line} onSave={saveLine(line.id)} />
      ),
    },
    {
      key: "unit",
      header: t("offer.unit"),
      render: (line: OfferLineItem) => (
        <UnitCell line={line} onSave={saveLine(line.id)} />
      ),
    },
    {
      key: "quantity",
      header: t("offer.quantity"),
      render: (line: OfferLineItem) => (
        <NumericCell
          value={line.quantity}
          width={90}
          testId={`line-quantity-${line.id}`}
          onSave={(num) => saveLine(line.id)({ quantity: num })}
        />
      ),
    },
    {
      key: "unitPrice",
      header: t("offer.unitPrice"),
      render: (line: OfferLineItem) =>
        line.price_grounded === false ? (
          <UnpricedCaption label={t("offer.unpriced")} />
        ) : (
          <UnitPriceCell
            line={line}
            currency={offer.currency}
            onSave={saveLine(line.id)}
          />
        ),
    },
    {
      key: "discountPct",
      header: t("offer.discountPct"),
      render: (line: OfferLineItem) => (
        <NumericCell
          value={line.discount_pct}
          width={90}
          testId={`line-discount-${line.id}`}
          check={(text) => discountFault(text, locale, t)}
          onSave={(num) => saveLine(line.id)({ discount_pct: num })}
        />
      ),
    },
    {
      key: "taxRate",
      header: t("offer.taxRate"),
      render: (line: OfferLineItem) => (
        <>
          <NumericCell
            value={line.tax_rate}
            width={90}
            testId={`line-tax-rate-${line.id}`}
            check={(text) => taxRateFault(text, locale, t)}
            onSave={(num) => saveLine(line.id)({ tax_rate: num })}
          />
          {line.tax_category && (
            <span
              className="t-caption"
              data-testid={`line-tax-category-${line.id}`}
            >
              {t(labelFor(TAX_CATEGORY_LABEL, line.tax_category))}
            </span>
          )}
        </>
      ),
    },
    {
      key: "lineTotal",
      header: t("offer.lineTotal"),
      render: (line: OfferLineItem) =>
        line.price_grounded === false ? (
          <UnpricedCaption label={t("offer.unpriced")} />
        ) : (
          <span className="t-num">
            {formatMoney(line.line_total_minor, offer.currency, locale)}
          </span>
        ),
    },
    {
      key: "remove",
      header: "",
      render: (line: OfferLineItem) => (
        <Button
          data-testid={`remove-line-${line.id}`}
          disabled={removeMutation.isPending}
          onClick={() => {
            attempt(removeMutation);
            removeMutation.mutate(line.id);
          }}
        >
          {t("offer.removeLine")}
        </Button>
      ),
    },
  ];

  return (
    <Panel title={t("offer.lines")}>
      <PanelBody>
        <DataTable
          label={t("offer.lines")}
          columns={columns}
          rows={offer.line_items}
          rowKey={(line) => line.id}
        />
      </PanelBody>
      {/* Its own body, so the seam divides the lines that exist from the one
          being written. */}
      <PanelBody>
        <span className="t-label">{t("offer.addLine")}</span>
        <div ref={faultsRoot} className="offers-new-line">
          <Field
            label={t("offer.description")}
            error={faults.of("description")}
          >
            {(control) => (
              <TextInput
                {...control}
                data-testid="new-line-description"
                className="offers-line-description"
                value={newLine.description}
                onChange={(event) =>
                  edit("description", { description: event.target.value })
                }
              />
            )}
          </Field>
          <Field
            label={t("offer.unit")}
            error={
              unitRefused ? t("product.unitOtherRequired") : faults.of("unit")
            }
          >
            {(control) => (
              <UnitPicker
                id={control.id}
                testId="new-line-unit"
                blankLabel={t("offer.unitFromProduct")}
                value={newLine.unit}
                onChange={(unit) => {
                  setUnitRefused(false);
                  edit("unit", { unit });
                }}
              />
            )}
          </Field>
          <Field
            label={t("offer.quantity")}
            error={
              typedNumberFault(newLine.quantity, locale, t) ??
              faults.of("quantity")
            }
          >
            {(control) => (
              <input
                {...control}
                data-testid="new-line-quantity"
                inputMode="decimal"
                className="input offers-line-quantity"
                value={newLine.quantity}
                onChange={(event) =>
                  edit("quantity", { quantity: event.target.value })
                }
              />
            )}
          </Field>
          <Field
            label={t("offer.unitPrice")}
            error={faults.of("unit_price_minor")}
          >
            {(control) => (
              <MoneyInput
                {...control}
                data-testid="new-line-unit-price"
                currency={offer.currency}
                onRefusedChange={setPriceRefused}
                valueMinor={newLine.unit_price_minor}
                onChangeMinor={(minor) => {
                  setPriceTouched(true);
                  edit("unit_price_minor", { unit_price_minor: minor });
                }}
              />
            )}
          </Field>
          <NewLineRates
            value={newLine}
            onChange={setNewLine}
            refused={{
              discount_pct: faults.of("discount_pct"),
              tax_rate: faults.of("tax_rate"),
            }}
            onEdited={faults.edited}
          />
          <OfferLineBillingFields value={billing} onChange={setBilling} />
        </div>
        <div className="offers-new-line-product">
          <div className="field offers-product-field">
            <span className="t-label">{t("offer.pickProduct")}</span>
            <RecordPicker
              label={t("offer.pickProduct")}
              searchTargets={searchProductCandidates}
              selected={product}
              onPick={setProduct}
            />
            {product && (
              <p className="t-caption">
                {t("offer.pickProductConfirm", { name: product.name })}
              </p>
            )}
          </div>
          <Button
            variant="primary"
            data-testid="add-line"
            disabled={addMutation.isPending}
            className="offers-add-line"
            onClick={() => {
              if (isUnnamedOtherUnit(newLine.unit)) {
                setUnitRefused(true);
                return;
              }
              // Each refused figure already names why under its field.
              if (
                priceRefused ||
                faults.show(figureFaults(newLine, locale, t))
              ) {
                return;
              }
              attempt(addMutation);
              addMutation.mutate(
                {
                  product_id: product?.id ?? undefined,
                  description: newLine.description || undefined,
                  unit:
                    storedUnitOf(newLine.unit.choice, newLine.unit.other) ||
                    undefined,
                  ...newLineFigures(newLine, locale),
                  unit_price_minor: priceTouched
                    ? newLine.unit_price_minor
                    : undefined,
                  ...lineBillingBody(billing),
                },
                {
                  onError: (error) =>
                    faults.show(refusalSentences(error, t, labels).byField),
                },
              );
            }}
          >
            {t("offer.addLine")}
          </Button>
        </div>
        {errorMessage && <ErrorLine>{errorMessage}</ErrorLine>}
      </PanelBody>
    </Panel>
  );
}

function figureFaults(
  line: NewLineState,
  locale: Locale,
  t: Translator,
): ReadonlyMap<string, string> {
  const faults = [
    ["quantity", typedNumberFault(line.quantity, locale, t)],
    ["discount_pct", discountFault(line.discount_pct, locale, t)],
    ["tax_rate", taxRateFault(line.tax_rate, locale, t)],
  ] as const;
  return new Map(
    faults.flatMap(([field, fault]) => (fault ? [[field, fault]] : [])),
  );
}

// A line's fields as a refusal names them, with bounds in the reader's unit.
function lineLabels(
  currency: string,
  locale: Locale,
  t: Translator,
): FieldLabels {
  const percent = percentLimit(locale);
  return {
    description: t("offer.description"),
    unit: t("offer.unit"),
    quantity: t("offer.quantity"),
    position: t("offer.position"),
    unit_price_minor: {
      label: t("offer.unitPrice"),
      limit: moneyLimit(currency, locale),
    },
    discount_pct: { label: t("offer.discountPct"), limit: percent },
    tax_rate: { label: t("offer.taxRate"), limit: percent },
  };
}

// An empty discount or tax rate is left out, so the stored default applies.
function newLineFigures(line: NewLineState, locale: Locale) {
  const optional = (text: string) =>
    text === "" ? undefined : typedNumberOrNaN(text, locale);
  return {
    quantity:
      line.quantity.trim() === "" ? 0 : typedNumberOrNaN(line.quantity, locale),
    discount_pct: optional(line.discount_pct),
    tax_rate: optional(line.tax_rate),
  };
}
