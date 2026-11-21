import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../../api/client";
import { Button, Field, TextInput } from "../../design-system/atoms";
import { ErrorLine } from "../../design-system/errorline";
import { MoneyInput } from "../../design-system/moneyinput";
import { Panel, PanelBody } from "../../design-system/panel";
import {
  RecordPicker,
  type RecordPickerCandidate,
} from "../../design-system/recordpicker";
import { useFieldFaults } from "../../design-system/savepage";
import { typedNumberFault, typedNumberOrNaN } from "../../format/decimalinput";
import { type Locale, type Translator, useLocale, useT } from "../../i18n";
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
  OfferLineItemInput,
  UpdateOfferLineItemRequest,
} from "./offers";
import { EditableLineTable, useLineRefusals } from "./offers.linetable";
import { useProductCandidateSearch } from "./products";
import { isUnnamedOtherUnit, storedUnitOf, UnitPicker } from "./productunit";

const EMPTY_NEW_LINE = {
  description: "",
  unit: { choice: "", other: "" },
  quantity: "",
  unit_price_minor: 0,
  discount_pct: "",
  tax_rate: "",
};

type NewLineState = typeof EMPTY_NEW_LINE;

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
  // Remounts the price box after an add, so a free line's "0.00" goes too.
  const [added, setAdded] = useState(0);
  // Set by an Add that found "Other" with no unit typed; any unit change clears it.
  const [unitRefused, setUnitRefused] = useState(false);
  const [product, setProduct] = useState<RecordPickerCandidate | null>(null);
  const [faults, faultsRoot] = useFieldFaults<HTMLDivElement>();
  const labels = lineLabels(offer.currency, locale, t);
  const searchProducts = useProductCandidateSearch();
  const refusals = useLineRefusals();
  const edit = (field: string, change: Partial<NewLineState>) => {
    setNewLine((prev) => ({ ...prev, ...change }));
    faults.edited(field);
  };

  // Whether the Add line form cannot be sent yet. Each refusal shows under its
  // own field, and a refused price box already says why.
  const refusesAdd = () => {
    if (isUnnamedOtherUnit(newLine.unit)) {
      setUnitRefused(true);
      return true;
    }
    const found = new Map(figureFaults(newLine, locale, t));
    if (!priceRefused && !priceTouched && !product) {
      found.set("unit_price_minor", t("offer.unitPriceRequired"));
    }
    return faults.show(found) || priceRefused;
  };

  const applyOffer = (updated: Offer) => {
    queryClient.setQueryData(["offer", offer.id], updated);
  };

  const addMutation = useMutation({
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
      setBilling(EMPTY_LINE_BILLING);
      setPriceTouched(false);
      setAdded((count) => count + 1);
      setProduct(null);
    },
  });

  // The contract declares no If-Match on updateOfferLineItem. Unlike the
  // header-level Offer PATCH, a line item's own `version` is not a
  // precondition the API accepts, and the generated client refuses one.
  const updateMutation = useMutation({
    // One at a time, so a line's later edit is the one the server keeps.
    scope: { id: `offer-lines:${offer.id}` },
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
    onSuccess: (data, { lineItemId }) => {
      applyOffer(data);
      refusals.settle(lineItemId);
    },
    onError: (error, { lineItemId, patch }) => {
      refusals.refuse(
        lineItemId,
        Object.keys(patch),
        problemCodeOf(error) === "conflict"
          ? t("offer.positionTaken")
          : problemMessageOf(error, t, undefined, labels),
      );
    },
  });

  const removeMutation = useMutation({
    mutationFn: async (lineItemId: string) => {
      return unwrap(
        await api.DELETE("/offers/{id}/line-items/{lineItemId}", {
          params: { path: { id: offer.id, lineItemId } },
        }),
      );
    },
    onSuccess: (data, lineItemId) => {
      applyOffer(data);
      refusals.settle(lineItemId);
    },
    onError: (error, lineItemId) => {
      refusals.refuse(lineItemId, [], problemMessageOf(error, t));
    },
  });

  // A refused field shows under that field; the rest of the refusal shows here.
  const addRefusal = addMutation.isError
    ? refusalSentences(addMutation.error, t, labels).rest
    : null;

  return (
    <Panel title={t("offer.lines")}>
      <PanelBody>
        <EditableLineTable
          offer={offer}
          refusals={refusals}
          onSave={(lineItemId, patch) =>
            // A refusal is shown in its row by the mutation itself.
            updateMutation
              .mutateAsync({ lineItemId, patch })
              .catch(() => undefined)
          }
          onRemove={(lineItemId) => removeMutation.mutate(lineItemId)}
          removing={removeMutation.isPending}
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
                key={added}
                data-testid="new-line-unit-price"
                currency={offer.currency}
                onRefusedChange={setPriceRefused}
                valueMinor={newLine.unit_price_minor}
                blankWhenZero={!priceTouched}
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
              searchTargets={searchProducts}
              selected={product}
              onPick={(picked) => {
                faults.edited("unit_price_minor");
                setProduct(picked);
              }}
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
            pending={addMutation.isPending}
            className="offers-add-line"
            onClick={() => {
              if (refusesAdd()) {
                return;
              }
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
        {addRefusal && <ErrorLine>{addRefusal}</ErrorLine>}
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
