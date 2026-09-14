// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import { Panel, PanelRow } from "../design-system/panel";
import { formatMoney } from "../format/format";
import { useLocale, useT } from "../i18n";

type Offer = components["schemas"]["Offer"];

/**
 * An offer's money totals: net, tax, gross, and — where it has a recurring
 * component — its annual and committed values.
 *
 * Its own file because offers.tsx sits at its length cap, and this is a
 * cohesive block: every figure a reader compares against the line list.
 */
export function OfferTotalsPanel({ offer }: Readonly<{ offer: Offer }>) {
  const t = useT();
  const { locale } = useLocale();
  return (
    <Panel title={t("offer.totals")}>
      <PanelRow>
        <span className="t-label">{t("offer.net")}</span>
        <div className="t-mono">
          {formatMoney(offer.net_minor, offer.currency, locale)}
        </div>
      </PanelRow>
      <PanelRow>
        <span className="t-label">{t("offer.tax")}</span>
        <div className="t-mono">
          {formatMoney(offer.tax_minor, offer.currency, locale)}
        </div>
      </PanelRow>
      <PanelRow>
        <span className="t-label">{t("offer.gross")}</span>
        <div className="t-mono">
          {formatMoney(offer.gross_minor, offer.currency, locale)}
        </div>
      </PanelRow>
      <OfferRecurringTotals offer={offer} />
    </Panel>
  );
}

/**
 * The two recurring figures beside an offer's net, tax and gross.
 *
 * Rendered only where the offer has a recurring component. An offer of purely
 * one-off work, and an offer whose lines nobody has classified, both show
 * nothing here rather than two zeros: a zero annual value asserts that the
 * work does not repeat, and an unclassified offer makes no such claim.
 *
 * The committed total is shown beside the annual one because the two are
 * different numbers that a reader will otherwise conflate. A quarterly line at
 * 3,000 committed for eight quarters is 24,000 of committed value and 12,000 a
 * year, and an offer page showing only one of them invites the wrong figure
 * into a forecast.
 *
 */
function OfferRecurringTotals({ offer }: Readonly<{ offer: Offer }>) {
  const t = useT();
  const { locale } = useLocale();
  // Both figures are derived server-side and always present on the wire, so an
  // absent one is a response from a server that predates them rather than an
  // offer with no recurring part.
  const arr = offer.arr_minor;
  if (arr == null || arr === 0) {
    return null;
  }
  return (
    <>
      <PanelRow>
        <span className="t-label">{t("offer.arr")}</span>
        <div className="t-mono">{formatMoney(arr, offer.currency, locale)}</div>
      </PanelRow>
      {offer.net_tcv_minor != null && (
        <PanelRow>
          <span className="t-label">{t("offer.committedNet")}</span>
          <div className="t-mono">
            {formatMoney(offer.net_tcv_minor, offer.currency, locale)}
          </div>
        </PanelRow>
      )}
    </>
  );
}
