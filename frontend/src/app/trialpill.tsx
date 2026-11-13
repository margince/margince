// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useState } from "react";
import { TrialContactDialog } from "../components/trialcontact";
import { NoticePill, type NoticePillTone } from "../design-system/noticepill";
import { formatNumber } from "../format/format";
import {
  type Locale,
  type PluralTranslator,
  type Translator,
  useLocale,
  usePlural,
  useT,
} from "../i18n";
import {
  type TrialStanding,
  useLicenseEntitlement,
  useTrialStanding,
} from "../lib/license-posture";
import { TrialRing } from "./trialring";

/**
 * The trial's count under the orb, for a seat that may read the licence. It is
 * the way to the Margince team for all of the trial: the banner only joins it
 * in the last week.
 */
export function TrialPill() {
  const t = useT();
  const { locale } = useLocale();
  const plural = usePlural();
  const standing = useTrialStanding();
  const entitlement = useLicenseEntitlement(standing !== undefined);
  const [writing, setWriting] = useState(false);
  if (!standing) {
    return null;
  }
  return (
    <span className="artrial">
      <NoticePill
        tone={pillTone(standing)}
        opensDialog
        onClick={() => setWriting(true)}
      >
        {trialPillText(standing, t, plural, locale)}
      </NoticePill>
      {writing && (
        <TrialContactDialog
          standing={standing}
          seatsUsed={entitlement.data?.seats_used}
          onClose={() => setWriting(false)}
        />
      )}
    </span>
  );
}

function pillTone(standing: TrialStanding): NoticePillTone {
  if (standing.ended) {
    return "danger";
  }
  return standing.closing ? "warning" : "success";
}

/** The trial's ring around the orb, for the same seats the pill is for. */
export function TrialOrbRing() {
  const standing = useTrialStanding();
  return standing && !standing.ended ? <TrialRing standing={standing} /> : null;
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
