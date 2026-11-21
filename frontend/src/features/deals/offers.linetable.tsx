// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Trash2 } from "lucide-react";
import { useState } from "react";
import { DataTable } from "../../design-system/datatable";
import { ErrorLine } from "../../design-system/errorline";
import { IconAction } from "../../design-system/iconaction";
import { formatMoney, ordinalNumber } from "../../format/format";
import { useLocale, useT } from "../../i18n";
import { labelFor } from "../../lib/labelfor";
import { BillingCell } from "./offerlinebilling";
import { discountFault, taxRateFault } from "./offerlinerates";
import type {
  Offer,
  OfferLineItem,
  UpdateOfferLineItemRequest,
} from "./offers";
import {
  DescriptionCell,
  NumericCell,
  UnitCell,
  UnitPriceCell,
} from "./offers.cells";
import { TAX_CATEGORY_LABEL } from "./taxcategory";

export type LineRefusals = ReturnType<typeof useLineRefusals>;

/**
 * The server's last refusal per line, shown in that line's row until the line
 * next saves. Each cell keeps the refused value itself, so a refusal never
 * mounts a cell again.
 */
export function useLineRefusals() {
  const [errors, setErrors] = useState<ReadonlyMap<string, string>>(new Map());
  return {
    refuse: (lineItemId: string, reason: string) => {
      setErrors((prev) => new Map(prev).set(lineItemId, reason));
    },
    settle: (lineItemId: string) => {
      setErrors((prev) => {
        if (!prev.has(lineItemId)) {
          return prev;
        }
        const next = new Map(prev);
        next.delete(lineItemId);
        return next;
      });
    },
    errorOf: (lineItemId: string) => errors.get(lineItemId),
  };
}

// An ungrounded (price_grounded === false) line has no price input anywhere
// below: grounding a price is a server/AI concern, not something a human
// free-types over. It stays ungrounded until a regenerate
// re-grounds it, or the human removes it and re-adds it with a price.
function UnpricedCaption({ label }: Readonly<{ label: string }>) {
  return <span className="t-caption">{label}</span>;
}

// The draft's lines, each cell saving itself on blur through `onSave`.
export function EditableLineTable({
  offer,
  refusals,
  onSave,
  onRemove,
  removing,
}: Readonly<{
  offer: Offer;
  refusals: LineRefusals;
  // Settles with the saved offer, and rejects once the save is refused.
  onSave: (
    lineItemId: string,
    patch: UpdateOfferLineItemRequest,
  ) => Promise<Offer>;
  onRemove: (lineItemId: string) => void;
  removing: boolean;
}>) {
  const t = useT();
  const { locale } = useLocale();
  // Every row holds the same controls, so each is named for its line as well.
  const cellLabel = (column: string, line: OfferLineItem) =>
    t("offer.lineCell", { column, position: ordinalNumber(line.position) });
  const numeric = (
    line: OfferLineItem,
    field: "position" | "quantity" | "discount_pct" | "tax_rate",
    shape: Readonly<{ width: number; testId: string }>,
    header: string,
    check?: (text: string) => string | undefined,
  ) => (
    <NumericCell
      value={line[field]}
      width={shape.width}
      testId={`${shape.testId}-${line.id}`}
      label={cellLabel(header, line)}
      check={check}
      onSave={(num) => {
        const patch: UpdateOfferLineItemRequest = {};
        patch[field] = num;
        return onSave(line.id, patch);
      }}
    />
  );
  const save = (line: OfferLineItem) => (patch: UpdateOfferLineItemRequest) =>
    onSave(line.id, patch);

  const columns = [
    {
      key: "position",
      header: t("offer.position"),
      render: (line: OfferLineItem) =>
        numeric(
          line,
          "position",
          { width: 60, testId: "line-position" },
          t("offer.position"),
        ),
    },
    {
      key: "description",
      header: t("offer.description"),
      render: (line: OfferLineItem) => (
        <DescriptionCell
          line={line}
          label={cellLabel(t("offer.description"), line)}
          onSave={save(line)}
        />
      ),
    },
    {
      key: "unit",
      header: t("offer.unit"),
      render: (line: OfferLineItem) => (
        <UnitCell
          line={line}
          label={(column) => cellLabel(column, line)}
          onSave={save(line)}
        />
      ),
    },
    {
      key: "quantity",
      header: t("offer.quantity"),
      render: (line: OfferLineItem) =>
        numeric(
          line,
          "quantity",
          { width: 90, testId: "line-quantity" },
          t("offer.quantity"),
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
            label={cellLabel(t("offer.unitPrice"), line)}
            onSave={save(line)}
          />
        ),
    },
    {
      key: "discountPct",
      header: t("offer.discountPct"),
      render: (line: OfferLineItem) =>
        numeric(
          line,
          "discount_pct",
          { width: 90, testId: "line-discount" },
          t("offer.discountPct"),
          (text) => discountFault(text, locale, t),
        ),
    },
    {
      key: "taxRate",
      header: t("offer.taxRate"),
      render: (line: OfferLineItem) => (
        <>
          {numeric(
            line,
            "tax_rate",
            { width: 90, testId: "line-tax-rate" },
            t("offer.taxRate"),
            (text) => taxRateFault(text, locale, t),
          )}
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
      key: "billing",
      header: t("product.billingModel"),
      render: (line: OfferLineItem) => (
        <BillingCell
          line={line}
          label={(column) => cellLabel(column, line)}
          onSave={save(line)}
        />
      ),
    },
    {
      key: "remove",
      header: "",
      render: (line: OfferLineItem) => (
        <>
          <IconAction
            icon={<Trash2 />}
            testId={`remove-line-${line.id}`}
            label={t("offer.removeLineNamed", {
              position: ordinalNumber(line.position),
            })}
            disabled={removing}
            onClick={() => onRemove(line.id)}
          />
          {refusals.errorOf(line.id) && (
            <ErrorLine inline>{refusals.errorOf(line.id)}</ErrorLine>
          )}
        </>
      ),
    },
  ];

  return (
    <DataTable
      label={t("offer.lines")}
      columns={columns}
      rows={offer.line_items}
      rowKey={(line) => line.id}
      rowTestId={(line) => `line-row-${line.id}`}
    />
  );
}
