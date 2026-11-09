// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { ArrowRight } from "lucide-react";
import { AppLink } from "../../design-system/applink";
import { Badge } from "../../design-system/atoms";
import { PanelBody, PanelRow } from "../../design-system/panel";
import { formatDate, formatMoney, formatNumber } from "../../format/format";
import { useLocale, usePlural, useT } from "../../i18n";
import { useCompanyZone } from "../../lib/companyzone";
import { routeHash } from "../../lib/router";
import type { WeeklyReview } from "./brief.queries";

/** How many deals moved stage, at the end of the section's label. */
export function MovedCount({ count }: Readonly<{ count: number }>) {
  const plural = usePlural();
  const { locale } = useLocale();
  return (
    <Badge dot>
      {plural("brief.report.movedCount", count, {
        count: formatNumber(count, locale),
      })}
    </Badge>
  );
}

type Deal = WeeklyReview["deals"][number];

/** The deal's name, linked. The label was frozen with the review, so a deal
 *  renamed since still reads as it did that week. */
function DealName({ deal }: Readonly<{ deal: Deal }>) {
  return (
    <AppLink
      className="brief-weekly-deal-name link-button"
      href={routeHash({ screen: "deals", id: deal.deal_id })}
    >
      {deal.label}
    </AppLink>
  );
}

/** What the week won and lost, at the end of the section's label. */
export function ClosedTotal({ review }: Readonly<{ review: WeeklyReview }>) {
  const t = useT();
  const { locale } = useLocale();
  const pipeline = review.pipeline;
  if (!pipeline) return null;
  return (
    // Green only for a week with no loss; a mixed result reads neutral.
    <Badge tone={pipeline.lost_minor === 0 ? "success" : undefined} dot>
      {t("brief.report.closedTotal", {
        won: formatMoney(pipeline.won_minor, pipeline.currency, locale),
        lost: formatMoney(pipeline.lost_minor, pipeline.currency, locale),
      })}
    </Badge>
  );
}

/** The deals the week won and lost: the split of their worth, then a line
 *  each with its outcome and what it closed at. */
export function ClosedDeals({
  review,
  deals,
}: Readonly<{ review: WeeklyReview; deals: readonly Deal[] }>) {
  const t = useT();
  const { locale } = useLocale();
  const recordZone = useCompanyZone();
  const pipeline = review.pipeline;
  const total = pipeline ? pipeline.won_minor + pipeline.lost_minor : 0;
  return (
    <>
      {/* The share won against lost, which the total beside the label states
          in words, so a screen reader is not read it twice. */}
      {pipeline && total > 0 && (
        <PanelBody>
          <div className="brief-report-split" aria-hidden="true">
            <span
              className="brief-report-split-won"
              style={{ flexGrow: pipeline.won_minor }}
            />
            <span
              className="brief-report-split-lost"
              style={{ flexGrow: pipeline.lost_minor }}
            />
          </div>
        </PanelBody>
      )}
      {deals.map((deal) => (
        <PanelRow key={`${deal.deal_id}-${deal.occurred_at}`}>
          <div className="brief-report-deal">
            <span className="brief-report-deal-what">
              <DealName deal={deal} />
              <time className="t-caption" dateTime={deal.occurred_at}>
                {formatDate(deal.occurred_at, locale, recordZone)}
              </time>
            </span>
            <span className="brief-report-keep">
              <Badge tone={deal.outcome === "won" ? "success" : "warning"} dot>
                {deal.outcome === "won"
                  ? t("brief.weekly.outcome.won")
                  : t("brief.weekly.outcome.lost")}
              </Badge>
            </span>
            <span
              className="brief-report-deal-amount t-num"
              data-outcome={deal.outcome}
            >
              {deal.amount_minor_at_close != null && deal.currency_at_close
                ? formatMoney(
                    deal.amount_minor_at_close,
                    deal.currency_at_close,
                    locale,
                  )
                : "—"}
            </span>
          </div>
        </PanelRow>
      ))}
    </>
  );
}

/** The deals that changed stage without closing, and the stage each reached. */
export function MovedDeals({ deals }: Readonly<{ deals: readonly Deal[] }>) {
  const t = useT();
  const { locale } = useLocale();
  const recordZone = useCompanyZone();
  return (
    <>
      {deals.map((deal) => (
        <PanelRow key={`${deal.deal_id}-${deal.occurred_at}`}>
          <div className="brief-report-deal">
            <span className="brief-report-deal-what">
              <DealName deal={deal} />
              {deal.to_stage_label && (
                <span className="brief-report-stage">
                  {t("brief.weekly.outcome.moved")}
                  <ArrowRight aria-hidden="true" size={14} />
                  <Badge>{deal.to_stage_label}</Badge>
                </span>
              )}
            </span>
            <time className="t-caption" dateTime={deal.occurred_at}>
              {formatDate(deal.occurred_at, locale, recordZone)}
            </time>
          </div>
        </PanelRow>
      ))}
    </>
  );
}
