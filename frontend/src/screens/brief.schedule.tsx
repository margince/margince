// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { routeHash } from "../app/router";
import { Badge } from "../design-system/atoms";
import { Panel, PanelRow } from "../design-system/panel";
import { type SectionState, SurfaceState } from "../design-system/surfacestate";
import { formatDayMonth, formatTimeOfDay } from "../format/format";
import { viewerZone } from "../format/timezone";
import { type Locale, useLocale, useT } from "../i18n";
import { type CalendarDay, calendarDay, sourceComplete } from "./brief.facts";
import { EntityRef } from "./entityref";
import { settingsHref } from "./settingsrouting";
import { isUnprepared, itemTitle, moveHref, rowHref } from "./worklist.copy";
import type { Worklist, WorklistItem } from "./worklist.queries";

type NextMeeting = NonNullable<Worklist["next_meeting"]>;

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
    rowsFrom(day, MEETING).length === 0 &&
    // A day whose calendar could not count, or whose next meeting is known,
    // still has something to say.
    calendarDay(day).state === "counted"
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
  return (
    <section id="brief-schedule">
      <Panel title={t("brief.panel.schedule")}>
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
          {meetings.length === 0 && answered(state) && day && (
            <NoMeetingRows calendar={calendarDay(day)} />
          )}
          {meetings.map((item) => (
            <PanelRow key={item.id} className="rail-schedule-row">
              <span className="t-caption rail-schedule-when">
                {whenOf(item, locale, zone)}
              </span>
              {/* The stop on the day's line. Decorative: the time beside it is
                  the fact, and the line the dots hang on is the panel's way of
                  reading as a schedule rather than as a list of sentences. */}
              <span className="rail-schedule-dot" aria-hidden="true" />
              <span className="rail-schedule-what">
                <Title item={item} />
                {isUnprepared(item) && (
                  <Badge tone="warning">{t("worklist.needsPrep")}</Badge>
                )}
              </span>
            </PanelRow>
          ))}
        </SurfaceState>
      </Panel>
    </section>
  );
}

/** What the panel says on a day it has no meeting rows to draw. */
function NoMeetingRows({ calendar }: Readonly<{ calendar: CalendarDay }>) {
  const t = useT();
  switch (calendar.state) {
    case "quiet":
      return <NextMeetingRow next={calendar.next} />;
    case "not_connected":
      return (
        <CalendarRow
          sentence={t("brief.schedule.notConnected")}
          verb={t("brief.schedule.connect")}
        />
      );
    case "unreadable":
      return (
        <CalendarRow
          sentence={t("brief.schedule.unreadable")}
          verb={t("brief.schedule.reconnect")}
        />
      );
    case "unread":
      return <PanelRow>{t("brief.schedule.unavailable")}</PanelRow>;
    case "counted":
      return <PanelRow>{t("brief.schedule.more")}</PanelRow>;
  }
}

/** Why the calendar counted nothing, and the way to where it is connected. */
function CalendarRow({
  sentence,
  verb,
}: Readonly<{ sentence: string; verb: string }>) {
  return (
    <PanelRow>
      {sentence}{" "}
      <a className="link-button" href={routeHash(settingsHref("connections"))}>
        {verb}
      </a>
    </PanelRow>
  );
}

/**
 * The next booked conversation, on the schedule's own line. Its gutter carries
 * a date rather than a time, which is what tells it apart from today's rows.
 */
function NextMeetingRow({ next }: Readonly<{ next: NextMeeting }>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = viewerZone();
  const subject = next.subject || t("contact.meetings.untitled");
  return (
    <PanelRow className="rail-schedule-row">
      <span className="t-caption rail-schedule-when">
        {formatDayMonth(next.starts_at, locale, zone)}
      </span>
      <span className="rail-schedule-dot" aria-hidden="true" />
      <span className="rail-schedule-what">
        {next.linked_deal_id ? (
          <EntityRef kind="deal" id={next.linked_deal_id} name={subject} />
        ) : (
          <span className="t-body">{subject}</span>
        )}
        <span className="t-caption">
          {t("brief.schedule.nextAt", {
            time: formatTimeOfDay(next.starts_at, locale, zone),
          })}
        </span>
        {next.participants?.map((participant) => (
          <EntityRef
            key={participant.contact_id}
            kind="contact"
            id={participant.contact_id}
            name={participant.full_name}
          />
        ))}
      </span>
    </PanelRow>
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
