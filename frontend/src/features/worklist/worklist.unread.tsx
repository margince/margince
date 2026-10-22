// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Callout } from "../../design-system/callout";
import { useLocale, useT } from "../../i18n";
import { sourceUnavailableText } from "./worklist.copy";
import { planCoverageGap, planCoverageText } from "./worklist.plancoverage";
import type { Worklist, WorklistItem } from "./worklist.queries";
import { represented } from "./worklistunits";

function dayBounded(day: Worklist, loaded: readonly WorklistItem[]): boolean {
  return (
    day.counts.some((count) => count.more_available) ||
    day.reach?.some((source) => source.more_available) === true ||
    represented(loaded).floor
  );
}

export function dayPartlyRead(
  day: Worklist,
  loaded: readonly WorklistItem[] = day.queue,
): boolean {
  return (
    day.sources_unavailable.length > 0 ||
    planCoverageGap(day.plan_coverage) ||
    dayBounded(day, loaded)
  );
}

// What this day did not read, said by the surface about ITSELF, which is what
// Callout is for.
//
// A teammate's unread plan is the same claim as an unread source, so it rides
// in the same notice. A team read that covered every plan says whose it covered
// in a caption: that tells a lead an empty week is empty, not unasked.
// A source read to its bound is named by the line under the filter pills.
export function DayUnread({ day }: Readonly<{ day: Worklist }>) {
  const t = useT();
  const { locale } = useLocale();
  const missing = day.sources_unavailable;
  const coverage = planCoverageText(day.plan_coverage, locale, t);
  if (missing.length === 0 && !planCoverageGap(day.plan_coverage)) {
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
