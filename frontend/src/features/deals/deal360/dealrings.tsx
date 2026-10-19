// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../../api/schema";
import { RingsLegend, type RingTone } from "../../../design-system/rings";
import { calendarDaysBetween, formatNumber } from "../../../format/format";
import { useNow } from "../../../format/now";
import { useLocale, usePlural, useT } from "../../../i18n";
import { ghostCount } from "./dealcommittee";

type DealStatusCard = components["schemas"]["DealStatusCard"];
type DealCoverage = components["schemas"]["DealCoverage"];

/** One ring of a deal's 360: a share and its tone. */
export type DealRing = {
  key: "stage" | "committee";
  value: number;
  max: number;
  tone: RingTone;
  // The count the caption says: the days in stage, which the arc stops
  // short of once they pass the usual time.
  reading: number;
};

/**
 * The two shares a deal is read from: how far into the usual time in its
 * stage it is, and how many of its buying roles are named. A ring is drawn
 * only from a reading there is: no usual time below the won-deal history the
 * median needs, and no committee while the coverage read is withheld.
 */
export function dealRings(
  card: DealStatusCard,
  coverage: DealCoverage | undefined,
  now: number,
): DealRing[] {
  const rings: DealRing[] = [];
  // Whole days, the unit the caption counts in; the median is a fraction.
  const usual =
    card.stage_usual_days == null ? null : Math.round(card.stage_usual_days);
  if (usual !== null && usual >= 1) {
    const days = Math.max(
      calendarDaysBetween(new Date(card.stage_entered_at), new Date(now)),
      0,
    );
    rings.push({
      key: "stage",
      value: Math.min(days, usual),
      max: usual,
      tone: days > usual ? "warning" : "calm",
      reading: days,
    });
  }
  if (coverage) {
    const named = coverage.stakeholders.length;
    const missing = ghostCount(coverage.risks);
    if (named + missing > 0) {
      rings.push({
        key: "committee",
        value: named,
        max: named + missing,
        tone: missing > 0 ? "warning" : "calm",
        reading: named,
      });
    }
  }
  return rings;
}

/** DealRings draws those shares beside the deal's verdict. */
export function DealRings({
  card,
  coverage,
}: Readonly<{ card: DealStatusCard; coverage: DealCoverage | undefined }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  // Pinned at mount: the stage ring counts days.
  const now = useNow(0);
  const n = (value: number) => formatNumber(value, locale);
  const rings = dealRings(card, coverage, now).map((ring) => ({
    ...ring,
    label: t(`deal.rings.${ring.key}`),
    caption:
      ring.key === "stage"
        ? t("deal.rings.stage.caption", {
            days: plural("co.spine.days", ring.reading, {
              count: n(ring.reading),
            }),
            usual: plural("co.spine.days", ring.max, { count: n(ring.max) }),
          })
        : plural("deal.rings.committee.caption", ring.max, {
            part: n(ring.reading),
            count: n(ring.max),
          }),
  }));
  return (
    <RingsLegend
      rings={rings}
      label={rings.map((ring) => `${ring.label}: ${ring.caption}`).join(", ")}
    />
  );
}
