// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Badge } from "../design-system/atoms";
import { Panel, PanelRow } from "../design-system/panel";
import { type SectionState, SurfaceState } from "../design-system/surfacestate";
import { formatTimeOfDay } from "../format/format";
import { viewerZone } from "../format/timezone";
import { type Locale, useLocale, useT } from "../i18n";
import { sourceComplete } from "./brief.facts";
import { isUnprepared, itemTitle, moveHref, rowHref } from "./worklist.copy";
import type { Worklist, WorklistItem } from "./worklist.queries";

// Calendar context is drawn from the same loaded agenda, with explicit partial states.
const MEETING = "meeting";

/**
 * Whether the day's schedule has nothing to draw.
 *
 * Exported because two surfaces turn on this one answer — the panel, which
 * draws nothing, and the rail's quiet panel, which prints the line standing in
 * for it. Spelled twice they would eventually disagree, and a reader would meet
 * an empty panel and the line announcing its absence on the same rail.
 */
export function scheduleIsEmpty(
  day: Worklist | undefined,
  state: SectionState,
): boolean {
  return (
    answered(state) &&
    day !== undefined &&
    sourceComplete(day, MEETING) &&
    rowsFrom(day, MEETING).length === 0
  );
}

/**
 * Whether the read has ANSWERED, which is what makes an absence of rows mean
 * there are none. Every other state is a fact about the request.
 */
function answered(state: SectionState): boolean {
  return state === "ready" || state === "empty";
}

/**
 * The day's schedule, in the order the server ranked it.
 *
 * A meeting with nothing prepared for it carries the badge here as well as in
 * the queue — the same `isUnprepared` the readings strip counts, so the figure
 * in the strip, the badge in the queue and the badge here are one answer.
 */
export function SchedulePanel({
  day,
  state,
}: Readonly<{ day: Worklist | undefined; state: SectionState }>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = viewerZone();
  if (scheduleIsEmpty(day, state)) {
    return null;
  }
  const meetings = rowsFrom(day, MEETING);
  const calendarFailed = day?.sources_unavailable.some(
    (entry) => entry.source === MEETING,
  );
  return (
    <section id="brief-schedule">
      <Panel title={t("brief.panel.schedule")} className="rail-panel">
        {/* The rows are `PanelRow`s and carry the panel's own gutter, so they
            sit in the Panel directly — inside a `PanelBody` they would be
            padded twice and read as an indented block against every other
            panel in the rail. `SurfaceState` draws its sentence either way. */}
        <SurfaceState
          loadingLabel={t("brief.panel.schedule")}
          state={state}
          // The words the rail prints for this absence, so the panel and the
          // quiet line cannot report one morning in two vocabularies.
          emptyLabel={t("brief.rail.quietSchedule")}
        >
          {meetings.length === 0 && answered(state) && (
            <PanelRow>
              {t(
                calendarFailed
                  ? "brief.schedule.unavailable"
                  : "brief.schedule.more",
              )}
            </PanelRow>
          )}
          {meetings.map((item) => (
            <PanelRow key={item.id} className="rail-schedule-row">
              <span className="t-caption rail-schedule-when">
                {whenOf(item, locale, zone)}
              </span>
              <span className="rail-schedule-what">
                <Title item={item} />
                {isUnprepared(item) && (
                  <Badge tone="warn">{t("worklist.needsPrep")}</Badge>
                )}
              </span>
            </PanelRow>
          ))}
        </SurfaceState>
      </Panel>
    </section>
  );
}

/** The row's own words, linked where the row names a record. */
function Title({ item }: Readonly<{ item: WorklistItem }>) {
  const t = useT();
  const { locale } = useLocale();
  const title = itemTitle(item, t, locale);
  const href = rowHref(item) ?? (item.move ? moveHref(item) : undefined);
  return href ? (
    <a className="entity-link t-body" href={href}>
      {title}
    </a>
  ) : (
    <span className="t-body">{title}</span>
  );
}

/**
 * When a meeting starts, or nothing.
 *
 * The start is on `due_at`. The meeting lane puts it there deliberately — it is
 * the deadline a reader is racing (attention/meeting.go) — and sets no
 * `occurred_at` at all, because a meeting on today's schedule has not occurred
 * yet. Reading the other field drew every row with a blank time.
 *
 * A row the server sent without one is still drawn without a time rather than
 * with an invented one.
 */
function whenOf(item: WorklistItem, locale: Locale, zone: string): string {
  return item.due_at ? formatTimeOfDay(item.due_at, locale, zone) : "";
}

/** One source's rows, in the server's order. */
function rowsFrom(day: Worklist | undefined, source: string): WorklistItem[] {
  return (day?.queue ?? []).filter((item) => item.source === source);
}
