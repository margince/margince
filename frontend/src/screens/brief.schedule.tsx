// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Badge } from "../design-system/atoms";
import { Panel, PanelRow } from "../design-system/panel";
import { type SectionState, SurfaceState } from "../design-system/surfacestate";
import { formatTimeOfDay } from "../format/format";
import { viewerZone } from "../format/timezone";
import { type Locale, useLocale, useT } from "../i18n";
import { isUnprepared, itemTitle, rowHref } from "./worklist.copy";
import type { Worklist, WorklistItem } from "./worklist.queries";

// The two rail panels the morning is read alongside: what the day is booked
// with, and what this rep owes.
//
// Both are cuts of the ONE worklist answer the work column is drawn from, not
// reads of their own. The rail is context for the work beside it, and a rail
// that fetched separately could show a meeting the queue had already dropped.
//
// NEITHER SORTS. The order is the server's, the same order the queue prints,
// so the rail and the work column cannot disagree about what comes first.
//
// A PANEL WITH NOTHING IN IT DOES NOT EARN ITS BOX. Either of these on a clear
// day used to draw a header band, a hairline and one grey sentence — two boxes
// of chrome around eleven words, which cost the populated panels beside them
// the reader's eye. Empty, the panel renders nothing and the rail's own
// `RailQuiet` prints the one line that says so (screens/brief.rail.tsx). A read
// still in flight or a read that failed still draws in full: those are facts
// about the request, and collapsing them would tell a reader their day was
// clear on the strength of an answer nobody received.

const MEETING = "meeting";
const TASK = "task";

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
  return answered(state) && rowsFrom(day, MEETING).length === 0;
}

/** Whether this rep has no task due today. Same contract as above. */
export function tasksIsEmpty(
  day: Worklist | undefined,
  state: SectionState,
): boolean {
  return answered(state) && rowsFrom(day, TASK).length === 0;
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

/**
 * What this rep owes today: the tasks due on them.
 *
 * TASKS ONLY, AND THE TITLE SAYS SO. It read "Promises & tasks" over a
 * disclaimer explaining that a promise made in conversation reaches nothing —
 * a heading that named a thing the product does not have, and a standing line
 * of apology in the narrowest column on the page. The panel now claims exactly
 * what it lists, which is what the disclaimer existed to walk back.
 */
export function PromisesPanel({
  day,
  state,
}: Readonly<{ day: Worklist | undefined; state: SectionState }>) {
  const t = useT();
  if (tasksIsEmpty(day, state)) {
    return null;
  }
  const tasks = rowsFrom(day, TASK);
  return (
    <section id="brief-tasks">
      <Panel title={t("brief.panel.tasks")} className="rail-panel">
        <SurfaceState
          loadingLabel={t("brief.panel.tasks")}
          state={state}
          emptyLabel={t("brief.rail.quietTasks")}
        >
          {tasks.map((item) => (
            <PanelRow key={item.id} className="rail-promise-row">
              <Title item={item} />
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
  const href = rowHref(item);
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
