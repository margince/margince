// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { api } from "../../api/client";
import { EntityRef } from "../../components/entityref";
import { DataTable } from "../../design-system/datatable";
import { FieldGrid, FieldRow } from "../../design-system/fieldgrid";
import { Panel, PanelBody } from "../../design-system/panel";
import { lineBillingPhrase } from "../../format/billingphrase";
import { formatDate, formatMoney, formatNumber } from "../../format/format";
import { unitLabel } from "../../format/productunit";
import { useLocale, usePlural, useT } from "../../i18n";
import { useHealthScope } from "../../lib/healthscope";
import { useLegalEntities } from "../../lib/legalentities.queries";
import { unwrap } from "../../lib/problem";
import { useViewerZone } from "../../lib/viewerzone";
import type { Offer, OfferLineItem } from "./offers";
import { isOpenDraft } from "./offers.actions";
import { useOfferTemplates } from "./offers.header";

// The buyer's name as the offer froze it on send; a draft has no snapshot yet,
// so EntityRef looks the company up.
function snapshotName(offer: Offer): string | undefined {
  const name = offer.buyer_snapshot?.display_name;
  return typeof name === "string" ? name : undefined;
}

/**
 * The template the offer names. A draft's page reads the template list for its
 * header form anyway, so a draft looks there first. Any other offer, or a
 * template the list leaves out, is read by its id.
 */
function useOfferTemplate(offer: Offer) {
  const id = offer.template_id;
  const draft = isOpenDraft(offer);
  const scope = useHealthScope();
  const list = useOfferTemplates(draft && id != null);
  const listed = list.data?.find((each) => each.id === id);
  const byId = useQuery({
    queryKey: ["offer-template", id, scope ?? ""],
    enabled:
      id != null &&
      scope !== undefined &&
      (!draft || (list.isSuccess && listed === undefined)),
    queryFn: async () =>
      unwrap(
        await api.GET("/offer-templates/{id}", {
          params: { path: { id: id ?? "" } },
        }),
      ),
  });
  return listed ?? byId.data;
}

/** What the offer says beyond its lines, readable in every status. */
export function OfferHeaderFields({ offer }: Readonly<{ offer: Offer }>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = useViewerZone();
  const entities = useLegalEntities(offer.legal_entity_id != null).data;
  const template = useOfferTemplate(offer);
  const issuer = entities?.find(
    (entity) => entity.id === offer.legal_entity_id,
  );
  return (
    <FieldGrid>
      <FieldRow label={t("offer.deal")}>
        <EntityRef kind="deal" id={offer.deal_id} />
      </FieldRow>
      {offer.buyer_company_id && (
        <FieldRow label={t("offer.buyerCompany")}>
          <EntityRef
            kind="company"
            id={offer.buyer_company_id}
            name={snapshotName(offer)}
          />
        </FieldRow>
      )}
      {issuer && (
        <FieldRow label={t("offer.legalEntity")}>
          {issuer.display_name || t("legalEntities.defaultUnnamed")}
        </FieldRow>
      )}
      <FieldRow label={t("deal.offerRevision")}>
        {formatNumber(offer.revision, locale)}
      </FieldRow>
      {offer.valid_until && (
        <FieldRow label={t("offer.validUntil")}>
          {formatDate(offer.valid_until, locale, zone)}
        </FieldRow>
      )}
      {offer.language && (
        <FieldRow label={t("offer.language")}>{offer.language}</FieldRow>
      )}
      {template && (
        <FieldRow label={t("offer.template")}>{template.name}</FieldRow>
      )}
      {offer.intro_text && (
        <FieldRow label={t("offer.introText")}>
          <span className="t-body offers-text">{offer.intro_text}</span>
        </FieldRow>
      )}
      {offer.terms_text && (
        <FieldRow label={t("offer.termsText")}>
          <span className="t-body offers-text">{offer.terms_text}</span>
        </FieldRow>
      )}
    </FieldGrid>
  );
}

/** The lines as the buyer reads them, where the editor is not shown. */
export function OfferLinesTable({ offer }: Readonly<{ offer: Offer }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const money = (minor: number, line: OfferLineItem) =>
    line.price_grounded === false
      ? t("offer.unpriced")
      : formatMoney(minor, offer.currency, locale);
  const columns = [
    {
      key: "position",
      header: t("offer.position"),
      render: (line: OfferLineItem) => formatNumber(line.position, locale),
    },
    {
      key: "description",
      header: t("offer.description"),
      render: (line: OfferLineItem) => line.description,
    },
    {
      key: "unit",
      header: t("offer.unit"),
      render: (line: OfferLineItem) => unitLabel(line.unit, t),
    },
    {
      key: "quantity",
      header: t("offer.quantity"),
      render: (line: OfferLineItem) => formatNumber(line.quantity, locale),
    },
    {
      key: "unitPrice",
      header: t("offer.unitPrice"),
      render: (line: OfferLineItem) => money(line.unit_price_minor, line),
    },
    {
      key: "discountPct",
      header: t("offer.discountPct"),
      render: (line: OfferLineItem) => formatNumber(line.discount_pct, locale),
    },
    {
      key: "taxRate",
      header: t("offer.taxRate"),
      render: (line: OfferLineItem) => formatNumber(line.tax_rate, locale),
    },
    {
      key: "billing",
      header: t("product.billingModel"),
      render: (line: OfferLineItem) =>
        lineBillingPhrase(line, t, plural, locale) ?? "—",
    },
    {
      key: "lineTotal",
      header: t("offer.lineTotal"),
      render: (line: OfferLineItem) => (
        <span className="t-num">{money(line.line_total_minor, line)}</span>
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
    </Panel>
  );
}
