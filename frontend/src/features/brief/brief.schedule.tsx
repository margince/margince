// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Fragment } from "react";
import { EntityRef } from "../../components/entityref";
import { MeetingOthers } from "../../components/meetingothers";
import { AppLink } from "../../design-system/applink";
import { Badge, Button } from "../../design-system/atoms";
import { Panel, PanelRow } from "../../design-system/panel";
import {
  type SectionState,
  SurfaceState,
} from "../../design-system/surfacestate";
import { activityStartsIn, activityTimeOfDay } from "../../format/activitywhen";
import { formatTimeOfDay } from "../../format/format";
import { useNow } from "../../format/now";
import { type Locale, useLocale, useT } from "../../i18n";
import { isUnprepared } from "../../lib/meetingreadiness";
import { routeHash } from "../../lib/router";
import { settingsHref } from "../../lib/settingsrouting";
import { useViewerZone } from "../../lib/viewerzone";
import { itemTitle, moveHref, rowHref } from "../../lib/worklistcopy";
import type { Worklist, WorklistItem } from "../worklist/worklist.queries";
import {
  type CalendarDay,
  calendarDay,
  meetingsCounted,
  nextMeetingDay,
  sourceComplete,
} from "./brief.facts";
import "./brief.css";

type NextMeeting = NonNullable<Worklist["next_meeting"]>;

// Calendar context is drawn from the same loaded agenda, with explicit partial states.
const MEETING = "meeting";

/**
 * Whether the calendar answered and holds nothing more for today, which is
 * what lets the Now stop say the rest of the day is clear.
 */
function dayIsClear(day: Worklist): boolean {
  if (rowsFrom(day, MEETING).length > 0) return false;
  const calendar = calendarDay(day).state;
  return (
    calendar === "quiet" ||
    (calendar === "counted" &&
      meetingsCounted(day) === 0 &&
      sourceComplete(day, MEETING))
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
 * The day as one line: the meetings already under way, a Now stop, and the
 * meetings still ahead, by start time. The first timed meeting ahead is the
 * next one, set apart with how soon it starts and the way into its brief.
 *
 * A meeting not confirmed ready carries the badge here as well as in the queue.
 * All three places read `isUnprepared`, so the strip's figure and both badges
 * are one answer.
 */
export function SchedulePanel({
  day,
  state,
  onOpenMeeting,
  onPrepareMeeting,
  now,
}: Readonly<{
  day: Worklist | undefined;
  state: SectionState;
  /** Opens one meeting by its activity id; without it the title is text. */
  onOpenMeeting?: (id: string) => void;
  /** Has Margince write the next meeting's brief; without it there is no button. */
  onPrepareMeeting?: (id: string) => void;
  /** The clock the Now stop is read against; the live minute when absent. */
  now?: number;
}>) {
  const t = useT();
  const liveNow = useNow(now === undefined ? 60_000 : 0);
  const at = now ?? liveNow;
  const { started, ahead } = splitAtNow(rowsFrom(day, MEETING), at);
  const next = ahead.find((item) => item.occurred_precision !== "day");
  return (
    <Panel title={t("brief.panel.schedule")} flatHead className="rail-schedule">
      <SurfaceState
        loadingLabel={t("brief.panel.schedule")}
        state={state}
        emptyLabel={t("brief.rail.quietSchedule")}
      >
        {answered(state) && day && (
          <>
            {started.map((item) => (
              <MeetingRow
                key={item.id}
                item={item}
                onOpenMeeting={onOpenMeeting}
              />
            ))}
            <NowRow
              at={at}
              freeUntil={started.length === 0 ? next : undefined}
              clear={dayIsClear(day)}
            />
            {ahead.map((item) => (
              <MeetingRow
                key={item.id}
                item={item}
                onOpenMeeting={onOpenMeeting}
                onPrepare={item === next ? onPrepareMeeting : undefined}
                countdownFrom={item === next ? at : undefined}
              />
            ))}
            {rowsFrom(day, MEETING).length === 0 && (
              <NoMeetingRows
                clear={dayIsClear(day)}
                calendar={calendarDay(day)}
                onOpenMeeting={onOpenMeeting}
              />
            )}
          </>
        )}
      </SurfaceState>
    </Panel>
  );
}

/**
 * Today's meetings by start, cut at `now`. A meeting known only by its day
 * has no start to cut at, so it waits at the end of the ones ahead.
 */
function splitAtNow(
  meetings: WorklistItem[],
  now: number,
): { started: WorklistItem[]; ahead: WorklistItem[] } {
  const startOf = (item: WorklistItem) =>
    item.occurred_precision === "day" || !item.due_at
      ? Number.POSITIVE_INFINITY
      : Date.parse(item.due_at);
  const sorted = [...meetings].sort((a, b) => startOf(a) - startOf(b));
  return {
    started: sorted.filter((item) => startOf(item) <= now),
    ahead: sorted.filter((item) => startOf(item) > now),
  };
}

/** Where the reader stands on the day's line, and what the next hour holds. */
function NowRow({
  at,
  freeUntil,
  clear,
}: Readonly<{ at: number; freeUntil?: WorklistItem; clear: boolean }>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = useViewerZone();
  const nowIso = new Date(at).toISOString();
  const until = freeUntil && whenOf(freeUntil, locale, zone);
  const caption = until
    ? t("brief.schedule.freeUntil", { time: until })
    : clear
      ? t("brief.schedule.clearRest")
      : undefined;
  return (
    <PanelRow className="rail-schedule-row rail-schedule-now">
      <time className="t-caption rail-schedule-when" dateTime={nowIso}>
        {formatTimeOfDay(nowIso, locale, zone)}
      </time>
      <span className="rail-schedule-what">
        <span className="t-body rail-schedule-title">
          {t("brief.schedule.now")}
        </span>
        {caption && <span className="t-caption">{caption}</span>}
      </span>
    </PanelRow>
  );
}

/**
 * One meeting on the line. The next one also says how soon it starts and,
 * where the row names whose page the brief is read on, opens that brief.
 */
function MeetingRow({
  item,
  onOpenMeeting,
  onPrepare,
  countdownFrom,
}: Readonly<{
  item: WorklistItem;
  onOpenMeeting?: (id: string) => void;
  onPrepare?: (id: string) => void;
  /** The clock to count down from; set only on the next meeting. */
  countdownFrom?: number;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = useViewerZone();
  const isNext = countdownFrom !== undefined;
  const startsIn =
    isNext && item.due_at
      ? activityStartsIn(
          { at: item.due_at, precision: item.occurred_precision },
          countdownFrom,
          locale,
        )
      : null;
  return (
    <PanelRow
      className={
        isNext ? "rail-schedule-row rail-schedule-next" : "rail-schedule-row"
      }
    >
      <span className="t-caption rail-schedule-when">
        {whenOf(item, locale, zone)}
      </span>
      <span className="rail-schedule-what">
        <Title item={item} onOpenMeeting={onOpenMeeting} />
        {(startsIn || isUnprepared(item)) && (
          <span className="rail-schedule-meta">
            {startsIn && <span className="t-caption">{startsIn}</span>}
            {isUnprepared(item) && (
              <Badge tone="warning">{t("worklist.needsPrep")}</Badge>
            )}
          </span>
        )}
        {onPrepare && (
          <Button
            variant="ai"
            className="rail-schedule-prep"
            onClick={() => onPrepare(item.id)}
          >
            {t("worklist.verb.open_meeting_brief")}
          </Button>
        )}
      </span>
    </PanelRow>
  );
}

/** What the panel says on a day it has no meeting rows to draw. */
function NoMeetingRows({
  calendar,
  clear,
  onOpenMeeting,
}: Readonly<{
  calendar: CalendarDay;
  /** The Now stop already says the day is clear. */
  clear: boolean;
  onOpenMeeting?: (id: string) => void;
}>) {
  const t = useT();
  switch (calendar.state) {
    case "quiet":
      return (
        <NextMeetingRow next={calendar.next} onOpenMeeting={onOpenMeeting} />
      );
    case "not_connected":
      return (
        <>
          <CalendarRow
            sentence={t("brief.schedule.notConnected")}
            verb={t("brief.schedule.connect")}
          />
          {calendar.next && (
            <NextMeetingRow
              next={calendar.next}
              onOpenMeeting={onOpenMeeting}
            />
          )}
        </>
      );
    case "unreadable":
      return (
        <>
          <CalendarRow
            sentence={t("brief.schedule.unreadable")}
            verb={t("brief.schedule.reconnect")}
          />
          {calendar.next && (
            <NextMeetingRow
              next={calendar.next}
              onOpenMeeting={onOpenMeeting}
            />
          )}
        </>
      );
    case "unread":
      return <PanelRow>{t("brief.schedule.unavailable")}</PanelRow>;
    case "counted":
      return clear ? null : <PanelRow>{t("brief.schedule.more")}</PanelRow>;
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
      <AppLink
        className="link-button"
        href={routeHash(settingsHref("connections"))}
      >
        {verb}
      </AppLink>
    </PanelRow>
  );
}

/**
 * The next booked conversation, on the schedule's own line. Its gutter carries
 * a date rather than a time, which is what tells it apart from today's rows.
 */
function NextMeetingRow({
  next,
  onOpenMeeting,
}: Readonly<{ next: NextMeeting; onOpenMeeting?: (id: string) => void }>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = useViewerZone();
  const time = activityTimeOfDay(
    next.starts_at,
    next.occurred_precision,
    locale,
    zone,
  );
  const subject = next.subject || t("contact.meetings.untitled");
  return (
    <PanelRow className="rail-schedule-row">
      <span className="t-caption rail-schedule-when">
        {nextMeetingDay(next, locale, zone)}
      </span>
      <span className="rail-schedule-what">
        {/* The title opens the meeting itself; its deal is one click on. */}
        {onOpenMeeting ? (
          <Button
            variant="link"
            align="start"
            className="btn-valuelabel"
            onClick={() => onOpenMeeting(next.activity_id)}
          >
            {subject}
          </Button>
        ) : (
          <span className="t-body">{subject}</span>
        )}
        {time && (
          <span className="t-caption">
            {t("brief.schedule.nextAt", { time })}
          </span>
        )}
        {/* One run of text, so the commas wrap with the names they follow. */}
        <span className="t-caption rail-schedule-attendees">
          {next.participants?.map((participant, index) => (
            <Fragment key={participant.contact_id}>
              {index > 0 && ", "}
              <EntityRef
                kind="contact"
                id={participant.contact_id}
                name={participant.full_name}
              />
            </Fragment>
          ))}
          <MeetingOthers
            others={next.other_participants}
            more={next.participants_more}
            afterContacts={(next.participants?.length ?? 0) > 0}
          />
        </span>
      </span>
    </PanelRow>
  );
}

/** The row's own words, linked where the row names a record. */
function Title({
  item,
  onOpenMeeting,
}: Readonly<{ item: WorklistItem; onOpenMeeting?: (id: string) => void }>) {
  const t = useT();
  const { locale } = useLocale();
  const title = itemTitle(item, t, locale);
  // A meeting row's title opens the meeting itself; its record is a click on.
  if (item.source === MEETING && onOpenMeeting) {
    return (
      <Button
        variant="link"
        align="start"
        className="btn-valuelabel"
        onClick={() => onOpenMeeting(item.id)}
      >
        {title}
      </Button>
    );
  }
  const href = rowHref(item) ?? (item.move ? moveHref(item) : undefined);
  return href ? (
    <AppLink className="entity-link t-body" href={href}>
      {title}
    </AppLink>
  ) : (
    <span className="t-body">{title}</span>
  );
}

/**
 * When a meeting starts, or nothing.
 *
 * The start is on `due_at`, the deadline a reader is racing
 * (attention/meeting.go). The lane sets no `occurred_at`, since a meeting on
 * today's schedule has not occurred yet. A row without a start, or with only
 * its day known, draws no time.
 */
function whenOf(item: WorklistItem, locale: Locale, zone: string): string {
  if (!item.due_at) return "";
  return (
    activityTimeOfDay(item.due_at, item.occurred_precision, locale, zone) ?? ""
  );
}

/** One source's rows, in the server's order. */
function rowsFrom(day: Worklist | undefined, source: string): WorklistItem[] {
  return (day?.queue ?? []).filter((item) => item.source === source);
}
