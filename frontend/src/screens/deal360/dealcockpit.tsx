// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The deal's cockpit: one band under the head's facts strip, the stage
// ladder and nothing beside it. It used to carry the four-tile DealStrip and,
// after that, three readings drawn next to the ladder — but the money and the
// close date are cells of the head's facts strip now (dealheaderfacts.tsx),
// the momentum sentence is the deal's own brief reading, and the Deal Room is
// its own tab on the record. A cockpit repeating all four said the same facts
// twice on one page.
//
// The ladder still stands on its own row, above the tabs rather than a
// scroll into one of them: a reader working thirty deals before a forecast
// call reads "where does this deal stand" once, and the same way on every
// tab.

import type { components } from "../../api/schema";
import { useRecordZone } from "../../app/recordzone";
import { formatDate, formatMoneyOrAbsent } from "../../format/format";
import { type Locale, useT } from "../../i18n";
import { DealStageLadder } from "../deals.stepper";
import "./dealcockpit.css";

type Deal = components["schemas"]["Deal"];
type Stage = components["schemas"]["Stage"];

// Where the money reading's own door leads, from callers elsewhere on the
// page: the offers card sits on the overview tab, one scroll down.
export const DEAL_OFFERS_ANCHOR = "deal-offers";

/**
 * DealCockpit draws the deal's stage ladder as the page's one full-width row.
 */
export function DealCockpit({
  deal,
  stages,
  advancing,
  advanceRefused,
  refusedReasonId,
  onAdvance,
}: Readonly<{
  deal: Deal;
  stages: readonly Stage[];
  advancing: boolean;
  advanceRefused: boolean;
  refusedReasonId?: string;
  onAdvance: (toStage: Stage) => void;
}>) {
  const t = useT();
  return (
    <section aria-label={t("deal.strip.title")}>
      <div className="deal-cockpit">
        {/* Where the deal STANDS, on its own row and at full width: the
            ladder is a sentence the eye reads left to right, and squeezed
            beside a reading it wrapped its last rungs onto a second line
            while the reading beside it kept its own. */}
        <div className="deal-cockpit-ladder">
          <DealStageLadder
            deal={deal}
            stages={stages}
            advancing={advancing}
            advanceRefused={advanceRefused}
            refusedReasonId={refusedReasonId}
            onAdvance={onAdvance}
          />
        </div>
      </div>
    </section>
  );
}

// The amount, converted into the installation's base currency at the frozen
// rate — kept as the exported component every story and test already reaches
// by name, drawn as the money fact's second line: the native amount and its
// converted twin are one fact, not two readings.
export function FxLine({
  amountMinor,
  baseCurrency,
  fxRateToBase,
  fxRateDate,
  locale,
}: Readonly<{
  amountMinor: number | null;
  // The installation's own base currency, from its settings. Not a constant:
  // an installation whose base is not the euro was reading a euro sign over a
  // figure converted into something else, which is the one error a converted
  // figure must not make. Null while the settings read is in flight or
  // refused — an unnamed base is not a euro base.
  baseCurrency: string | null;
  fxRateToBase: string;
  fxRateDate: string | null;
  locale: Locale;
}>) {
  const t = useT();
  const recordZone = useRecordZone();
  // A deal carrying a rate but no amount converts to nothing, not to zero.
  const baseMinor =
    amountMinor == null ? null : Math.round(amountMinor * Number(fxRateToBase));
  return (
    <p className="t-caption">
      {t("deal.fxBase", {
        value: formatMoneyOrAbsent(baseMinor, baseCurrency, locale),
        rate: fxRateToBase,
        date: fxRateDate ? formatDate(fxRateDate, locale, recordZone) : "—",
      })}
    </p>
  );
}
