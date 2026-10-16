// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// One review's pack: the deal as it stood at the request, built from the deal
// so nobody briefs legal by hand. A document is listed only when this reader
// can open it, and opens through the file's own gate.

import { EmptyState } from "../../design-system/atoms";
import { FieldGrid, FieldRow } from "../../design-system/fieldgrid";
import { FileChip } from "../../design-system/filechip";
import { Panel, PanelBody, PanelRow } from "../../design-system/panel";
import { middayInstant } from "../../format/calendarday";
import {
  formatBytes,
  formatDate,
  formatDateTime,
  formatMoneyOrAbsent,
  formatNumber,
  ordinalNumber,
} from "../../format/format";
import { viewerZone } from "../../format/timezone";
import {
  type Locale,
  type PluralTranslator,
  type Translator,
  useLocale,
  usePlural,
  useT,
} from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import type { LegalReview } from "./legalreviews.queries";
import { LegalReviewStatusBadge } from "./legalreviewstatus";

type Packet = LegalReview["packet"];
type PacketDocument = Packet["documents"][number];

/** A calendar day (`2026-11-01`) in the reader's words. */
export function calendarDayLabel(
  day: string,
  locale: Locale,
  zone: string,
): string {
  return formatDate(middayInstant(day, zone), locale, zone);
}

const CATEGORY_LABEL: Readonly<Record<PacketDocument["category"], MessageKey>> =
  {
    contract: "legalReview.docCategory.contract",
    offer: "legalReview.docCategory.offer",
    legal: "legalReview.docCategory.legal",
  };

const STATE_LABEL: Readonly<Record<PacketDocument["doc_state"], MessageKey>> = {
  draft: "legalReview.docState.draft",
  current: "legalReview.docState.current",
  final: "legalReview.docState.final",
};

export function LegalReviewPack({ review }: Readonly<{ review: LegalReview }>) {
  const t = useT();
  const packet = review.packet;
  return (
    <>
      <PackFacts packet={packet} />
      <PackOffer packet={packet} />
      <PackDocuments packet={packet} />
      {packet.history.length > 0 && <PackHistory packet={packet} />}
      {review.earlier.length > 0 && (
        <Panel title={t("legalReview.earlier")}>
          {review.earlier.map((earlier) => (
            <PanelRow key={earlier.id} className="meta-row">
              <span className="meta-row-line t-caption">
                <LegalReviewStatusBadge status={earlier.status} />
                <EarlierWhen at={earlier.decided_at ?? earlier.created_at} />
              </span>
              {earlier.comments && (
                <span className="meta-row-entry t-body">
                  {t("legalReview.commentsShown", {
                    comments: earlier.comments,
                  })}
                </span>
              )}
            </PanelRow>
          ))}
        </Panel>
      )}
    </>
  );
}

function EarlierWhen({ at }: Readonly<{ at: string }>) {
  const { locale } = useLocale();
  return <span>{formatDateTime(at, locale, viewerZone())}</span>;
}

function PackFacts({ packet }: Readonly<{ packet: Packet }>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = viewerZone();
  const deal = packet.deal;
  return (
    <Panel title={t("legalReview.packTitle")}>
      <PanelBody>
        <FieldGrid>
          <FieldRow label={t("legalReview.packCompany")}>
            {deal.company_name ?? "—"}
          </FieldRow>
          <FieldRow label={t("legalReview.packValue")}>
            {formatMoneyOrAbsent(deal.amount_minor, deal.currency, locale)}
          </FieldRow>
          <FieldRow label={t("legalReview.packStage")}>
            {deal.stage_name ?? "—"}
          </FieldRow>
          <FieldRow label={t("legalReview.packClose")}>
            {deal.expected_close_date
              ? calendarDayLabel(deal.expected_close_date, locale, zone)
              : "—"}
          </FieldRow>
          <FieldRow label={t("legalReview.packOwner")}>
            {deal.owner_name ?? "—"}
          </FieldRow>
        </FieldGrid>
      </PanelBody>
    </Panel>
  );
}

type PacketOffer = NonNullable<Packet["offer"]>;
type PacketOfferLine = PacketOffer["lines"][number];

const CADENCE: Readonly<Record<number, MessageKey>> = {
  1: "product.billingMonthly",
  3: "product.billingQuarterly",
  6: "product.billingHalfYearly",
  12: "product.billingYearly",
};

// The approval binds to every offer the buyer holds, so each one is shown.
function PackOffer({ packet }: Readonly<{ packet: Packet }>) {
  const t = useT();
  const binding = packet.binding ?? [];
  return (
    <>
      <Panel title={t("legalReview.packOffer")}>
        <PanelBody>
          {packet.offer ? (
            <OfferFields offer={packet.offer} />
          ) : (
            <EmptyState>{t("legalReview.packNoOffer")}</EmptyState>
          )}
        </PanelBody>
      </Panel>
      {binding.length > 0 && (
        <Panel title={t("legalReview.packBinding")}>
          {binding.map((offer) => (
            <PanelRow key={offer.id}>
              <OfferFields offer={offer} />
            </PanelRow>
          ))}
        </Panel>
      )}
    </>
  );
}

function OfferFields({ offer }: Readonly<{ offer: PacketOffer }>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = viewerZone();
  return (
    <FieldGrid>
      <FieldRow label={t("legalReview.offerNumber")}>
        {t("legalReview.offerRevision", {
          number: offer.offer_number,
          revision: ordinalNumber(offer.revision),
        })}
      </FieldRow>
      <FieldRow label={t("legalReview.offerTotal")}>
        {formatMoneyOrAbsent(offer.gross_minor, offer.currency, locale)}
      </FieldRow>
      {offer.valid_until && (
        <FieldRow label={t("legalReview.offerValidUntil")}>
          {calendarDayLabel(offer.valid_until, locale, zone)}
        </FieldRow>
      )}
      {offer.intro_text && (
        <FieldRow label={t("offer.introText")}>
          <span className="t-body">{offer.intro_text}</span>
        </FieldRow>
      )}
      {offer.lines.map((line, index) => (
        <FieldRow
          key={line.position}
          label={t("legalReview.offerLine", {
            number: ordinalNumber(index + 1),
          })}
        >
          {t("legalReview.offerLineText", {
            quantity: line.quantity,
            description: line.description,
            price: formatMoneyOrAbsent(
              line.unit_price_minor,
              offer.currency,
              locale,
            ),
          })}
          <LineTerms line={line} />
        </FieldRow>
      ))}
      {offer.terms_text && (
        <FieldRow label={t("legalReview.offerTerms")}>
          <span className="t-body">{offer.terms_text}</span>
        </FieldRow>
      )}
    </FieldGrid>
  );
}

// How long the buyer pays, and whether the line is only a proposal yet.
function LineTerms({ line }: Readonly<{ line: PacketOfferLine }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const billing = lineBilling(line, t, plural, locale);
  return (
    <>
      {billing && <div className="t-caption">{billing}</div>}
      {line.staged && (
        <div className="t-caption">{t("legalReview.offerLineStaged")}</div>
      )}
    </>
  );
}

function lineBilling(
  line: PacketOfferLine,
  t: Translator,
  plural: PluralTranslator,
  locale: Locale,
): string | null {
  if (line.billing_model === "one_time") return t("product.billingOneTime");
  if (line.billing_model !== "recurring") return null;
  const months = line.billing_interval_months;
  const cadence = t(
    (months != null && CADENCE[months]) || "product.billingRecurring",
  );
  if (line.interval_count == null) return cadence;
  return plural("legalReview.offerLineRecurring", line.interval_count, {
    cadence,
    count: formatNumber(line.interval_count, locale),
  });
}

function PackDocuments({ packet }: Readonly<{ packet: Packet }>) {
  const t = useT();
  const { locale } = useLocale();
  return (
    <Panel title={t("legalReview.packDocuments")}>
      {packet.documents.length === 0 ? (
        <PanelBody>
          <EmptyState>{t("legalReview.packNoDocuments")}</EmptyState>
        </PanelBody>
      ) : (
        packet.documents.map((doc) => (
          <PanelRow key={doc.attachment_id} className="meta-row">
            <span className="meta-row-line t-caption">
              <span>{t(CATEGORY_LABEL[doc.category])}</span>
              <span>{t(STATE_LABEL[doc.doc_state])}</span>
            </span>
            <span className="meta-row-entry">
              {doc.available ? (
                <FileChip
                  href={`/v1/attachments/${doc.attachment_id}`}
                  filename={doc.title ?? doc.filename ?? ""}
                  size={
                    doc.byte_size == null
                      ? undefined
                      : formatBytes(doc.byte_size, locale)
                  }
                />
              ) : (
                <FileChip
                  filename={t(CATEGORY_LABEL[doc.category])}
                  withheld={t("legalReview.docUnavailable")}
                />
              )}
            </span>
          </PanelRow>
        ))
      )}
    </Panel>
  );
}

function PackHistory({ packet }: Readonly<{ packet: Packet }>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = viewerZone();
  return (
    <Panel title={t("legalReview.packHistory")}>
      {packet.history.map((stage) => (
        <PanelRow key={`${stage.stage_name}-${stage.entered_at}`}>
          <span className="t-body">
            {t("legalReview.enteredStage", {
              stage: stage.stage_name,
              date: formatDateTime(stage.entered_at, locale, zone),
            })}
          </span>
        </PanelRow>
      ))}
    </Panel>
  );
}
