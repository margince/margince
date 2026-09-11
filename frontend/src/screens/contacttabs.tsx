import { useQueryClient } from "@tanstack/react-query";
import type { components } from "../api/schema";
import { useCanWrite } from "../app/capability";
import { useRecordZone } from "../app/recordzone";
import { navigate } from "../app/router";
import { activityTimeline } from "../design-system/activitytimeline";
import { Avatar, Button } from "../design-system/atoms";
import { GroupedTimelineList } from "../design-system/composed";
import { Eyebrow } from "../design-system/eyebrow";
import { Panel, PanelBody, PanelRow } from "../design-system/panel";
import {
  hasTimelineFilters,
  useRecordTimeline,
} from "../design-system/recordtimeline";
import { SurfaceState, sectionState } from "../design-system/surfacestate";
import { TimelineFilterBar } from "../design-system/timelinefilterbar";
import { formatDateTime } from "../format/format";
import { useLocale, useT } from "../i18n";
import { useViewerId } from "./common";
import { ContactCommercialCard, readableRole } from "./contactcards";
import { timelineState } from "./contacttimelinestate";
import { RecordHistoryTab } from "./history";
import {
  CHRONOLOGY_EMPTY_KEYS,
  ChronologyFilter,
  ChronologyFooter,
  hasChronologyFooter,
  readsExchangesOnly,
  useRecordChronology,
} from "./recordchronology";
import { ConversationList, useChronologyCut } from "./recordconversations";
import { AddRelationshipAction } from "./relationships";
import { TimelineActions } from "./timelineactions";
import { groupChronology } from "./timelinegroups";
import "./contact360.css";
import { invalidateRecord } from "./recordwritekeys";

// The tabs beside Overview that read what the 360 already assembled — the same
// rule the overview cards hold to (contactcards.tsx): a tab can never show a
// record the tab beside it is withholding. The Timeline's CHANGES half and the
// Files tab are the exceptions, and they fetch because the 360 carries
// neither.

type Contact360 = components["schemas"]["Contact360"];
type Activity = components["schemas"]["Activity"];

// --- Timeline ---------------------------------------------------------------

/**
 * ContactTimelineTab is the contact's ONE chronology: what was said to them and
 * what was changed about them, in one order, through the same
 * `useRecordChronology` the account page reads. They were two tabs for a
 * release, and a reader who wanted them in order had to interleave two lists
 * by hand.
 *
 * The activities half is the 360's own section — already fetched, and a PAGE:
 * when the server says there is more, the tab says so rather than letting a
 * cut list read as the whole ledger. The changes half is fetched here, and
 * only once the reader asks for it.
 */
export function ContactTimelineTab({
  contactId,
  view,
  loading = false,
  onBriefMeeting,
  onOpenEmail,
}: Readonly<{
  contactId: string;
  /** Opens one message in the record's drawer, which the page owns. */
  onOpenEmail?: (activityId: string) => void;
  view?: Contact360;
  loading?: boolean;
  // Opens the pre-meeting brief for one meeting row. The drawer lives on the
  // page, so the tab asks rather than renders it.
  onBriefMeeting?: (activityId: string) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const queryClient = useQueryClient();
  const recordZone = useRecordZone();
  const { filter, filters, setFilters, openCut, kinds } =
    useChronologyCut(contactId);
  // Every cut renders through the ONE chronicle, off the same groups: the
  // Conversations list is the same rows cut to the exchanges somebody can
  // answer, never a second rendering of what another cut already shows.
  const List =
    filter === "conversations" ? ConversationList : GroupedTimelineList;
  // The 360's own page seeds the list; older pages and every narrowed read
  // come from the activity list itself.
  const timeline = useRecordTimeline("contact", contactId, {
    filters,
    firstPage: view?.activities,
  });
  const chronology = useRecordChronology({
    onOpenEmail,
    kind: "contact",
    recordId: contactId,
    filter,
    // A narrowed read is a question about what was said, so the record's own
    // edits stand down: they are not meetings, and not what the reader asked.
    narrowed: hasTimelineFilters(filters),
    activities: timeline.activities,
    activitiesHaveMore: timeline.hasNextPage,
    loadMore: timeline,
    // No name resolver yet: a uuid in a change row still renders untouched
    // rather than guessed at, which is the correct fallback while nothing
    // here resolves contacts/company ids to names.
    // What a stored value needs to be read as what it MEANS: this record holds
    // no money of its own, so the currency is absent and a minor-unit column
    // says so rather than printing a bare integer.
    values: { currency: null, locale, zone: recordZone },
    renderActions: (activity) => (
      <TimelineActions
        activity={activity}
        entityType="contact"
        entityId={contactId}
        contactId={contactId}
        extra={(row) => (
          <MeetingBriefAction activity={row} onBriefMeeting={onBriefMeeting} />
        )}
      />
    ),
  });
  return (
    <Panel
      title={t("tab.timeline")}
      actions={
        hasChronologyFooter(filter, chronology) ? (
          <ChronologyFooter filter={filter} chronology={chronology} />
        ) : undefined
      }
    >
      <PanelBody>
        {/* Both sets of dials under the head, in the one block the account and
            the project pages already put them in (`timeline-header`): the cuts
            through the chronology, then the narrowing of whichever cut is open.
            The pills wrap to as many rows as the column needs, which the head's
            single band cannot hold — and the two rows of controls read as one
            block here rather than as a head that grew. */}
        <div className="timeline-header">
          <ChronologyFilter filter={filter} conversations onFilter={openCut} />
          {filter !== "changes" && (
            <TimelineFilterBar
              value={filters}
              kinds={kinds}
              onChange={setFilters}
            />
          )}
        </div>
        {/* The Changes view IS the record's history: one reading of what
            changed on this record, and the one that can put a change back. A
            second rendering of the same audit rows beside it would be two
            answers to one question, and only one of them would ever carry the
            control. */}
        {filter === "changes" ? (
          <RecordHistoryTab
            kind="contact"
            id={contactId}
            restore={{
              // Handed over whether or not it is there. `RecordRestore.version`
              // is optional and the panel withholds the button when it is
              // absent, so a second guard here would be the same policy in two
              // places — and the company surface, which does not repeat it,
              // would be the one that looked wrong.
              version: view?.contact.version,
              onRestored: () =>
                invalidateRecord(queryClient, "contact", contactId),
            }}
          />
        ) : (
          <SurfaceState
            loadingLabel={t("tab.timeline")}
            state={timelineState(
              view,
              filter,
              chronology,
              timeline,
              hasTimelineFilters(filters),
              loading,
            )}
            emptyLabel={
              filter === "activities"
                ? t("contact.timeline.empty")
                : t(CHRONOLOGY_EMPTY_KEYS[filter])
            }
            // Retrying the CHANGE feed, on the cuts that read one: a cut that
            // never asked it a question would offer a retry for a read that is
            // not the one that came up short.
            detail={
              readsExchangesOnly(filter)
                ? undefined
                : { onRetry: chronology.changes.refetch }
            }
          >
            {/* Half the chronology is missing and the other half is right
                  here. Taking the exchanges away because the change feed fell
                  over would serve nobody, and leaving the reader to take a
                  partial record for a complete one is the failure this line
                  exists to prevent. Above the rows, because a caveat under a
                  list is read after the list it qualifies. */}
            {chronology.changesUnread && (
              <p className="t-caption">{t("state.failed")}</p>
            )}
            <List
              groups={groupChronology(chronology.entries, timeline.hasNextPage)}
              zone={recordZone}
            />
          </SurfaceState>
        )}
      </PanelBody>
    </Panel>
  );
}

// --- Deals ------------------------------------------------------------------

/**
 * ContactDealsTab answers two different questions and keeps them apart: which
 * deals this contact is recorded on at all, and what the one that matters looks
 * like right now.
 *
 * The second half is the overview's own commercial card, rendered again rather
 * than re-spelled — a second wording of the same figures is how two surfaces
 * start disagreeing about one deal.
 */
export function ContactDealsTab({
  view,
  loading = false,
}: Readonly<{ view?: Contact360; loading?: boolean }>) {
  const t = useT();
  // The object half of the gate, asked as the server asks it. The row half —
  // whether this caller may write THIS contact — is the anchor's own, and the
  // server applies it to the write; a tab that second-guessed it here would
  // withhold the verb on a record the write would have accepted.
  const canSeat = useCanWrite("relationship", "create");
  const roles = view?.deal_roles?.data ?? [];
  const state = sectionState(
    view,
    "deal_roles",
    Boolean(view?.deal_roles),
    roles.length,
    loading,
  );
  return (
    <div className="record-stack">
      <Panel
        title={t("tab.deals")}
        // THE TAB'S OWN VERB, and the same one the relationships tab offers —
        // reached from where a reader is already asking the question rather
        // than spelled a second time. Narrowed to the deal edge: a kind
        // selector offering "employment" on a Deals tab would ask the reader to
        // answer what the tab already answered.
        //
        // Withheld outright without the grant, as the relationships tab does:
        // there is no fact about this contact to report, so a disabled button
        // would be an affordance that says nothing.
        titleAction={
          view?.contact.id && canSeat ? (
            <AddRelationshipAction
              scope={{ contact_id: view.contact.id }}
              only={{ kind: "deal_stakeholder", label: "rel.seatOnDeal" }}
            />
          ) : undefined
        }
      >
        <SurfaceState
          state={state}
          emptyLabel={t("contact.deals.empty")}
          loadingLabel={t("tab.deals")}
        >
          {roles.map((role) => (
            <PanelRow className="pe-row" key={role.relationship_id}>
              {/* The seat first, in the row grid's own label column: which
                  deal it is answers "where", and the seat answers the question
                  a reader opened this tab with — what am I to them there. */}
              <span className="pe-row-label">{readableRole(role.role)}</span>
              <span className="pe-row-value">
                <button
                  type="button"
                  className="link-button"
                  onClick={() =>
                    navigate({ screen: "deals", id: role.deal_id })
                  }
                >
                  {role.deal_title ?? t("contact.deals.untitled")}
                </button>
              </span>
              <span className="pe-row-label">
                {role.deal_stage ?? t("contact.deals.noStage")}
              </span>
            </PanelRow>
          ))}
        </SurfaceState>
      </Panel>
      {view && <ContactCommercialCard view={view} />}
    </div>
  );
}

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
function MeetingBriefAction({
  activity,
  onBriefMeeting,
}: Readonly<{
  activity: Pick<Activity, "id" | "kind" | "content_state"> | undefined;
  onBriefMeeting?: (activityId: string) => void;
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
    <Button small onClick={() => onBriefMeeting(activityId)}>
      {t("contact.meeting.brief")}
    </Button>
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
}: Readonly<{
  view?: Contact360;
  loading?: boolean;
  onBriefMeeting?: (activityId: string) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const recordZone = useRecordZone();
  const viewerId = useViewerId();
  // The booked meeting is drawn above, from the server's own next-meeting
  // read. It is also an activity, so an unfiltered list draws it a second time
  // under "already held" — which was merely untidy while the rows were inert
  // and becomes two identical brief buttons for one room now that they carry a
  // verb.
  const booked = view?.next_meeting?.activity_id;
  const met = (view?.activities?.data ?? []).filter(
    (activity: Activity) =>
      activity.kind === "meeting" && activity.id !== booked,
  );
  const hasMore = view?.activities?.page.has_more ?? false;
  const past = sectionState(
    view,
    "activities",
    Boolean(view?.activities),
    met.length,
    loading,
  );
  const next = view?.next_meeting;
  return (
    <div className="record-stack">
      <Panel title={t("contact.meetings.next")}>
        <PanelBody>
          <SurfaceState
            loadingLabel={t("contact.meetings.next")}
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
              <>
                <p className="pe-prose t-body">
                  {next.subject ?? t("contact.meetings.untitled")}
                </p>
                <p className="pe-brief-line">
                  {formatDateTime(next.starts_at, locale, recordZone)}
                </p>
                {/* next_meeting carries no content_state because the 360
                    withholds the whole section rather than a redacted row, so
                    a booked meeting the reader can see here is one they can
                    read. The kind is stated for the same reason: this section
                    IS the meeting. */}
                <MeetingBriefAction
                  activity={{ id: next.activity_id, kind: "meeting" }}
                  onBriefMeeting={onBriefMeeting}
                />
                {next.participants && next.participants.length > 0 && (
                  <>
                    <Eyebrow as="h3">
                      {t("contact.meetings.participants")}
                    </Eyebrow>
                    <div className="pe-chiprow">
                      {next.participants.map((who) => (
                        <span
                          className="pe-memory-channel t-caption"
                          key={who.contact_id}
                        >
                          <Avatar
                            name={who.full_name}
                            identity={who.contact_id}
                            size="xs"
                          />
                          {who.full_name}
                        </span>
                      ))}
                    </div>
                  </>
                )}
              </>
            )}
          </SurfaceState>
        </PanelBody>
      </Panel>
      <Panel title={t("contact.meetings.past")}>
        <PanelBody>
          <SurfaceState
            loadingLabel={t("contact.meetings.past")}
            state={past === "ready" && hasMore ? "partial" : past}
            emptyLabel={t("contact.meetings.noneLogged")}
          >
            <GroupedTimelineList
              groups={groupChronology(
                activityTimeline(met, viewerId, (activity) => (
                  <MeetingBriefAction
                    activity={activity}
                    onBriefMeeting={onBriefMeeting}
                  />
                )),
                hasMore,
              )}
              zone={recordZone}
            />
          </SurfaceState>
        </PanelBody>
      </Panel>
    </div>
  );
}
