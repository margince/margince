// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Clock, MapPin, UserMinus, UserPlus } from "lucide-react";
import { activityDay, type OccurredPrecision } from "../format/activitywhen";
import { dateTileParts } from "../format/datetile";
import { useLocale, useT } from "../i18n";
import {
  type CalendarBefore,
  type CalendarChange,
  changeBadge,
  changeSentence,
  guestLines,
  meetingWhen,
} from "../lib/calendarchange";
import { useViewerZone } from "../lib/viewerzone";
import { Badge } from "./atoms";
import "./meetingchange.css";

// The day as a small calendar page, as a meeting row on a record draws it.
function DateTile({
  at,
  precision,
  muted,
}: Readonly<{ at: string; precision: OccurredPrecision; muted: boolean }>) {
  const { locale } = useLocale();
  const zone = useViewerZone();
  const tile = activityDay(at, precision, locale, zone, dateTileParts);
  return (
    <time
      className={
        muted
          ? "meetingchange__date meetingchange__date--off"
          : "meetingchange__date"
      }
      dateTime={at}
    >
      <span className="meetingchange__date-edge">{tile.weekday}</span>
      <span className="meetingchange__day">{tile.day}</span>
      <span className="meetingchange__date-edge">{tile.month}</span>
    </time>
  );
}

/** What changed, one line each: the old value struck through beside the new. */
function ChangeLines({ change }: Readonly<{ change: CalendarChange }>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = useViewerZone();
  const before: CalendarBefore = change.before ?? {};
  const lines: { key: string; text: string; was?: string | null }[] = [];
  if (change.changed.includes("title") && before.title)
    lines.push({
      key: "title",
      text: t("meetingChange.was.title"),
      was: before.title,
    });
  if (change.changed.includes("time"))
    lines.push({
      key: "time",
      text:
        change.kind === "proposed"
          ? t("meetingChange.was.current")
          : t("meetingChange.was.time"),
      was: meetingWhen(before, locale, zone),
    });
  if (change.changed.includes("location"))
    lines.push({
      key: "location",
      text: t("meetingChange.was.location"),
      was: before.location ?? t("meetingChange.was.noPlace"),
    });
  const guests = guestLines(change, t);
  const shown = lines.filter((line) => line.was);
  if (shown.length === 0 && guests.length === 0) return null;
  // A proposal leaves the booking standing, so its time is not struck out.
  const Was = change.kind === "proposed" ? "span" : "s";
  return (
    <span className="meetingchange__changes">
      {shown.map((line) => (
        <span key={line.key}>
          {line.text} <Was className="meetingchange__was">{line.was}</Was>
        </span>
      ))}
      {guests.map((line, index) => (
        <span key={line}>
          {index === 0 && (change.guests_added?.length ?? 0) > 0 ? (
            <UserPlus aria-hidden="true" />
          ) : (
            <UserMinus aria-hidden="true" />
          )}
          {line}
        </span>
      ))}
    </span>
  );
}

/**
 * A calendar mail drawn as its meeting: the day, the title, when and where,
 * and what changed since the send before it. It takes the place of the mail's
 * words, which are the calendar's boilerplate. Phrasing content only, since
 * the row it sits in may be a button.
 */
export function MeetingChange({
  change,
  fallbackTitle,
}: Readonly<{
  change: CalendarChange;
  /** The mail's subject, for a calendar that sent no title. */
  fallbackTitle: string;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = useViewerZone();
  const badge = changeBadge(change);
  const canceled = change.kind === "canceled";
  const when = meetingWhen(change, locale, zone);
  const lead =
    change.kind === "proposed" || change.kind === "answered"
      ? changeSentence(change, t, locale, zone)
      : null;
  const moved = change.changed.includes("time");
  const placed = change.changed.includes("location");
  return (
    <span
      className={
        canceled ? "meetingchange meetingchange--canceled" : "meetingchange"
      }
    >
      {change.starts_at && (
        <DateTile
          at={change.starts_at}
          precision={change.starts_precision ?? "instant"}
          muted={canceled}
        />
      )}
      <span className="meetingchange__body">
        <span className="meetingchange__head">
          <span className="meetingchange__title">
            {change.title ?? fallbackTitle}
          </span>
          <Badge tone={badge.tone}>{t(badge.key)}</Badge>
        </span>
        {lead && <span className="meetingchange__lead">{lead}</span>}
        <span className="meetingchange__facts t-caption t-num">
          {when && (
            <span className={moved ? "meetingchange__fact--new" : undefined}>
              <Clock aria-hidden="true" />
              {when}
            </span>
          )}
          {change.location && (
            <span className={placed ? "meetingchange__fact--new" : undefined}>
              <MapPin aria-hidden="true" />
              {change.location}
            </span>
          )}
        </span>
        {!canceled && <ChangeLines change={change} />}
      </span>
    </span>
  );
}
