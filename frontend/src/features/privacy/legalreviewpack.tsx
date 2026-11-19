// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// One review's pack: the deal as it stood at the request, built from the deal
// so nobody briefs legal by hand. A document is listed only when this reader
// can open it, and opens through the file's own gate.

import { LegalReviewStatusBadge } from "../../components/legalreviewstatus";
import { EmptyState } from "../../design-system/atoms";
import { FieldGrid, FieldRow } from "../../design-system/fieldgrid";
import { FileChip } from "../../design-system/filechip";
import { Panel, PanelBody, PanelRow } from "../../design-system/panel";
import { formatActivityWhen } from "../../format/activitywhen";
import { middayInstant } from "../../format/calendarday";
import {
  formatBytes,
  formatDate,
  formatDateTime,
  formatMoneyOrAbsent,
  formatNumber,
  ordinalNumber,
} from "../../format/format";
import {
  type Locale,
  type PluralTranslator,
  type Translator,
  useLocale,
  usePlural,
  useT,
} from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { attachmentTitle } from "../../lib/attachmentlink";
import { CONTRACT_STATUS_LABEL } from "../../lib/contractstatus";
import { labelFor } from "../../lib/labelfor";
import type { LegalReview } from "../../lib/legalreviews.queries";
import { useViewerZone } from "../../lib/viewerzone";

type Packet = LegalReview["packet"];
type PacketDocument = Packet["documents"][number];
type PacketContract = NonNullable<Packet["contracts"]>[number];
type PacketTouch = NonNullable<Packet["touches"]>[number];

const TOUCH_LABEL: Readonly<Record<PacketTouch["kind"], MessageKey>> = {
  call: "timeline.kind.call",
  email: "timeline.kind.email",
  meeting: "timeline.kind.meeting",
  message: "timeline.kind.message",
  note: "timeline.kind.note",
};

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
      {packet.contracts === undefined && packet.deal.stage_name != null && (
        <EmptyState>{t("legalReview.packOlder")}</EmptyState>
      )}
      {(packet.contracts ?? []).length > 0 && <PackContracts packet={packet} />}
      {packet.history.length > 0 && <PackHistory packet={packet} />}
      {(packet.touches ?? []).length > 0 && <PackTouches packet={packet} />}
      {(packet.tasks ?? []).length > 0 && <PackTasks packet={packet} />}
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
  const readerZone = useViewerZone();
  const { locale } = useLocale();
  return <span>{formatDateTime(at, locale, readerZone)}</span>;
}

function PackFacts({ packet }: Readonly<{ packet: Packet }>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = useViewerZone();
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
  const zone = useViewerZone();
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
      {offer.has_pdf && (
        <FieldRow label={t("legalReview.offerPdf")}>
          <a
            href={`/v1/offers/${offer.id}/pdf`}
            target="_blank"
            rel="noreferrer"
          >
            {t("offer.viewPdf")}
          </a>
        </FieldRow>
      )}
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
              <span>{t(labelFor(CATEGORY_LABEL, doc.category))}</span>
              <span>{t(labelFor(STATE_LABEL, doc.doc_state))}</span>
            </span>
            <span className="meta-row-entry">
              {doc.available ? (
                <FileChip
                  href={`/v1/attachments/${doc.attachment_id}`}
                  filename={attachmentTitle(doc)}
                  size={
                    doc.byte_size == null
                      ? undefined
                      : formatBytes(doc.byte_size, locale)
                  }
                />
              ) : (
                <FileChip
                  filename={t(labelFor(CATEGORY_LABEL, doc.category))}
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
  const zone = useViewerZone();
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

function PackContracts({ packet }: Readonly<{ packet: Packet }>) {
  const t = useT();
  return (
    <Panel title={t("legalReview.packContracts")}>
      {(packet.contracts ?? []).map((contract) => (
        <PanelRow key={contract.id}>
          <ContractFields contract={contract} />
        </PanelRow>
      ))}
    </Panel>
  );
}

function ContractFields({ contract }: Readonly<{ contract: PacketContract }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const zone = useViewerZone();
  const day = (value: string) => calendarDayLabel(value, locale, zone);
  const days = contract.notice_period_days;
  return (
    <FieldGrid>
      <FieldRow label={contract.title}>
        {t(labelFor(CONTRACT_STATUS_LABEL, contract.status))}
      </FieldRow>
      {contract.contract_number && (
        <FieldRow label={t("contracts.form.number")}>
          {contract.contract_number}
        </FieldRow>
      )}
      {contract.starts_on && (
        <FieldRow label={t("contracts.form.startsOn")}>
          {day(contract.starts_on)}
        </FieldRow>
      )}
      {contract.ends_on && (
        <FieldRow label={t("contracts.form.endsOn")}>
          {day(contract.ends_on)}
        </FieldRow>
      )}
      {contract.renewal_on && (
        <FieldRow label={t("contracts.form.renewalOn")}>
          {day(contract.renewal_on)}
          {contract.auto_renew != null && (
            <div className="t-caption">
              {t(
                contract.auto_renew
                  ? "legalReview.contractAutoRenews"
                  : "legalReview.contractNoAutoRenew",
              )}
            </div>
          )}
        </FieldRow>
      )}
      {days != null && (
        <FieldRow label={t("legalReview.contractNotice")}>
          {plural("legalReview.contractNoticeDays", days, {
            count: formatNumber(days, locale),
          })}
        </FieldRow>
      )}
      {contract.cancellation_notice_on && (
        <FieldRow label={t("legalReview.contractCancelBy")}>
          {day(contract.cancellation_notice_on)}
        </FieldRow>
      )}
    </FieldGrid>
  );
}

function PackTouches({ packet }: Readonly<{ packet: Packet }>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = useViewerZone();
  return (
    <Panel title={t("legalReview.packTouches")}>
      {(packet.touches ?? []).map((touch) => (
        <PanelRow key={touch.activity_id} className="meta-row">
          <span className="meta-row-line t-caption">
            {t("legalReview.touchLine", {
              kind: t(labelFor(TOUCH_LABEL, touch.kind)),
              date: formatActivityWhen(
                touch.occurred_at,
                touch.occurred_precision,
                locale,
                zone,
              ),
            })}
          </span>
          {touch.subject && (
            <span className="meta-row-entry t-body">{touch.subject}</span>
          )}
        </PanelRow>
      ))}
    </Panel>
  );
}

function PackTasks({ packet }: Readonly<{ packet: Packet }>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = useViewerZone();
  return (
    <Panel title={t("legalReview.packTasks")}>
      {(packet.tasks ?? []).map((task) => (
        <PanelRow key={task.activity_id} className="meta-row">
          {task.due_at && (
            <span className="meta-row-line t-caption">
              {t("legalReview.taskDue", {
                date: formatDateTime(task.due_at, locale, zone),
              })}
            </span>
          )}
          {task.subject && (
            <span className="meta-row-entry t-body">{task.subject}</span>
          )}
        </PanelRow>
      ))}
    </Panel>
  );
}
