import type { components } from "../api/schema";
import { useRecordZone } from "../app/recordzone";
import { Button, type ButtonVariant } from "../design-system/atoms";
import { Heading } from "../design-system/heading";
import { SurfaceState, sectionState } from "../design-system/surfacestate";
import { dateTileParts } from "../format/datetile";
import { formatTimeOfDay } from "../format/format";
import { type Locale, useLocale, useT } from "../i18n";
import "./contact360.css";

type Contact360 = components["schemas"]["Contact360"];
type Activity = components["schemas"]["Activity"];
type ContactMomentAction = components["schemas"]["ContactMomentAction"];

// --- Meetings ---------------------------------------------------------------

// The brief verb for one meeting, booked or already held — the backend
// assembles a brief for any meeting activity, and reading one afterwards is
// how a reader recovers what a room agreed.
//
// Every reason NOT to offer it is decided here rather than at each call site,
// because a third caller that forgets one of them ships a button that fails:
//
//   - no id, or a surface with no drawer to open — nothing to ask for.
//   - a row that is not a meeting — the endpoint answers 404 for any other
//     kind, by design.
//   - a meeting the reader may DISCOVER but not READ. The timeline carries
//     those deliberately, as `content_state: "withheld"`, so the reader knows
//     a conversation happened without seeing it. The brief endpoint applies
//     the stricter content gate, so offering the verb here would promise a
//     reader something their own grant refuses.
export function MeetingBriefAction({
  activity,
  onBriefMeeting,
  variant,
}: Readonly<{
  activity: Pick<Activity, "id" | "kind" | "content_state"> | undefined;
  onBriefMeeting?: (activityId: string) => void;
  // Indigo for the next meeting, where a reader is about to walk in and the
  // verb is the card's own lead; the default ghost for a meeting already
  // held, where the brief is one of several equal ways to look back.
  variant?: ButtonVariant;
}>) {
  const t = useT();
  if (!activity?.id || !onBriefMeeting) {
    return null;
  }
  if (activity.kind !== "meeting" || activity.content_state === "withheld") {
    return null;
  }
  const activityId = activity.id;
  return (
    <Button variant={variant} onClick={() => onBriefMeeting(activityId)}>
      {t("contact.meeting.brief")}
    </Button>
  );
}

// One verb off a moment, rendered where the meeting it is about already sits.
// The moment's OWN card (contacttoday.tsx) carries the same contract for the
// record's front page; this is the tab's copy of the button, not of the
// gating logic: `state` and `blocked_reason` come straight off the action.
function MeetingMomentAction({
  action,
  onAction,
}: Readonly<{
  action: ContactMomentAction;
  onAction?: (action: ContactMomentAction) => void;
}>) {
  const t = useT();
  if (!onAction) {
    return null;
  }
  const blocked = action.state === "blocked";
  return (
    <Button
      onClick={() => onAction(action)}
      reason={
        blocked
          ? (action.blocked_reason ?? t("contact.rail.blocked"))
          : undefined
      }
    >
      {action.label}
    </Button>
  );
}

// One meeting, dated: the card shape both the booked meeting and every one
// already held share, so a reader learns it once. `withheld` redacts the
// title exactly as the timeline's own row does: the same fact rendered a
// second way would be the two disagreeing about what "withheld" means.
function MeetingCard({
  startsAt,
  subject,
  withheld,
  participants,
  activity,
  locale,
  zone,
  note,
  briefVariant,
  onBriefMeeting,
  secondaryActions,
  onAction,
}: Readonly<{
  startsAt: string;
  subject?: string | null;
  withheld?: boolean;
  participants?: readonly Readonly<{ contact_id: string; full_name: string }>[];
  activity: Pick<Activity, "id" | "kind" | "content_state"> | undefined;
  locale: Locale;
  zone: string;
  note?: Readonly<{ by: string; whyNow: string }>;
  briefVariant?: ButtonVariant;
  onBriefMeeting?: (activityId: string) => void;
  secondaryActions?: readonly ContactMomentAction[];
  onAction?: (action: ContactMomentAction) => void;
}>) {
  const t = useT();
  const tile = dateTileParts(startsAt, locale, zone);
  const title = withheld
    ? t("timeline.withheld")
    : (subject ?? t("contact.meetings.untitled"));
  const meta = [
    formatTimeOfDay(startsAt, locale, zone),
    (participants ?? []).length > 0
      ? (participants ?? []).map((who) => who.full_name).join(", ")
      : undefined,
  ].filter(Boolean);
  return (
    <article className="pe-meeting">
      <time className="pe-meeting-date" dateTime={startsAt}>
        <span className="t-caption">{tile.weekday}</span>
        <span className="pe-meeting-day">{tile.day}</span>
        <span className="t-caption">{tile.month}</span>
      </time>
      <div className="pe-meeting-body">
        <p className="t-body pe-meeting-title">{title}</p>
        <p className="t-caption pe-meeting-meta">{meta.join(" · ")}</p>
        {note && (
          <p className="pe-meeting-note">
            <span className="pe-meeting-note-by">{note.by}</span> ·{" "}
            {note.whyNow}
          </p>
        )}
        <div className="pe-meeting-actions">
          <MeetingBriefAction
            activity={activity}
            onBriefMeeting={onBriefMeeting}
            variant={briefVariant}
          />
          {secondaryActions?.map((action) => (
            <MeetingMomentAction
              key={action.label}
              action={action}
              onAction={onAction}
            />
          ))}
        </div>
      </div>
    </article>
  );
}

/**
 * ContactMeetingsTab puts the meeting that has not happened yet above the ones
 * that have. The booked meeting is the server's own next-meeting read, taken
 * through this contact's activity link rather than their account's — the company's
 * answer names a meeting this contact may not be in.
 */
export function ContactMeetingsTab({
  view,
  loading = false,
  onBriefMeeting,
  onAction,
}: Readonly<{
  view?: Contact360;
  loading?: boolean;
  onBriefMeeting?: (activityId: string) => void;
  // Runs a moment's own verb ("Draft agenda" today, whatever the ladder grows
  // next tomorrow) through the page's one action loop, the same door the
  // record's front page opens.
  onAction?: (action: ContactMomentAction) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const recordZone = useRecordZone();
  // The booked meeting is drawn above, from the server's own next-meeting
  // read. It is also an activity, so an unfiltered list draws it a second time
  // under "already held" — which was merely untidy while the rows were inert
  // and becomes two identical brief buttons for one room now that they carry a
  // verb.
  const booked = view?.next_meeting?.activity_id;
  const met = (view?.activities?.data ?? [])
    .filter(
      (activity: Activity) =>
        activity.kind === "meeting" && activity.id !== booked,
    )
    // Newest first: the row above answers "what just happened", not "what was
    // logged first": the order the page's own list otherwise arrives in.
    .sort((a, b) => Date.parse(b.occurred_at) - Date.parse(a.occurred_at));
  const hasMore = view?.activities?.page.has_more ?? false;
  const past = sectionState(
    view,
    "activities",
    Boolean(view?.activities),
    met.length,
    loading,
  );
  const next = view?.next_meeting;
  // The prep chip and its agenda verb belong to the meeting the moment is
  // ABOUT (the next one) and to no other rung: a moment on a different
  // claim (a re-engagement, an overdue promise) has nothing to say about a
  // meeting at all.
  const meetingPrep =
    view?.moment?.rule === "meeting_prep" ? view.moment : undefined;
  return (
    <div className="record-stack">
      <section>
        <Heading size="large" className="t-h3">
          {t("contact.meetings.upcoming")}
        </Heading>
        <SurfaceState
          loadingLabel={t("contact.meetings.upcoming")}
          state={sectionState(
            view,
            "next_meeting",
            Boolean(view),
            next ? 1 : 0,
            loading,
          )}
          emptyLabel={t("contact.meetings.noneBooked")}
        >
          {next && (
            <MeetingCard
              startsAt={next.starts_at}
              subject={next.subject}
              participants={next.participants}
              // next_meeting carries no content_state because the 360
              // withholds the whole section rather than a redacted row, so a
              // booked meeting the reader can see here is one they can read.
              // The kind is stated for the same reason: this section IS the
              // meeting.
              activity={{ id: next.activity_id, kind: "meeting" }}
              locale={locale}
              zone={recordZone}
              note={
                meetingPrep && {
                  by: "Margince",
                  whyNow: meetingPrep.why_now,
                }
              }
              briefVariant="ai"
              onBriefMeeting={onBriefMeeting}
              secondaryActions={meetingPrep?.secondary_actions}
              onAction={onAction}
            />
          )}
        </SurfaceState>
      </section>
      <section>
        <Heading size="large" className="t-h3">
          {t("contact.meetings.past")}
        </Heading>
        <SurfaceState
          loadingLabel={t("contact.meetings.past")}
          state={past === "ready" && hasMore ? "partial" : past}
          emptyLabel={t("contact.meetings.noneLogged")}
        >
          {met.map((activity) => (
            <MeetingCard
              key={activity.id}
              startsAt={activity.occurred_at}
              subject={activity.subject}
              withheld={activity.content_state === "withheld"}
              activity={activity}
              locale={locale}
              zone={recordZone}
              onBriefMeeting={onBriefMeeting}
            />
          ))}
        </SurfaceState>
      </section>
    </div>
  );
}
