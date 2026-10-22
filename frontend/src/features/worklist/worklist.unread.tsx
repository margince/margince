// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Callout } from "../../design-system/callout";
import { useLocale, useT } from "../../i18n";
import { sourceUnavailableText } from "./worklist.copy";
import { planCoverageGap, planCoverageText } from "./worklist.plancoverage";
import type { Worklist, WorklistItem } from "./worklist.queries";
import { sourceName } from "./worklist.sources";
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
// in the same notice. A team read that covered every plan has nothing to warn
// about and still says whose it covered, quietly: that is what tells a lead an
// empty week is empty rather than unasked.
export function DayUnread({
  day,
  loaded = day.queue,
}: Readonly<{ day: Worklist; loaded?: readonly WorklistItem[] }>) {
  const t = useT();
  const { locale } = useLocale();
  const missing = day.sources_unavailable;
  const limited = day.reach?.filter((source) => source.more_available) ?? [];
  const bounded = dayBounded(day, loaded);
  const coverage = planCoverageText(day.plan_coverage, locale, t);
  if (!dayPartlyRead(day, loaded)) {
    return coverage ? <p className="t-caption">{coverage}</p> : null;
  }
  const warning = missing.length > 0 || planCoverageGap(day.plan_coverage);
  return (
    <>
      {!warning && coverage && <p className="t-caption">{coverage}</p>}
      {warning && (
        <Callout
          tone="warning"
          kind="standing"
          title={t("worklist.partialTitle")}
        >
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
      )}
      {bounded && (
        <Callout kind="standing" title={t("worklist.limits.title")}>
          <p>{t("worklist.limits.boundary")}</p>
          {limited.length > 0 && (
            <ul>
              {limited.map((source) => (
                <li key={source.source}>
                  {t("worklist.limits.source", {
                    source: sourceName(source.source, t),
                    scope: t(
                      source.personal
                        ? "worklist.limits.personal"
                        : "worklist.limits.selected",
                    ),
                  })}
                </li>
              ))}
            </ul>
          )}
          <p>{t("worklist.limits.remedy")}</p>
        </Callout>
      )}
    </>
  );
}
