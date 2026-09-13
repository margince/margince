// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Panel, PanelBody, PanelRow } from "../design-system/panel";
import { type SectionState, SurfaceState } from "../design-system/surfacestate";
import { viewerZone } from "../format/timezone";
import { useLocale, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { BRIEF_FEED_LIMIT } from "./brief.feed";
import { useMorningDigest } from "./brief.queries";
import { overnightIsEmpty } from "./brief.rail.overnight";
import { scheduleIsEmpty, tasksIsEmpty } from "./brief.schedule";
import { waitingRows } from "./brief.sentence";
import {
  dealFactsText,
  itemTitle,
  phrasedReasons,
  reasonText,
  rowHref,
} from "./worklist.copy";
import { worklistLaneHref } from "./worklist.header";
import type { Worklist, WorklistItem } from "./worklist.queries";

export { OvernightPanel } from "./brief.rail.overnight";

function isRisk(item: WorklistItem): boolean {
  return item.category === "deals_at_risk" || (item.deal?.quiet_days ?? 0) > 0;
}

/** The same scoped queue as Today, excluding the prefix already on screen. */
export function remainingRisks(
  day: Worklist | undefined,
): readonly WorklistItem[] {
  return waitingRows(day).slice(BRIEF_FEED_LIMIT).filter(isRisk);
}

export function watchIsEmpty(
  day: Worklist | undefined,
  state: SectionState,
): boolean {
  return (
    state === "ready" &&
    day !== undefined &&
    !day.next_cursor &&
    !day.readings.more_available &&
    day.sources_unavailable.length === 0 &&
    !waitingRows(day).some(isRisk)
  );
}

export function WatchPanel({
  day,
  state,
}: Readonly<{ day: Worklist | undefined; state: SectionState }>) {
  const t = useT();
  const { locale } = useLocale();
  const rows = remainingRisks(day);
  const more = Boolean(day?.next_cursor || day?.readings.more_available);
  if (state === "loading" || (state === "ready" && rows.length === 0 && !more))
    return null;
  return (
    <Panel
      title={t("brief.panel.remainingRisk")}
      className="rail-panel"
      footer={
        day && (
          <a
            className="entity-link"
            href={worklistLaneHref("except_decisions", day.scope)}
          >
            {t("brief.readings.openPriorities")}
          </a>
        )
      }
    >
      <PanelBody>
        <SurfaceState
          state={state === "ready" && more ? "partial" : state}
          emptyLabel={t("brief.rail.quietWatch")}
          loadingLabel={t("brief.panel.remainingRisk")}
        >
          {rows.map((item) => (
            <PanelRow key={`${item.source}-${item.id}`}>
              <a className="entity-link" href={rowHref(item)}>
                {itemTitle(item, t, locale)}
              </a>
              <p className="t-caption">
                {dealFactsText(item, t, locale, viewerZone())}
              </p>
              <p className="t-caption">
                {phrasedReasons(item, false)
                  .map((reason) => reasonText(reason, t, locale, viewerZone()))
                  .filter(Boolean)
                  .join(" · ")}
              </p>
            </PanelRow>
          ))}
        </SurfaceState>
      </PanelBody>
    </Panel>
  );
}

export function RailQuiet({
  day,
  dayState,
  includeOvernight = true,
}: Readonly<{
  day: Worklist | undefined;
  dayState: SectionState;
  includeOvernight?: boolean;
}>) {
  const t = useT();
  const digestQuery = useMorningDigest();
  const silent: MessageKey[] = [];
  if (scheduleIsEmpty(day, dayState)) silent.push("brief.rail.quietSchedule");
  if (tasksIsEmpty(day, dayState)) silent.push("brief.rail.quietTasks");
  if (includeOvernight && overnightIsEmpty(digestQuery.data))
    silent.push("brief.rail.quietOvernight");
  if (watchIsEmpty(day, dayState)) silent.push("brief.rail.quietWatch");
  if (silent.length === 0) return null;
  return (
    <section id="brief-quiet">
      <Panel title={t("brief.panel.quiet")} className="rail-panel">
        {silent.map((key) => (
          <PanelRow key={key}>
            <span className="t-caption">{t(key)}</span>
          </PanelRow>
        ))}
      </Panel>
    </section>
  );
}
