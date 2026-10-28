import type { ReactNode } from "react";
import { useState } from "react";
import { useRoster } from "../../components/entityref";
import {
  hasTimelineFilters,
  useRecordTimeline,
} from "../../design-system/recordtimeline";
import type {
  TimelineEntry,
  TimelineGroup,
} from "../../design-system/timelineentry";
import { TimelineFilterBar } from "../../design-system/timelinefilterbar";
import { groupChronology } from "../../design-system/timelinegroups";
import { useLocale, useT } from "../../i18n";
import { useOpenEmail } from "../../lib/openemail";
import { useRecordZone } from "../../lib/recordzone";
import type { OpenReceipt } from "../records/record360";
import {
  ChronologyFilter,
  ChronologyFooter,
  chronologyNotice,
  useRecordChronology,
} from "../records/recordchronology";
import {
  ConversationList,
  useChronologyCut,
} from "../records/recordconversations";
import { TimelineActions } from "../records/timelineactions";
import type { Company, Company360View } from "./companies";
import { recordNamesIn } from "./company360";
import type { CitedRecord } from "./companyevidence";

// The four RecordView slots the chronology section fills: the list, the
// filter above it, the load-more and disclosure below it, and the notice.
// The notice replaces the list when nothing true can be drawn. Assembled here
// so the page's render reads as a layout, not four nested ternaries.
type ChronologySlots = {
  timeline?: TimelineEntry[];
  timelineGroups?: readonly TimelineGroup[];
  timelineHeader?: ReactNode;
  timelineFooter?: ReactNode;
  timelineNotice?: ReactNode;
  onOpenThread?: (threadKey: string) => void;
};

export function useChronologySlots({
  company,
  view,
  loading,
  failed,
  active,
}: Readonly<{
  company: Company;
  view?: Company360View;
  loading: boolean;
  failed: boolean;
  // Whether the chronology is on screen at all. The Partner tab is a form,
  // so it renders no timeline rather than an empty one.
  active: boolean;
}>): {
  slots: ChronologySlots;
  showChanges: () => void;
  // The open message and its setter leave this hook, so the drawer is not
  // mounted in a timeline slot. A record-level dialog belongs to the page, as
  // the audit modal's rule states. A drawer that unmounts with the Timeline tab
  // leaves its id behind, and returning could stack a second dialog.
  openEmail: string | null;
  setOpenEmail: (activityId: string | null) => void;
} {
  const t = useT();
  const { locale } = useLocale();
  const recordZone = useRecordZone();
  // The workspace roster, for the ids a change row stores. Still walks rather
  // than reading useMemberNames by id: `colleagueName` becomes useRecordChronology's
  // `values.nameOf`, called per row as it renders, so the ids are unknown here.
  // roster-failure: names change rows only; nothing here is picked.
  const roster = useRoster("user", true);
  const colleagues = new Map(
    (roster.data ?? []).flatMap((entry) =>
      "display_name" in entry ? [[entry.id, entry.display_name] as const] : [],
    ),
  );
  // One resolver for every id the chronology can meet: a colleague from the
  // workspace roster, or one of the account's records from its 360. Keyed by
  // id alone: a stored value carries no type (an owner field holds a uuid).
  // The two id spaces do not collide.
  const records = recordNamesIn(view);
  const colleagueName = (id: string) =>
    colleagues.get(id) ??
    records("contact", id) ??
    records("deal", id) ??
    records("company", id);
  const { filter, filters, setFilters, openCut, kinds } = useChronologyCut(
    company.id,
  );
  // The 360's own page seeds the list; older pages and every narrowed read
  // come from the activity list itself.
  const timeline = useRecordTimeline("company", company.id, {
    filters,
    firstPage: view?.activities,
  });

  const [openEmail, setOpenEmail] = useOpenEmail();
  const history = useRecordChronology({
    onOpenEmail: setOpenEmail,
    kind: "company",
    recordId: company.id,
    filter,
    activities: timeline.activities,
    activitiesHaveMore: timeline.hasNextPage,
    loadMore: timeline,
    // What a stored value means, not the shape it is kept in. A date reads
    // at the record's zone, a list as its words, a colleague's id as a name.
    // The resolver reaches the workspace roster because change rows carry our
    // ids. An owner moving between colleagues is the change most often sought.
    // An account holds no currency of its own. A minor-unit column on it says
    // so, rather than printing a bare integer under a currency it never had.
    values: { currency: null, locale, zone: recordZone, nameOf: colleagueName },
    renderActions: (activity) => (
      <TimelineActions
        activity={activity}
        entityType="company"
        entityId={company.id}
      />
    ),
  });
  // An evidence mark asks "where did this value come from", and the answer is
  // the change history. The mark turns the timeline to Changes through the
  // opener a pill press uses, so one set of rules opens a cut.
  const showChanges = () => openCut("changes");

  if (!active) {
    return {
      slots: { timelineNotice: <span /> },
      showChanges,
      openEmail,
      setOpenEmail,
    };
  }
  return {
    showChanges,
    openEmail,
    setOpenEmail,
    slots: {
      timeline: history.entries,
      // Conversations, not messages. Ungrouped, a product update to three
      // contacts is three rows, and a five-message thread is five.
      timelineGroups: groupChronology(history.entries, timeline.hasNextPage),
      timelineHeader: (
        <>
          <ChronologyFilter filter={filter} conversations onFilter={openCut} />
          {filter !== "changes" && (
            <TimelineFilterBar
              value={filters}
              kinds={kinds}
              onChange={setFilters}
            />
          )}
        </>
      ),
      timelineFooter: <ChronologyFooter filter={filter} chronology={history} />,
      // Every cut renders through the one chronicle, so no cut is a second
      // rendering of rows another cut shows. The By-field reading and the
      // put-back control live in the record's Full history (the header's
      // overflow menu), the one surface that carries the restore write.
      timelineNotice:
        chronologyNotice(
          filter === "conversations"
            ? "chronology.conversationsEmpty"
            : "co.timeline.empty",
          filter,
          history,
          {
            loading: loading || timeline.isPending,
            failed: failed || timeline.isError,
            // A narrowed read is the list's own and is assembled once it
            // answers. So is an archived account's, whose 360 is refused.
            assembled:
              hasTimelineFilters(filters) || company.archived_at
                ? timeline.isSuccess
                : Boolean(view?.activities),
          },
          t,
        ) ??
        // The Conversations cut narrows the chronicle to the exchanges
        // somebody can answer. It uses the grouped list every other cut uses,
        // standing where the unfiltered list would.
        (filter === "conversations" ? (
          <ConversationList
            groups={groupChronology(history.entries, timeline.hasNextPage)}
            zone={recordZone}
          />
        ) : undefined),
    },
  };
}

// The receipt drawer's state: which claim is open, and the ordered list it
// steps through.
//
// The order belongs to the card that offered the chip, not to the drawer. A
// reader who clicked a sentence's third citation expects "next" to be its
// fourth. A drawer with its own order would step somewhere unpredictable. A
// sentence with one receipt has nowhere to step, so the drawer draws no arrows.
export function useCitedReceipt() {
  const [cited, setCited] = useState<CitedRecord | null>(null);
  const [list, setList] = useState<readonly CitedRecord[]>([]);
  // Beside the receipt, because both answer "what did this chip open" and a
  // caller should not wire two states for one question.
  const [email, setEmail] = useState<string | null>(null);
  const open: OpenReceipt = (receipt, siblings) => {
    setCited(receipt);
    setList(siblings);
  };
  // Wraps at each end, so a reader walking a sentence's citations never has to
  // close the drawer to reach the first one again.
  const step = (direction: -1 | 1) => {
    if (!cited) {
      return;
    }
    const at = list.findIndex(
      (each) =>
        each.entityType === cited.entityType &&
        each.entityId === cited.entityId,
    );
    if (at < 0) {
      return;
    }
    const next = list[(at + direction + list.length) % list.length];
    if (next === undefined) {
      return;
    }
    setCited(next);
  };
  return {
    cited,
    email,
    open,
    openEmail: setEmail,
    close: () => setCited(null),
    closeEmail: () => setEmail(null),
    step: list.length > 1 ? step : undefined,
  };
}
