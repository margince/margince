import { useQueryClient } from "@tanstack/react-query";
import type { components } from "../../api/schema";
import { LatestInboundPanel } from "../../components/latestinbound";
import { CallAction } from "../../design-system/callaction";
import { Panel, PanelBody } from "../../design-system/panel";
import {
  hasTimelineFilters,
  useRecordTimeline,
} from "../../design-system/recordtimeline";
import { SurfaceState } from "../../design-system/surfacestate";
import { GroupedTimelineList } from "../../design-system/timeline";
import { TimelineFilterBar } from "../../design-system/timelinefilterbar";
import { groupChronology } from "../../design-system/timelinegroups";
import { useLocale, useT } from "../../i18n";
import type { LatestInbound } from "../../lib/latestinbound";
import { useRecordZone } from "../../lib/recordzone";
import { RecordHistoryTab } from "../records/history";
import {
  CHRONOLOGY_EMPTY_KEYS,
  ChronologyFilter,
  ChronologyFooter,
  hasChronologyFooter,
  useRecordChronology,
} from "../records/recordchronology";
import {
  ConversationList,
  useChronologyCut,
} from "../records/recordconversations";
import { RowAnswerActions, TimelineActions } from "../records/timelineactions";
import { MeetingBriefAction } from "./contactmeetings";
import { callBackNumber } from "./contactmemory.rows";
import { timelineState } from "./contacttimelinestate";
import "./contact360.css";
import { invalidateRecord } from "../../lib/recordwritekeys";

// The tabs beside Overview that read what the 360 already assembled — the same
// rule the overview cards hold to (contactcards.tsx): a tab can never show a
// record the tab beside it is withholding. The Timeline's CHANGES half and the
// Files tab are the exceptions, and they fetch because the 360 carries
// neither.

type Contact360 = components["schemas"]["Contact360"];

// --- Timeline ---------------------------------------------------------------

type Activity = components["schemas"]["Activity"];

// The verbs one of the contact's activities offers, wherever it is drawn. That
// is its timeline row, and the "Just in" panel when it is the newest one in.
// The panel only answers, so `answerOnly` leaves the filing verbs to the row.
function ContactRowActions({
  activity,
  contactId,
  view,
  onBriefMeeting,
  answerOnly = false,
}: Readonly<{
  activity: Activity;
  contactId: string;
  view?: Contact360;
  onBriefMeeting?: (activityId: string) => void;
  answerOnly?: boolean;
}>) {
  const t = useT();
  const Verbs = answerOnly ? RowAnswerActions : TimelineActions;
  return (
    <Verbs
      activity={activity}
      entityType="contact"
      entityId={contactId}
      contactId={contactId}
      extra={(row) => (
        <>
          <MeetingBriefAction activity={row} onBriefMeeting={onBriefMeeting} />
          <CallAction
            appearance="button"
            label={t("record.verb.call")}
            number={callBackNumber(row.kind, view?.contact)}
            record={{ entityType: "contact", entityId: contactId }}
          />
        </>
      )}
    />
  );
}

/** The newest unanswered thing the contact sent, leading the Overview. */
export function ContactLatestInbound({
  latest,
  contactId,
  view,
}: Readonly<{
  latest: LatestInbound | undefined;
  contactId: string;
  view: Contact360;
}>) {
  return (
    <LatestInboundPanel
      latest={latest}
      actions={(activity) => (
        <ContactRowActions
          activity={activity}
          contactId={contactId}
          view={view}
          answerOnly
        />
      )}
    />
  );
}

/**
 * ContactTimelineTab is the contact's History, read through the same
 * `useRecordChronology` every record page uses. All and Threads show what was
 * said to them, and Changes what was changed about them.
 *
 * The exchanges are the 360's own section, already fetched, and one page of
 * it. When the server says there is more, the tab says so, so a cut list does
 * not read as the whole ledger.
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
    // RecordHistoryTab draws Changes here and pages its own feed.
    changesInPanel: true,
    activities: timeline.activities,
    activitiesHaveMore: timeline.hasNextPage,
    loadMore: timeline,
    // What a stored value needs to be read as what it means. This record holds
    // no money of its own, so the currency is absent and a minor-unit column
    // says so instead of printing an integer. No name resolver: a mail filed
    // here may have been sent by somebody else. A resolved link name would
    // hide that sender behind this contact's own.
    values: { currency: null, locale, zone: recordZone },
    renderActions: (activity) => (
      <ContactRowActions
        activity={activity}
        contactId={contactId}
        view={view}
        onBriefMeeting={onBriefMeeting}
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
              chronology,
              timeline,
              hasTimelineFilters(filters),
              loading,
            )}
            emptyLabel={
              filter === "all"
                ? t("contact.timeline.empty")
                : t(CHRONOLOGY_EMPTY_KEYS[filter])
            }
          >
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
