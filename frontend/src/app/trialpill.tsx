// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { trialTone } from "../components/trialcontact";
import { Badge } from "../design-system/atoms";
import { formatNumber } from "../format/format";
import {
  type Locale,
  type PluralTranslator,
  type Translator,
  useLocale,
  usePlural,
  useT,
} from "../i18n";
import { type TrialStanding, useTrialStanding } from "../lib/license-posture";

/**
 * The trial's count under the orb, for a seat that may read the licence. A
 * label only: the banner on every page holds the way to the Margince team.
 */
export function TrialPill() {
  const t = useT();
  const { locale } = useLocale();
  const plural = usePlural();
  const standing = useTrialStanding();
  if (!standing) {
    return null;
  }
  const tone = trialTone(standing);
  return (
    <span className="artrial">
      <Badge tone={tone === "info" ? "success" : tone}>
        {trialPillText(standing, t, plural, locale)}
      </Badge>
    </span>
  );
}

export function trialPillText(
  standing: TrialStanding,
  t: Translator,
  plural: PluralTranslator,
  locale: Locale,
): string {
  if (standing.ended) {
    return t("trial.pill.ended");
  }
  if (standing.daysLeft === 1) {
    return t("trial.pill.lastDay");
  }
  const count = { count: formatNumber(standing.daysLeft, locale) };
  return standing.closing
    ? plural("trial.pill.closing", standing.daysLeft, count)
    : plural("trial.pill.days", standing.daysLeft, count);
}
