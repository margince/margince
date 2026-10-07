// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Callout } from "../design-system/callout";
import { useLocale, useT } from "../i18n";
import { sourceUnavailableText } from "./worklist.copy";
import { planCoverageGap, planCoverageText } from "./worklist.plancoverage";
import type { Worklist } from "./worklist.queries";

/** True when something that would have filled the day was never read. */
export function dayPartlyRead(day: Worklist): boolean {
  return (
    day.sources_unavailable.length > 0 || planCoverageGap(day.plan_coverage)
  );
}

// What this day did not read, said by the surface about ITSELF, which is what
// Callout is for.
//
// A teammate's unread plan is the same claim as an unread source, so it rides
// in the same notice. A team read that covered every plan has nothing to warn
// about and still says whose it covered, quietly: that is what tells a lead an
// empty week is empty rather than unasked.
export function DayUnread({ day }: Readonly<{ day: Worklist }>) {
  const t = useT();
  const { locale } = useLocale();
  const missing = day.sources_unavailable;
  const coverage = planCoverageText(day.plan_coverage, locale, t);
  if (!dayPartlyRead(day)) {
    return coverage ? <p className="t-caption">{coverage}</p> : null;
  }
  return (
    <Callout tone="warning" kind="standing" title={t("worklist.partialTitle")}>
      {missing.length > 0 && (
        <p>
          {t("worklist.partial", {
            sources: missing
              .map((source) => sourceUnavailableText(source, t))
              .join(", "),
          })}
        </p>
      )}
      {coverage && <p>{coverage}</p>}
    </Callout>
  );
}
