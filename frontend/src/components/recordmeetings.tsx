// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
import type { components } from "../api/schema";
import { Badge, Button, type ButtonVariant } from "../design-system/atoms";
import {
  Panel,
  PanelBody,
  RailPanel,
  type RailPanelState,
} from "../design-system/panel";
import { LoadMoreButton } from "../design-system/querystates";
import {
  type TimelineFilters,
  useRecordTimeline,
} from "../design-system/recordtimeline";
import {
  omitted,
  SurfaceState,
  sectionState,
  type Withholding,
} from "../design-system/surfacestate";
import { formatNumber } from "../format/format";
import { useLocale, useT } from "../i18n";
import { entityTimelineKeys } from "../lib/activitykeys";
import { useActivity } from "../lib/activityread";
import { bookedCount, MeetingFigures, timelineRead } from "./meetingfigures";
import { RecordMeetingOutcome } from "./meetingoutcome";
import { MeetingRow, type NextMeeting, NextMeetingCard } from "./meetingrows";
import { HeldMeetingsList } from "./meetingsheld";

type Activity = components["schemas"]["Activity"];
type Listed = ReturnType<typeof useRecordTimeline>;

// The brief verb for one meeting, booked or already held. Read afterwards, a
// brief is how a reader recovers what a room agreed.
//
// Every reason not to offer it is decided here, so no caller can forget one:
//
//   - no id, or no drawer to open: nothing to ask for.
//   - a row that is not a meeting: the endpoint answers 404 for other kinds.
//   - a meeting the reader may discover but not read (`content_state:
//     "withheld"`). The brief endpoint applies that stricter content gate.
export function MeetingBriefAction({
  activity,
  onBriefMeeting,
  variant,
}: Readonly<{
  activity: Pick<Activity, "id" | "kind" | "content_state"> | undefined;
  onBriefMeeting?: (activityId: string) => void;
  // Filled for the next meeting: a reader is about to walk in, and the verb is
  // the card's lead. `aiQuiet` for a meeting already held, where the brief is
  // one of several equal ways to look back.
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

const MEETINGS_ONLY: TimelineFilters = { kind: "meeting" };

function calledOff(activity: Activity): boolean {
  return (
    activity.meeting_status === "canceled" ||
    activity.invitation_status === "canceled" ||
    activity.invitation_status === "canceling"
  );
}

// Which of the record's meetings lie ahead and which are behind. The clock is
// the read's own `as_of`, so a wrong computer clock files no meeting wrongly.
// The soonest booked one is drawn on its own and is not listed again.
function splitMeetings(
  listed: readonly Activity[],
  asOf: string,
  nextId: string | undefined,
): Readonly<{
  ahead: Activity[];
  held: Activity[];
  happened: Activity[];
  pastReached: boolean;
}> {
  const now = Date.parse(asOf);
  const at = (activity: Activity) => Date.parse(activity.occurred_at);
  // A meeting called off keeps its date but is never upcoming. It is listed
  // with what became of it.
  const isAhead = (activity: Activity) =>
    at(activity) > now && !calledOff(activity);
  const others = listed.filter((activity) => activity.id !== nextId);
  // Newest first: the row on top answers "what just happened".
  const held = others
    .filter((activity) => !isAhead(activity))
    .sort((a, b) => at(b) - at(a));
  return {
    ahead: others.filter(isAhead).sort((a, b) => at(a) - at(b)),
    held,
    // What the figures count as held: one called off keeps its row, unheld.
    happened: held.filter((activity) => !calledOff(activity)),
    // The list arrives newest first. Once a page reaches the past, every
    // meeting still ahead is on the pages read so far.
    pastReached: listed.some((activity) => at(activity) <= now),
  };
}

// The held list pages through the record's own meetings read. The composite
// read carries only a first page of every kind.
function heldState(
  view: Withholding | undefined,
  loading: boolean,
  listed: Listed,
  count: number,
): RailPanelState {
  if (!view) return loading ? "loading" : "unavailable";
  if (omitted(view, "activities")) return "withheld";
  if (count > 0) return "ready";
  if (listed.isError) return "failed";
  // A first page of meetings still ahead says nothing yet about those held:
  // the panel offers the next page rather than calling itself empty.
  if (listed.hasNextPage) return "ready";
  return listed.isPending ? "loading" : "empty";
}

/** The composite read a Meetings tab stands on: its clock and what it withheld. */
export type MeetingsView = Withholding & Readonly<{ as_of: string }>;

/**
 * RecordMeetings is a record's Meetings tab: its figures, then what is booked,
 * then what has been held. The booked meeting leads, from the server's own
 * next-meeting read. The lists are the record's activity read narrowed to
 * meetings. A company's tab so holds its contacts' meetings, as its History does.
 */
export function RecordMeetings({
  entity,
  recordId,
  view,
  loading,
  next,
  onBriefMeeting,
  toolbar,
  nextNote,
  nextVerbs,
  waiting,
}: Readonly<{
  entity: "contact" | "company";
  recordId: string | undefined;
  view?: MeetingsView;
  loading: boolean;
  next?: NextMeeting;
  onBriefMeeting?: (activityId: string) => void;
  // The record's own ways to start a meeting, under the figures.
  toolbar?: ReactNode;
  // Margince's line about why the next meeting is worth reading ahead of, and
  // the verbs that line offers beside the brief.
  nextNote?: Readonly<{ by: string; whyNow: string }>;
  nextVerbs?: ReactNode;
  // What sits between booked and held: invitations still waiting on an answer.
  waiting?: ReactNode;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const listed = useRecordTimeline(entity, recordId ?? "", {
    filters: MEETINGS_ONLY,
    enabled: view !== undefined && !omitted(view, "activities"),
  });
  const { ahead, held, happened, pastReached } = splitMeetings(
    listed.activities,
    view?.as_of ?? "",
    next?.activity_id,
  );
  const upcoming = sectionState(
    view,
    "next_meeting",
    Boolean(view),
    (next ? 1 : 0) + ahead.length,
    loading,
  );
  // An AI verb among equals on every row: outlined with its ✦, where the next
  // meeting leads with the filled one.
  const briefOf = (activity: Activity) => (
    <MeetingBriefAction
      activity={activity}
      onBriefMeeting={onBriefMeeting}
      variant="aiQuiet"
    />
  );
  const heldVerbs = (activity: Activity) => (
    <>
      {recordId && (
        <RecordMeetingOutcome
          activity={activity}
          invalidateKeys={entityTimelineKeys(entity, recordId)}
        />
      )}
      {briefOf(activity)}
    </>
  );
  const { withheld, ...timeline } = timelineRead(view, listed);
  const booked = bookedCount(upcoming, timeline, pastReached, [next, ...ahead]);
  const figured = view && (upcoming === "ready" || upcoming === "empty");
  return (
    <div className="record-stack meetingrows-tab">
      {(figured || toolbar) && (
        <div className="meetingrows-head">
          {figured && (
            <MeetingFigures
              next={next}
              booked={booked}
              held={timeline.isSuccess ? happened : null}
              heldMore={listed.hasNextPage}
              withheld={withheld}
              asOf={view.as_of}
            />
          )}
          {toolbar}
        </div>
      )}
      <Panel
        title={t("contact.meetings.upcoming")}
        titleAction={
          upcoming === "ready" &&
          booked !== null && <Badge>{formatNumber(booked, locale)}</Badge>
        }
      >
        {upcoming === "ready" ? (
          <>
            {view && next && (
              <NextMeetingLead
                next={next}
                asOf={view.as_of}
                listed={listed}
                note={nextNote}
                verbs={
                  <>
                    <MeetingBriefAction
                      // next_meeting has no content_state: the composite
                      // read withholds the whole section, never one row.
                      activity={{ id: next.activity_id, kind: "meeting" }}
                      onBriefMeeting={onBriefMeeting}
                      variant="ai"
                    />
                    {nextVerbs}
                  </>
                }
              />
            )}
            {ahead.map((activity) => (
              <MeetingRow
                key={activity.id}
                activity={activity}
                asOf={view?.as_of}
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
      {waiting}
      <RailPanel
        title={t("contact.meetings.past")}
        state={heldState(view, loading, listed, held.length)}
        emptyLabel={t("contact.meetings.noneLogged")}
        detail={{ onRetry: () => void listed.refetch() }}
        footer={listed.hasNextPage ? <LoadMoreButton query={listed} /> : null}
      >
        {held.length > 0 && <HeldMeetingsList held={held} verbs={heldVerbs} />}
      </RailPanel>
    </div>
  );
}

// The booked meeting, drawn large. The meeting's own row says whether it was
// booked through Margince. The record's list carries it unless a busy
// calendar pushed it past the first page; then it is read on its own.
function NextMeetingLead({
  next,
  asOf,
  listed,
  note,
  verbs,
}: Readonly<{
  next: NextMeeting;
  asOf: string;
  listed: Listed;
  note?: Readonly<{ by: string; whyNow: string }>;
  verbs: ReactNode;
}>) {
  const listedRow = listed.activities.find(
    (activity) => activity.id === next.activity_id,
  );
  const ownRow = useActivity(
    next.activity_id,
    !listedRow && listed.isSuccess && listed.hasNextPage,
  );
  return (
    <NextMeetingCard
      next={next}
      asOf={asOf}
      activity={listedRow ?? ownRow.data}
      note={note}
      verbs={verbs}
    />
  );
}
