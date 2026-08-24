// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { StatCard } from "../design-system/atoms";
import { StatStrip } from "../design-system/statstrip";
import { useT } from "../i18n";

// The day's readings, on one plate. The record pages read theirs the same way,
// which is the point: a strip is read ACROSS as one comparison, and four
// free-standing tiles would be read one at a time.
//
// Every slot is a COUNT, never money. The pipeline is worth three different
// currencies at once, and no honest single figure exists for it — adding native
// minor units across currencies produces a number that is not money. The
// per-currency figures live in the rail's Position panel, one line each, where
// they can be stated without being summed.
//
// A reading that could not be taken is absent from the strip rather than shown
// as a zero: `StatStrip` counts the slots the caller actually drew, so a missing
// reading leaves no empty cell behind to be misread as a failed one.

export type HomeReadings = Readonly<{
  /** Decisions waiting, and how many stop waiting today. Null when unread. */
  decisions: { pending: number; expiringToday: number } | null;
  /** Open deals and the currencies they are priced in. Null when unread. */
  open: { deals: number; currencies: number } | null;
  /** The ranked queue: how many, and the top composite. Null when no run. */
  ranked: { count: number; topPct: number | null } | null;
  /** Open deals that have gone quiet. Null when unread. */
  quiet: number | null;
}>;

export function HomeReadingsStrip({
  decisions,
  open,
  ranked,
  quiet,
}: HomeReadings) {
  const t = useT();
  return (
    <StatStrip className="home-readings" testId="home-readings">
      {decisions && (
        <StatCard
          numeric
          label={t("home.readings.decisions")}
          value={String(decisions.pending)}
          // The tile is bad news only when something runs out TODAY. A queue
          // with work in it is the normal state of a morning; a queue with a
          // deadline in it is the one that changes what a reader does first.
          tone={decisions.expiringToday > 0 ? "warn" : undefined}
          dot={decisions.expiringToday > 0}
          detail={
            decisions.expiringToday > 0
              ? t(
                  decisions.expiringToday === 1
                    ? "home.readings.expiring.one"
                    : "home.readings.expiring.other",
                  { count: decisions.expiringToday },
                )
              : t("home.readings.expiringNone")
          }
        />
      )}
      {open && (
        <StatCard
          numeric
          label={t("home.readings.openDeals")}
          value={String(open.deals)}
          detail={t(
            open.currencies === 1
              ? "home.readings.currencies.one"
              : "home.readings.currencies.other",
            { count: open.currencies },
          )}
        />
      )}
      {ranked && (
        <StatCard
          numeric
          label={t("home.readings.ranked")}
          value={String(ranked.count)}
          detail={
            ranked.topPct === null
              ? t("home.readings.noRun")
              : t("home.readings.topScore", { pct: ranked.topPct })
          }
        />
      )}
      {quiet !== null && (
        <StatCard
          numeric
          label={t("home.readings.quiet")}
          value={String(quiet)}
          tone={quiet > 0 ? "warn" : "good"}
          dot={quiet > 0}
          detail={quiet === 0 ? t("home.readings.quietNone") : undefined}
        />
      )}
    </StatStrip>
  );
}
