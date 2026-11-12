// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { trialTone } from "../components/trialcontact";
import { AppLink } from "../design-system/applink";
import { Badge } from "../design-system/atoms";
import {
  type PluralTranslator,
  type Translator,
  usePlural,
  useT,
} from "../i18n";
import { type TrialStanding, useTrialStanding } from "../lib/license-posture";
import { routeHash } from "../lib/router";
import { settingsHref } from "../lib/settingsrouting";

const LICENSE_SETTINGS_HREF = routeHash(settingsHref("seats"));

/**
 * The trial's count under the orb, for a seat that may read the licence. A link
 * rather than a button: the rail's hit area leaves links alone, and the
 * licence card it opens is where the reader writes to the Margince team.
 */
export function TrialPill() {
  const t = useT();
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
          {trialPillText(standing, t, plural)}
        </Badge>
      </AppLink>
    </span>
  );
}

export function trialPillText(
  standing: TrialStanding,
  t: Translator,
  plural: PluralTranslator,
): string {
  if (standing.ended) {
    return t("trial.pill.ended");
  }
  if (standing.daysLeft === 1) {
    return t("trial.pill.lastDay");
  }
  const count = { count: String(standing.daysLeft) };
  return standing.closing
    ? plural("trial.pill.closing", standing.daysLeft, count)
    : plural("trial.pill.days", standing.daysLeft, count);
}
