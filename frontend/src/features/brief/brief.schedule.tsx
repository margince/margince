// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";
import { MeetingOthers } from "../../components/meetingothers";
import { RecordMarkChip } from "../../components/recordlinks";
import { AppLink } from "../../design-system/applink";
import { Badge, Button } from "../../design-system/atoms";
import { Panel, PanelRow } from "../../design-system/panel";
import {
  type SectionState,
  SurfaceState,
} from "../../design-system/surfacestate";
import { activityStartsIn, activityTimeOfDay } from "../../format/activitywhen";
import { useNow } from "../../format/now";
import { type Locale, useLocale, useT } from "../../i18n";
import { captureRefusalOf, useConnectors } from "../../lib/connectors";
import { ENTITY } from "../../lib/entity";
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
type MeetingRoom = components["schemas"]["WorklistMeetingRoom"];
type ContactEmployer = components["schemas"]["ContactEmployer"];

// Calendar context is drawn from the same loaded agenda, with explicit partial states.
const MEETING = "meeting";

/**
 * Whether the calendar answered and holds nothing more for today, which is
 * what lets the panel say no external meeting is left.
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
 * Today's external meetings by start time, each saying who it is with. The
 * server sends only meetings with somebody from outside the own domains, so
 * this panel applies no rule of its own. The next timed meeting sits on an
 * outlined plate with how soon it starts and the way into its brief.
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
  /** The clock "starts in" counts from; the live minute when absent. */
  now?: number;
}>) {
  const t = useT();
  const liveNow = useNow(now === undefined ? 60_000 : 0);
  const at = now ?? liveNow;
  const meetings = byStart(rowsFrom(day, MEETING));
  const next = meetings.find(
    (item) =>
      item.occurred_precision !== "day" &&
      item.due_at !== undefined &&
      Date.parse(item.due_at) > at,
  );
  return (
    <Panel title={t("brief.panel.schedule")} flatHead className="rail-schedule">
      <SurfaceState
        loadingLabel={t("brief.panel.schedule")}
        state={state}
        emptyLabel={t("brief.rail.quietSchedule")}
      >
        {answered(state) && day && (
          <>
            {withoutCalendar(day) && (
              <CalendarRow
                sentence={t("brief.schedule.notConnected")}
                verb={t("brief.schedule.connect")}
              />
            )}
            {meetings.map((item) => (
              <MeetingRow
                key={item.id}
                item={item}
                onOpenMeeting={onOpenMeeting}
                onPrepare={item === next ? onPrepareMeeting : undefined}
                countdownFrom={item === next ? at : undefined}
              />
            ))}
            {meetings.length === 0 && (
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
 * Whether the reader has no calendar. Every meeting the panel shows is then
 * one booked, logged or imported in Margince, and the panel says so.
 */
function withoutCalendar(day: Worklist): boolean {
  return (
    day.calendar === "not_connected" && calendarDay(day).state !== "unread"
  );
}

/**
 * Today's meetings by start. A meeting known only by its day has no start, so
 * it follows the timed ones.
 */
function byStart(meetings: WorklistItem[]): WorklistItem[] {
  const startOf = (item: WorklistItem) =>
    item.occurred_precision === "day" || !item.due_at
      ? Number.POSITIVE_INFINITY
      : Date.parse(item.due_at);
  return [...meetings].sort((a, b) => startOf(a) - startOf(b));
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
        <MeetingWho
          company={item.contact?.employer}
          room={item.room ?? roomOfContact(item)}
        />
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
  /** The calendar answered and holds no external meeting for the rest of today. */
  clear: boolean;
  onOpenMeeting?: (id: string) => void;
}>) {
  const t = useT();
  switch (calendar.state) {
    case "quiet":
      return (
        <NextMeetingRow next={calendar.next} onOpenMeeting={onOpenMeeting} />
      );
    // The panel's head already says where these meetings come from.
    case "not_connected":
      return calendar.next ? (
        <NextMeetingRow next={calendar.next} onOpenMeeting={onOpenMeeting} />
      ) : null;
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
      return (
        <PanelRow>
          {clear ? t("brief.schedule.clearRest") : t("brief.schedule.more")}
        </PanelRow>
      );
  }
}

/** Why the calendar counted nothing, and the way to where it is connected. */
function CalendarRow({
  sentence,
  verb,
}: Readonly<{ sentence: string; verb: string }>) {
  const t = useT();
  // A seat or role that may not capture connects nothing: it is told why, and
  // never sent to a setting it cannot change.
  const refusal = captureRefusalOf(useConnectors().data);
  return (
    <PanelRow>
      {sentence}
      {refusal !== null && ` ${t(refusal)}`}
      {refusal === null && (
        <>
          {" "}
          <AppLink
            className="link-button"
            href={routeHash(settingsHref("connections"))}
          >
            {verb}
          </AppLink>
        </>
      )}
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
        <MeetingWho room={{ ...next, participants: next.participants ?? [] }} />
      </span>
    </PanelRow>
  );
}

/**
 * Who a meeting is with: the account, then the outside contacts, as the same
 * linked chips a task card names its records with. Outside addresses and how
 * many more there are follow as text. Colleagues are left out, because every
 * meeting here has them and the reader is asking which customer it is.
 */
function MeetingWho({
  company,
  room,
}: Readonly<{ company?: ContactEmployer; room?: MeetingRoom }>) {
  const contacts = room?.participants ?? [];
  const outside = (room?.other_participants ?? []).filter(
    (other) => other.kind !== "colleague",
  );
  const more = room?.participants_more ?? 0;
  if (!company && contacts.length === 0 && outside.length === 0 && more === 0) {
    return null;
  }
  return (
    <span className="rail-schedule-attendees">
      {company && (
        <RecordMarkChip
          id={company.company_id}
          name={company.company_name}
          href={routeHash(ENTITY.company.route(company.company_id))}
        />
      )}
      {contacts.map((contact) => (
        <RecordMarkChip
          key={contact.contact_id}
          id={contact.contact_id}
          name={contact.full_name}
          href={routeHash(ENTITY.contact.route(contact.contact_id))}
        />
      ))}
      {(outside.length > 0 || more > 0) && (
        <span className="t-caption">
          <MeetingOthers others={outside} more={more} afterContacts={false} />
        </span>
      )}
    </span>
  );
}

/**
 * The room a row without one still names: the counterparty the lane chose,
 * where the reader may read them.
 */
function roomOfContact(item: WorklistItem): MeetingRoom | undefined {
  const label = item.contact?.label;
  if (!item.contact || !label) return undefined;
  return { participants: [{ contact_id: item.contact.id, full_name: label }] };
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
