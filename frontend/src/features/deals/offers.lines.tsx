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
import { formatMoney } from "../../format/format";
import { useLocale, useT } from "../../i18n";
import { problemMessageOf, unwrap } from "../../lib/problem";
import {
  EMPTY_LINE_BILLING,
  type LineBilling,
  lineBillingBody,
  OfferLineBillingFields,
} from "./offerlinebilling";
import { NewLineRates } from "./offerlinerates";
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
import { storedUnitOf, UnitPicker } from "./productunit";

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
  const [product, setProduct] = useState<RecordPickerCandidate | null>(null);

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
      setPriceTouched(false);
      setProduct(null);
    },
  });

  // The contract declares no If-Match on updateOfferLineItem. Unlike the
  // header-level Offer PATCH, a line item's own `version` is not a
  // precondition the API accepts, and the generated client refuses one.
  const updateMutation = useMutation({
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

  const activeError =
    addMutation.error ?? updateMutation.error ?? removeMutation.error;
  const errorMessage = activeError ? problemMessageOf(activeError, t) : null;

  const saveLine =
    (lineItemId: string) => (patch: UpdateOfferLineItemRequest) => {
      updateMutation.mutate({ lineItemId, patch });
    };

  const columns = [
    {
      key: "position",
      header: t("offer.position"),
      render: (line: OfferLineItem) => (
        <NumericCell
          value={line.position}
          step="1"
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
          step="0.001"
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
          step="0.01"
          width={90}
          testId={`line-discount-${line.id}`}
          onSave={(num) => saveLine(line.id)({ discount_pct: num })}
        />
      ),
    },
    {
      key: "taxRate",
      header: t("offer.taxRate"),
      render: (line: OfferLineItem) => (
        <NumericCell
          value={line.tax_rate}
          step="0.01"
          width={90}
          testId={`line-tax-rate-${line.id}`}
          onSave={(num) => saveLine(line.id)({ tax_rate: num })}
        />
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
          onClick={() => removeMutation.mutate(line.id)}
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
          rowKey={(line) => `${line.id}:${line.version ?? 0}`}
        />
      </PanelBody>
      {/* Its own body, so the seam divides the lines that exist from the one
          being written. */}
      <PanelBody>
        <span className="t-label">{t("offer.addLine")}</span>
        <div className="offers-new-line">
          <Field label={t("offer.description")}>
            {(control) => (
              <TextInput
                {...control}
                data-testid="new-line-description"
                className="offers-line-description"
                value={newLine.description}
                onChange={(event) =>
                  setNewLine((prev) => ({
                    ...prev,
                    description: event.target.value,
                  }))
                }
              />
            )}
          </Field>
          <Field label={t("offer.unit")}>
            {(control) => (
              <UnitPicker
                id={control.id}
                testId="new-line-unit"
                blankLabel={t("offer.unitFromProduct")}
                value={newLine.unit}
                onChange={(unit) => setNewLine((prev) => ({ ...prev, unit }))}
              />
            )}
          </Field>
          <Field label={t("offer.quantity")}>
            {(control) => (
              <input
                {...control}
                data-testid="new-line-quantity"
                type="number"
                step="0.001"
                className="input offers-line-quantity"
                value={newLine.quantity}
                onChange={(event) =>
                  setNewLine((prev) => ({
                    ...prev,
                    quantity: event.target.value,
                  }))
                }
              />
            )}
          </Field>
          <Field label={t("offer.unitPrice")}>
            {(control) => (
              <MoneyInput
                {...control}
                data-testid="new-line-unit-price"
                currency={offer.currency}
                valueMinor={newLine.unit_price_minor}
                onChangeMinor={(minor) => {
                  setPriceTouched(true);
                  setNewLine((prev) => ({ ...prev, unit_price_minor: minor }));
                }}
              />
            )}
          </Field>
          <NewLineRates value={newLine} onChange={setNewLine} />
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
            onClick={() =>
              addMutation.mutate({
                product_id: product?.id ?? undefined,
                description: newLine.description || undefined,
                unit:
                  storedUnitOf(newLine.unit.choice, newLine.unit.other) ||
                  undefined,
                quantity: Number(newLine.quantity),
                unit_price_minor: priceTouched
                  ? newLine.unit_price_minor
                  : undefined,
                discount_pct:
                  newLine.discount_pct === ""
                    ? undefined
                    : Number(newLine.discount_pct),
                tax_rate:
                  newLine.tax_rate === ""
                    ? undefined
                    : Number(newLine.tax_rate),
                ...lineBillingBody(billing),
              })
            }
          >
            {t("offer.addLine")}
          </Button>
        </div>
        {errorMessage && <ErrorLine>{errorMessage}</ErrorLine>}
      </PanelBody>
    </Panel>
  );
}
