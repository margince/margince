// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Clock, MapPin, Video } from "lucide-react";
import type { ReactNode } from "react";
import type { components } from "../api/schema";
import { useRecordZone } from "../app/recordzone";
import { routeHash } from "../app/router";
import { Badge } from "../design-system/atoms";
import { AvatarStack } from "../design-system/avatarstack";
import { OffsiteLink } from "../design-system/offsitelink";
import { PanelBody, PanelRow } from "../design-system/panel";
import { dateTileParts } from "../format/datetile";
import { calendarDaysUntil } from "../format/daysuntil";
import { formatNumber, formatTimeOfDay } from "../format/format";
import { formatTimeRange } from "../format/meetingtime";
import { useLocale, usePlural, useT } from "../i18n";
import { InvitationBadge } from "./booking-meeting-parts";
import { PROVIDER_VIDEO_APP, VIDEO_APP_NAME } from "./booking-video";
import { useMeetingInvitation } from "./meeting-invitation-query";

type Activity = components["schemas"]["Activity"];
type Invitation = components["schemas"]["MeetingInvitation"];
type NextMeeting = NonNullable<
  components["schemas"]["Contact360"]["next_meeting"]
>;
type InvitationStatus = Invitation["status"];

/**
 * Whether a meeting has a page of its own to open: one booked through
 * Margince, which the reader may read. The invitation behind it answers the
 * content gate, so a row the reader may only discover offers no way in.
 */
export function opensMeetingPage(
  activity: Pick<Activity, "invitation_status" | "content_state"> | undefined,
): boolean {
  return Boolean(
    activity?.invitation_status && activity.content_state !== "withheld",
  );
}

// When a recorded meeting ends, from its start and its length. A meeting
// logged without a length has no end to show.
function endOf(activity: Activity): string | undefined {
  const seconds = activity.duration_seconds ?? 0;
  if (seconds <= 0) return undefined;
  return new Date(
    Date.parse(activity.occurred_at) + seconds * 1000,
  ).toISOString();
}

function useMeetingSpan(start: string, end: string | undefined): string {
  const { locale } = useLocale();
  const zone = useRecordZone();
  return end
    ? formatTimeRange(start, end, locale, zone)
    : formatTimeOfDay(start, locale, zone);
}

// The day as a small calendar page: weekday over day over month.
function DateTile({ at }: Readonly<{ at: string }>) {
  const { locale } = useLocale();
  const zone = useRecordZone();
  const tile = dateTileParts(at, locale, zone);
  return (
    <time className="pe-meeting-date" dateTime={at}>
      <span className="pe-meeting-date-edge">{tile.weekday}</span>
      <span className="pe-meeting-day">{tile.day}</span>
      <span className="pe-meeting-date-edge">{tile.month}</span>
    </time>
  );
}

// How far off a meeting still ahead is, counted in the calendar its date tile
// is drawn in and from the read's own `as_of`.
function DaysAway({
  at,
  asOf,
  lead,
}: Readonly<{ at: string; asOf: string; lead?: boolean }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const zone = useRecordZone();
  const days = calendarDaysUntil(at, zone, new Date(asOf));
  if (days < 0) return null;
  const label =
    days === 0
      ? t("contact.meetings.today")
      : days === 1
        ? t("contact.meetings.tomorrow")
        : plural("contact.meetings.inDays", days, {
            days: formatNumber(days, locale),
          });
  return <Badge tone={lead ? "accent" : "default"}>{label}</Badge>;
}

// The subject, and the way into the meeting's own page where it has one.
function MeetingTitle({
  activityId,
  opens,
  lead,
  children,
}: Readonly<{
  activityId: string;
  opens: boolean;
  lead?: boolean;
  children: string;
}>) {
  const className = lead
    ? "pe-meeting-title pe-meeting-title-lead"
    : "pe-meeting-title";
  if (!opens) return <span className={className}>{children}</span>;
  return (
    <a
      className={`entity-link ${className}`}
      href={routeHash({ screen: "book", id: `meeting-${activityId}` })}
    >
      {children}
    </a>
  );
}

// What became of a meeting, said only where it is not the ordinary answer: a
// calendar still working or refusing, a contact who did not come, a meeting
// called off. A confirmed invitation and a meeting held are what the list
// already implies.
function MeetingStatus({
  outcome,
  invitation,
}: Readonly<{
  outcome?: Activity["meeting_status"];
  invitation?: InvitationStatus | null;
}>) {
  const t = useT();
  if (invitation && invitation !== "confirmed")
    return <InvitationBadge status={invitation} />;
  if (outcome === "no_show")
    return <Badge tone="warning">{t("contact.meetings.noShow")}</Badge>;
  if (outcome === "canceled")
    return <Badge>{t("contact.meetings.canceled")}</Badge>;
  return null;
}

/**
 * One meeting as a row of a panel: its day, its subject and time, and the
 * verbs the caller offers on it at the far end. `withheld` redacts the subject
 * exactly as the timeline's own row does.
 */
export function MeetingRow({
  activity,
  asOf,
  verbs,
}: Readonly<{
  activity: Activity;
  // The read's own clock, for a meeting still ahead: it then says how far off.
  asOf?: string;
  verbs?: ReactNode;
}>) {
  const t = useT();
  const span = useMeetingSpan(activity.occurred_at, endOf(activity));
  const title =
    activity.content_state === "withheld"
      ? t("timeline.withheld")
      : (activity.subject ?? t("contact.meetings.untitled"));
  return (
    <PanelRow>
      <article className="pe-meeting">
        <DateTile at={activity.occurred_at} />
        <div className="pe-meeting-body">
          <div className="pe-meeting-headline">
            <MeetingTitle
              activityId={activity.id}
              opens={opensMeetingPage(activity)}
            >
              {title}
            </MeetingTitle>
            {asOf && <DaysAway at={activity.occurred_at} asOf={asOf} />}
            <MeetingStatus
              outcome={activity.meeting_status}
              invitation={activity.invitation_status}
            />
          </div>
          <p className="t-caption t-num pe-meeting-meta">{span}</p>
        </div>
        <div className="pe-meeting-verbs">{verbs}</div>
      </article>
    </PanelRow>
  );
}

// The call's own join link, once the calendar has made one.
function JoinCall({ invitation }: Readonly<{ invitation: Invitation }>) {
  const t = useT();
  const ended =
    invitation.status === "canceled" || invitation.status === "canceling";
  if (!invitation.video_url || ended) return null;
  const app = invitation.provider
    ? VIDEO_APP_NAME[PROVIDER_VIDEO_APP[invitation.provider]]
    : undefined;
  return (
    <li>
      <Video aria-hidden="true" />
      <OffsiteLink href={invitation.video_url}>
        {app
          ? t("contact.meetings.join", { app })
          : t("contact.meetings.joinCall")}
      </OffsiteLink>
    </li>
  );
}

/**
 * The meeting the reader walks into next, drawn larger than the rest: who is
 * in the room, how to join, and what to do before it. A meeting booked through
 * Margince reads its invitation too, for the end time, the place, the join link
 * and whether the calendar has taken it — the same read the meeting's own page
 * makes, so the two agree.
 */
export function NextMeetingCard({
  next,
  asOf,
  activity,
  note,
  verbs,
}: Readonly<{
  next: NextMeeting;
  asOf: string;
  // The meeting's row in the contact's list, which says whether it was booked
  // through Margince. Absent until that list arrives.
  activity?: Activity;
  // Margince's own line about why this meeting is worth reading ahead of.
  note?: Readonly<{ by: string; whyNow: string }>;
  verbs: ReactNode;
}>) {
  const t = useT();
  const opens = opensMeetingPage(activity);
  const invitation = useMeetingInvitation(
    opens ? next.activity_id : undefined,
  ).data;
  const span = useMeetingSpan(
    next.starts_at,
    invitation?.end ?? (activity && endOf(activity)),
  );
  const place = invitation?.location.trim();
  const attendees = next.participants ?? [];
  return (
    <PanelBody>
      <article className="pe-meeting pe-meeting-lead">
        <DateTile at={next.starts_at} />
        <div className="pe-meeting-body">
          <div className="pe-meeting-headline">
            <MeetingTitle activityId={next.activity_id} opens={opens} lead>
              {next.subject ?? t("contact.meetings.untitled")}
            </MeetingTitle>
            <DaysAway at={next.starts_at} asOf={asOf} lead />
            <MeetingStatus
              invitation={invitation?.status ?? activity?.invitation_status}
            />
          </div>
          <ul className="pe-meeting-facts">
            <li>
              <Clock aria-hidden="true" />
              <span className="t-num">{span}</span>
            </li>
            {place && (
              <li>
                <MapPin aria-hidden="true" />
                <span>{place}</span>
              </li>
            )}
            {invitation && <JoinCall invitation={invitation} />}
          </ul>
          {attendees.length > 0 && (
            <div className="pe-meeting-attendees">
              <AvatarStack
                contacts={attendees.map((who) => ({
                  name: who.full_name,
                  identity: who.contact_id,
                }))}
              />
              <span className="t-caption">
                {attendees.map((who) => who.full_name).join(", ")}
              </span>
            </div>
          )}
          {note && (
            <p className="pe-meeting-note">
              <span className="pe-meeting-note-by">{note.by}</span> ·{" "}
              {note.whyNow}
            </p>
          )}
          <div className="pe-meeting-actions">{verbs}</div>
        </div>
      </article>
    </PanelBody>
  );
}
