// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { formatNumber } from "../format/format";
import { useLocale, useT } from "../i18n";
import type { WeeklyReview } from "./brief.queries";

import "./brief.weekly.css";

// The five figures that are not the week's outcomes, drawn under the strip.
//
// Its own file rather than a block inside the panel: the outcome strip is one
// comparison read across, and this is a lookup read one pair at a time — two
// different claims, and the sheet beside them names them separately too.

/**
 * The week's workings, under the strip.
 *
 * Five readings that answer "how did the week go" rather than "what did the week
 * produce" — how much of the queue was worked, how proposals were decided, how
 * many deals moved without closing. They were slots six to ten of a ten-slot
 * strip, where they made the row fold into two ranks and cost the outcomes their
 * one-comparison reading.
 *
 * A definition list, not more cards: these are looked up one at a time by
 * somebody who already read the strip, which is the opposite of the strip's
 * read-across claim.
 */
export function WeeklyWorkings({
  counts,
}: Readonly<{ counts: WeeklyReview["counts"] }>) {
  const t = useT();
  const { locale } = useLocale();
  const n = (value: number) => formatNumber(value, locale);
  return (
    <dl className="brief-weekly-workings">
      <Working
        label={t("brief.weekly.tasksDelivered")}
        value={t("brief.weekly.ofDue", {
          done: n(counts.tasks_done),
          due: n(counts.tasks_due),
        })}
      />
      <Working
        label={t("brief.weekly.dealsMoved")}
        value={n(counts.deals_moved)}
      />
      <Working
        label={t("brief.weekly.dealsLost")}
        value={n(counts.deals_lost)}
      />
      <Working
        label={t("brief.weekly.decided")}
        value={t("brief.weekly.acceptedRejected", {
          accepted: n(counts.proposals_accepted),
          rejected: n(counts.proposals_rejected),
        })}
      />
      <Working
        label={t("brief.weekly.queueWorked")}
        value={t("brief.weekly.actedDismissed", {
          acted: n(counts.brief_items_acted),
          dismissed: n(counts.brief_items_dismissed),
        })}
      />
    </dl>
  );
}

function Working({ label, value }: Readonly<{ label: string; value: string }>) {
  return (
    // No type class on either half: the list is ONE meta-sized row, and the
    // size is the list's (brief.weekly.css) rather than a rung each pair
    // repeats — a label at the meta rung beside a value at the body rung made
    // the same row two sizes.
    <div className="brief-weekly-working">
      <dt>{label}</dt>
      <dd>{value}</dd>
    </div>
  );
}
