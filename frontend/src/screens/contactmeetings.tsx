import { Link } from "lucide-react";
import { type Ref, useRef, useState } from "react";
import type { components } from "../api/schema";
import { useCanWrite } from "../app/capability";
import { navigate } from "../app/router";
import { ActionRow } from "../design-system/actionrow";
import { Button, type ButtonVariant } from "../design-system/atoms";
import { useClipboardCopy } from "../design-system/clipboardcopy";
import {
  Panel,
  PanelBody,
  RailPanel,
  type RailPanelState,
} from "../design-system/panel";
import {
  type TimelineFilters,
  useRecordTimeline,
} from "../design-system/recordtimeline";
import {
  omitted,
  SurfaceState,
  sectionState,
} from "../design-system/surfacestate";
import { useT } from "../i18n";
import { LoadMoreButton, useMe } from "./common";
import { MeetingRow, NextMeetingCard } from "./contactmeetings.rows";
import {
  type CopiedLink,
  useMeetingProposals,
  WaitingSection,
} from "./contactmeetings.waiting";
import { useSchedulingProfile } from "./scheduling-profile-query";
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

const MEETINGS_ONLY: TimelineFilters = { kind: "meeting" };

// Which of the contact's meetings still lie ahead and which are behind, on the
// server's clock: the read's own `as_of`, so a reader whose computer clock is
// wrong still sees the meeting where it belongs. The soonest booked one is
// drawn on its own, from the server's next-meeting read, and is not listed a
// second time.
function splitMeetings(
  listed: readonly Activity[],
  asOf: string,
  nextId: string | undefined,
): Readonly<{ ahead: Activity[]; held: Activity[] }> {
  const now = Date.parse(asOf);
  const at = (activity: Activity) => Date.parse(activity.occurred_at);
  const others = listed.filter((activity) => activity.id !== nextId);
  return {
    ahead: others
      .filter((activity) => at(activity) > now)
      .sort((a, b) => at(a) - at(b)),
    // Newest first: the row on top answers "what just happened".
    held: others
      .filter((activity) => at(activity) <= now)
      .sort((a, b) => at(b) - at(a)),
  };
}

// The held list is the contact's own meetings read, page by page, so it can
// say more than the composite read's first page of every kind carries.
function heldState(
  view: Contact360 | undefined,
  loading: boolean,
  listed: ReturnType<typeof useRecordTimeline>,
  count: number,
): RailPanelState {
  if (!view) return loading ? "loading" : "unavailable";
  if (omitted(view, "activities")) return "withheld";
  if (count > 0) return "ready";
  if (listed.isError) return "failed";
  return listed.isPending ? "loading" : "empty";
}

/**
 * ContactMeetingsTab reads as three panels: what is booked, what is still
 * waiting on the contact's answer, and what has already been held. The booked
 * meeting leads, from the server's own next-meeting read taken through this
 * contact's activity link rather than their account's — the company's answer
 * names a meeting this contact may not be in.
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
  const canBook = useCanWrite("activity", "create");
  // Only a reader who may book reads the proposals: the list answers 403 to
  // anyone else, and the section they fill is theirs alone.
  const proposals = useMeetingProposals(canBook ? view?.contact.id : undefined);
  const listed = useRecordTimeline("contact", view?.contact.id ?? "", {
    filters: MEETINGS_ONLY,
    enabled: view !== undefined && !omitted(view, "activities"),
  });
  const next = view?.next_meeting;
  const { ahead, held } = splitMeetings(
    listed.activities,
    view?.as_of ?? "",
    next?.activity_id,
  );
  // The prep chip and its agenda verb belong to the meeting the moment is
  // ABOUT (the next one) and to no other rung: a moment on a different
  // claim (a re-engagement, an overdue promise) has nothing to say about a
  // meeting at all.
  const meetingPrep =
    view?.moment?.rule === "meeting_prep" ? view.moment : undefined;
  const bookButton = useRef<HTMLButtonElement>(null);
  const [copiedUrl, setCopiedUrl] = useState<string | null>(null);
  const copied: CopiedLink = { url: copiedUrl, onCopied: setCopiedUrl };
  const bookingLink = useBookingLink(copied);
  const upcoming = sectionState(
    view,
    "next_meeting",
    Boolean(view),
    (next ? 1 : 0) + ahead.length,
    loading,
  );
  const briefOf = (activity: Activity) => (
    <MeetingBriefAction activity={activity} onBriefMeeting={onBriefMeeting} />
  );
  return (
    <div className="record-stack pe-meetings">
      <Panel title={t("contact.meetings.upcoming")}>
        <PanelBody className="pe-meetings-toolbar">
          <ActionRow
            primary={
              <BookMeeting
                view={view}
                loading={loading}
                canBook={canBook}
                ref={bookButton}
              />
            }
          >
            {bookingLink.url && (
              <Button onClick={bookingLink.copy.copy}>
                <Link aria-hidden="true" />
                {t(
                  copied.url === bookingLink.url
                    ? "scheduling.copied"
                    : "contact.meetings.copyBookingLink",
                )}
              </Button>
            )}
          </ActionRow>
          {bookingLink.copy.notice && (
            <div className="pe-copy-notice">
              {bookingLink.copy.notice}
              <p className="pe-copy-url">{bookingLink.url}</p>
            </div>
          )}
        </PanelBody>
        {upcoming === "ready" ? (
          <>
            {next && (
              <NextMeetingCard
                next={next}
                activity={listed.activities.find(
                  (activity) => activity.id === next.activity_id,
                )}
                note={
                  meetingPrep && {
                    by: "Margince",
                    whyNow: meetingPrep.why_now,
                  }
                }
                verbs={
                  <>
                    <MeetingBriefAction
                      // next_meeting carries no content_state because the 360
                      // withholds the whole section rather than a redacted
                      // row, so a booked meeting the reader can see here is
                      // one they can read. The kind is stated for the same
                      // reason: this section IS the meeting.
                      activity={{ id: next.activity_id, kind: "meeting" }}
                      onBriefMeeting={onBriefMeeting}
                      variant="ai"
                    />
                    {meetingPrep?.secondary_actions?.map((action) => (
                      <MeetingMomentAction
                        key={action.label}
                        action={action}
                        onAction={onAction}
                      />
                    ))}
                  </>
                }
              />
            )}
            {ahead.map((activity) => (
              <MeetingRow
                key={activity.id}
                activity={activity}
                verbs={briefOf(activity)}
              />
            ))}
          </>
        ) : (
          <PanelBody>
            <SurfaceState
              state={upcoming}
              loadingLabel={t("contact.meetings.upcoming")}
              emptyLabel={t("contact.meetings.noneBooked")}
            >
              {null}
            </SurfaceState>
          </PanelBody>
        )}
      </Panel>
      {view && (
        <WaitingSection
          contact={view.contact}
          proposals={proposals}
          copied={copied}
          afterWithdraw={() => bookButton.current}
        />
      )}
      <RailPanel
        title={t("contact.meetings.past")}
        state={heldState(view, loading, listed, held.length)}
        emptyLabel={t("contact.meetings.noneLogged")}
        detail={{ onRetry: () => void listed.refetch() }}
        footer={listed.hasNextPage ? <LoadMoreButton query={listed} /> : null}
      >
        {held.map((activity) => (
          <MeetingRow
            key={activity.id}
            activity={activity}
            verbs={briefOf(activity)}
          />
        ))}
      </RailPanel>
    </div>
  );
}

// The tab's own verb: a meeting with this contact, from the booking flow.
function BookMeeting({
  view,
  loading,
  canBook,
  ref,
}: Readonly<{
  view?: Contact360;
  loading: boolean;
  canBook: boolean;
  ref: Ref<HTMLButtonElement>;
}>) {
  const t = useT();
  const me = useMe();
  const grantKnown = me.data?.authorization !== undefined;
  return (
    <Button
      ref={ref}
      variant="primary"
      disabled={loading || !view || !grantKnown}
      reason={
        view && grantKnown && !canBook ? t("scheduling.bookRefused") : undefined
      }
      onClick={() => {
        if (view)
          navigate({ screen: "book", id: `contact-${view.contact.id}` });
      }}
    >
      {t("scheduling.bookContact")}
    </Button>
  );
}

// The reader's own public link, for a contact who would rather pick a time
// themselves. Offered only while the page takes bookings: a paused link hands
// the contact a page that turns them away.
function useBookingLink(copied: CopiedLink) {
  const t = useT();
  const profile = useSchedulingProfile();
  const url = profile.data?.enabled ? (profile.data.public_url ?? "") : "";
  const copy = useClipboardCopy(
    url,
    {
      copy: t("contact.meetings.copyBookingLink"),
      copied: t("scheduling.copied"),
      remedy: t("scheduling.copyFallback"),
    },
    () => copied.onCopied(url),
  );
  return { url, copy };
}
