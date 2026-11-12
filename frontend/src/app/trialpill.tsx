// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { trialTone } from "../components/trialcontact";
import { AppLink } from "../design-system/applink";
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
import { routeHash } from "../lib/router";
import { settingsHref } from "../lib/settingsrouting";

const LICENSE_SETTINGS_HREF = routeHash(settingsHref("seats"));

/**
 * The trial's count under the orb, for a seat that may read the licence. It is
 * a link, because the rail's hit area leaves links alone. The licence card it
 * opens is where the reader writes to the Margince team.
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
      <AppLink href={LICENSE_SETTINGS_HREF}>
        <Badge tone={tone === "info" ? "success" : tone}>
          {trialPillText(standing, t, plural, locale)}
        </Badge>
      </AppLink>
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
