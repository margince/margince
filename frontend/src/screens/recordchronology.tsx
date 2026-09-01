import type { ReactNode } from "react";
import { useState } from "react";
import type { components } from "../api/schema";
import type { EntityKind } from "../app/entity";
import { activityTimeline } from "../design-system/activitytimeline";
import { EmptyState, Skeleton } from "../design-system/atoms";
import type { TimelineEntry } from "../design-system/composed";
import { FilterPills } from "../design-system/filterpills";
import type { RecordTimeline } from "../design-system/recordtimeline";
import { useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { coldFieldLabel, LoadMoreButton, useViewerId } from "./common";
import { changeTimeline, useFieldHistory } from "./history";
import { mergeChronology } from "./history.logic";
import type { HistoryValueCtx } from "./historyvalues";

// A record has ONE chronology, and this is where it is assembled — for any
// record, not for the account page alone. What was said to a record and what
// was changed about it are one story to the person reading them: kept apart,
// a reader comparing "we told them X" against "someone set stage to Y" had to
// hold two orderings in their head.
//
// It lives here rather than in the account page because the contact page asks
// the same question of the same two feeds. A second copy would answer it
// slightly differently the first time either half moved.

type Activity = components["schemas"]["Activity"];
type ChangesQuery = ReturnType<typeof useFieldHistory>;

// All first: it is where a reader starts, and a row of cuts reads left to
// right from the whole to its parts. `conversations` is in the vocabulary
// but not in the base row — a page offers it through `ChronologyFilter`'s
// own flag once it wires a renderer for the cut, because a pill whose press
// changes nothing teaches the reader the row is broken.
export const TIMELINE_FILTERS = ["all", "activities", "changes"] as const;
export type TimelineFilter =
  | (typeof TIMELINE_FILTERS)[number]
  | "conversations";

/**
 * useChronologyFilter owns the filter for ONE record rather than for the
 * session. When both records are already cached the route swaps one for
 * another without ever unmounting the section, so a reader who checked
 * Changes once met Changes on every record afterwards.
 */
export function useChronologyFilter(
  recordId: string,
): [TimelineFilter, (next: TimelineFilter) => void] {
  // ALL by default. The record's history is read to find out what happened,
  // and a default that hid every field change answered a narrower question
  // than the one the reader opened the tab with — they had to know a cut
  // existed before they could see the whole.
  const [filter, setFilter] = useState<TimelineFilter>("all");
  const [filterFor, setFilterFor] = useState(recordId);
  if (filterFor !== recordId) {
    setFilterFor(recordId);
    setFilter("all");
  }
  return [filter, setFilter];
}

/**
 * ChronologyFilter narrows the record's own history. It sits ABOVE the list
 * rather than in the page's tab strip: it scopes this section, and a control
 * that looks like a tab reads as a different page.
 */
export function ChronologyFilter({
  filter,
  conversations = false,
  onFilter,
}: Readonly<{
  filter: TimelineFilter;
  // Whether this page offers the Conversations cut — the mail and message
  // threads alone, drawn as conversations rather than as chronicle rows.
  // Opt-in per page: the pill only stands where the page renders the cut.
  conversations?: boolean;
  // Whether the caller has narrowed the exchanges — by kind, by words, by a
  // date range. A narrowed read is a question about what was SAID, and a field
  // edit is not a meeting: leaving the changes in answered a question nobody
  // asked, and the reader who picked Meetings got a list of record edits with
  // two meetings in it.
  //
  // It also stops the change read from being made at all, which is the honest
  // consequence: a feed whose rows cannot appear should not be fetched.
  narrowed?: boolean;
  onFilter: (next: TimelineFilter) => void;
}>) {
  const t = useT();
  const labels: Record<TimelineFilter, string> = {
    all: t("chronology.all"),
    conversations: t("chronology.conversations"),
    activities: t("chronology.activities"),
    changes: t("chronology.changes"),
  };
  // Conversations sits between the whole and the parts: it is a READING of
  // the exchanges, narrower than All and wider than one kind.
  const cuts: readonly TimelineFilter[] = conversations
    ? ["all", "conversations", "activities", "changes"]
    : TIMELINE_FILTERS;
  return (
    <FilterPills
      pills={cuts.map((value) => ({
        value,
        label: labels[value],
        // No counts. Each cut is its own paged read, so this page knows a
        // floor rather than a total — and a floor printed as a count is a
        // wrong number where a missing one is merely a missing one.
        count: undefined,
      }))}
      value={filter}
      onChange={onFilter}
      label={t("chronology.label")}
    />
  );
}

export type RecordChronology = {
  entries: TimelineEntry[];
  truncated: boolean;
  changes: ChangesQuery;
  // What the CURRENT filter is waiting on or failed at. A query that is
  // switched off never resolves — it reports pending forever — so the caller
  // must not read the query's own flags. Reading them turned the default
  // Activities view into a skeleton that never became a timeline.
  loading: boolean;
  failed: boolean;
  // The CHANGE read errored, whether or not the section as a whole is drawn as
  // failed. On the combined cut the exchanges that DID load stay on screen —
  // taking them away because a second feed fell over serves nobody — but the
  // reader still has to be told that half the chronology is missing, or they
  // read a partial record as a complete one.
  changesUnread: boolean;
  // Whether fetching more CHANGES would lengthen the merged view. When the
  // activity feed is the shorter of the two, it is not: the merge cuts at its
  // oldest row and every extra change page falls below that line.
  changesAreTheLimit: boolean;
  // The activity feed's own next page, when the caller can fetch one. Under
  // "all" it is offered exactly when the changes are NOT the limit — the
  // activity feed is the cut, so another page of activities lengthens the
  // merged view and another page of changes would not.
  activities?: RecordTimeline;
};

/**
 * useRecordChronology reads the change feed for the record and folds it in
 * with the activities the caller already holds. The activities arrive as a
 * prop because the composite record read (the 360) has already fetched them:
 * a second fetch here would show the reader a different moment in the two
 * halves of one list.
 */
export function useRecordChronology({
  kind,
  recordId,
  filter,
  narrowed = false,
  activities,
  activitiesHaveMore,
  loadMore,
  renderActions,
  values,
}: Readonly<{
  kind: EntityKind;
  recordId: string;
  filter: TimelineFilter;
  // Whether the caller has narrowed the exchanges — by kind, by words, by a
  // date range. A narrowed read is a question about what was SAID, and a field
  // edit is not a meeting: left in, the reader who picked Meetings got a list
  // of record edits with two meetings in it.
  //
  // It also stops the change feed from being READ, which is the honest
  // consequence: rows that cannot appear should not be fetched.
  narrowed?: boolean;
  activities: Activity[];
  activitiesHaveMore: boolean;
  // The paged read behind `activities`, for the footer's Load more. Absent on
  // a surface that shows one page and says so.
  loadMore?: RecordTimeline;
  // The per-row verbs (Reply, Relink). Absent on a surface that offers none.
  renderActions?: (activity: Activity) => ReactNode;
  // Everything a stored value needs to be read as what it MEANS rather than as
  // the shape it is kept in: the record's currency for a minor-unit column, the
  // record's zone for a timestamp, and a resolver for the ids a change row
  // holds. One object because they travel together — a row that scaled its
  // money and still printed a uuid would be half-read.
  values: HistoryValueCtx;
}>): RecordChronology {
  const t = useT();
  const viewerId = useViewerId();
  // Only the cuts that DRAW change rows read them: Conversations is about
  // what was said, exactly as a narrowed read is.
  const wantsChanges = (filter === "all" || filter === "changes") && !narrowed;
  const changes = useFieldHistory(kind, recordId, { enabled: wantsChanges });
  // `page.data ?? []`, not `page.data`: a 200 with no body is a shape the
  // contract permits and the overlay mirror actually returns, and flattening
  // it yielded an `undefined` row that the mapper below dereferenced. The
  // activity timeline has guarded this since the same payload crashed it; the
  // change list only started meeting it now that ALL is the default filter and
  // every record page reads changes on open.
  const changeRows =
    changes.data?.pages.flatMap((page) => page.data ?? []) ?? [];
  // The people on each exchange, named through the same resolver the change
  // rows use for their stored ids. One resolver for both feeds, because a
  // chronology that named a person on a mail and not on the field edit beside
  // it would look like two different lists.
  const activityEntries = activityTimeline(
    activities,
    viewerId,
    renderActions,
    values.nameOf
      ? {
          nameOf: (entityType, entityId) =>
            entityType === "person" ? values.nameOf?.(entityId) : undefined,
          t,
          locale: values.locale,
        }
      : undefined,
  );
  const changeEntries = changeTimeline(
    changeRows,
    (field) => coldFieldLabel(field, t),
    values,
    t("timeline.fieldUpdated"),
    viewerId,
  );
  // Loading means NOTHING IS ON SCREEN YET, not "one of the two reads is still
  // out". On the combined view the activities usually arrive first — the 360
  // seeds them — and blanking them behind a skeleton until the change history
  // lands takes rows away from a reader who already had them. It matters now
  // that ALL is the default: every record page would open on a skeleton and
  // then fill in.
  // Rows already on screen are what makes a second feed's wait bearable, and
  // only the COMBINED cut has any: on the changes cut the change feed is the
  // whole list, so its wait and its failure are the section's own however many
  // exchanges the record holds.
  const holdingRows = filter === "all" && activityEntries.length > 0;
  const loading = wantsChanges && changes.isPending && !holdingRows;
  // Failure is judged the same way, and for the same reason: on the combined
  // view a change history that could not be read must not erase the exchanges
  // that WERE read. A reader who can see the conversation is better served by
  // it than by a page saying the whole chronology is unavailable when half of
  // it is on screen.
  //
  // The half that failed still goes unreported here, which is the gap worth
  // naming: the notice belongs beside the rows rather than instead of them,
  // and that is a change to what the footer says rather than to this test.
  const failed = wantsChanges && changes.isError && !holdingRows;

  // Conversations reads the same feed as Activities — the exchanges — and
  // its renderer, not this hook, is what narrows the rows to threads.
  if (filter === "activities" || filter === "conversations") {
    return {
      entries: activityEntries,
      // The composite read caps this section, and a capped list that says
      // nothing reads as the whole history: a reader looking at the oldest of
      // 25 rows would take it for the day the relationship began.
      truncated: activitiesHaveMore,
      changes,
      loading: false,
      failed: false,
      // The Activities cut reads no changes, so there is no failure of theirs
      // to report on it.
      changesUnread: false,
      changesAreTheLimit: false,
      activities: loadMore,
    };
  }
  if (filter === "changes") {
    return {
      entries: changeEntries,
      truncated: false,
      changes,
      loading,
      failed,
      changesUnread: false,
      changesAreTheLimit: changes.hasNextPage,
      activities: undefined,
    };
  }
  // The change feed joins the merge only once it has ANSWERED. A feed that has
  // more rows and has loaded none is blind — its newest row is unknown, so no
  // part of the merge is provably ordered and mergeChronology correctly
  // returns nothing. But a read still in flight, or one that failed, is not a
  // feed with more rows: it is a feed with no rows yet, and treating the two
  // alike blanked the whole chronology on every record open once ALL became
  // the default.
  //
  // The exchanges still say they are cut, because they are.
  const changesAnswered =
    wantsChanges && !changes.isPending && !changes.isError;
  const merged = mergeChronology<TimelineEntry>(
    changesAnswered
      ? [
          { rows: activityEntries, hasMore: activitiesHaveMore },
          { rows: changeEntries, hasMore: changes.hasNextPage },
        ]
      : [{ rows: activityEntries, hasMore: activitiesHaveMore }],
    (entry) => entry.atIso,
  );
  return {
    entries: merged.rows,
    truncated: merged.truncated,
    changes,
    loading,
    failed,
    changesUnread: wantsChanges && changes.isError,
    changesAreTheLimit: changesOwnTheCut(
      changes.hasNextPage,
      activitiesHaveMore,
      changeEntries,
      activityEntries,
    ),
    activities: loadMore,
  };
}

/**
 * hasChronologyFooter says whether the footer would draw anything at all. A
 * caller that hangs it in a band of its own asks first: an empty band is a
 * strip of chrome the reader sees and cannot use.
 */
export function hasChronologyFooter(
  filter: TimelineFilter,
  chronology: RecordChronology,
): boolean {
  return (
    chronology.truncated ||
    filter === "changes" ||
    (filter === "all" && chronology.changesAreTheLimit) ||
    activitiesCanGrow(filter, chronology)
  );
}

/**
 * ChronologyFooter is what the list owes the reader underneath it: where the
 * merged view stops being complete, and the one button that can lengthen it.
 * Silence here would read as the end of the record's history.
 */
export function ChronologyFooter({
  filter,
  chronology,
}: Readonly<{ filter: TimelineFilter; chronology: RecordChronology }>) {
  const t = useT();
  return (
    <>
      {chronology.truncated && (
        <p className="t-small">
          {t(
            filter === "activities"
              ? "chronology.truncatedActivities"
              : "chronology.truncated",
          )}
        </p>
      )}
      {/* Only where fetching more changes actually lengthens the list. Under
          "all" the merge is cut at whichever feed is shorter, so if the
          ACTIVITY feed is the constraint, another page of changes is filtered
          straight back out and the button does nothing. */}
      {(filter === "changes" ||
        (filter === "all" && chronology.changesAreTheLimit)) && (
        <LoadMoreButton query={chronology.changes} />
      )}
      {activitiesCanGrow(filter, chronology) && chronology.activities && (
        <LoadMoreButton query={chronology.activities} />
      )}
    </>
  );
}

// Another page of ACTIVITIES lengthens the list under Activities whenever the
// server holds one, and under All only while the activity feed owns the cut.
function activitiesCanGrow(
  filter: TimelineFilter,
  chronology: RecordChronology,
): boolean {
  if (!chronology.activities?.hasNextPage) {
    return false;
  }
  return (
    filter === "activities" ||
    filter === "conversations" ||
    (filter === "all" && !chronology.changesAreTheLimit)
  );
}

/**
 * chronologyEmptyKey is what "there is none" says under each filter. Only the
 * Activities view takes the caller's own word, because only that view is
 * about a relationship somebody has to recognise; the other two are about the
 * record and read the same everywhere.
 */
export const CHRONOLOGY_EMPTY_KEYS: Readonly<
  Record<Exclude<TimelineFilter, "activities">, MessageKey>
> = {
  changes: "chronology.changesEmpty",
  all: "chronology.allEmpty",
  conversations: "chronology.conversationsEmpty",
};

// The merged view is cut at the newest "oldest loaded" among the feeds that
// still have more. Another page of changes only reaches the reader when the
// change feed owns that cut — i.e. its oldest loaded row is not older than
// the activity feed's.
function changesOwnTheCut(
  changesHaveMore: boolean,
  activitiesHaveMore: boolean,
  changeEntries: TimelineEntry[],
  activityEntries: TimelineEntry[],
): boolean {
  // Instants, not the strings that spell them — the same reason
  // mergeChronology compares numbers: two feeds written by two stores spell
  // one moment two ways. Seeded with the first row rather than left to throw
  // on an empty one.
  const oldest = (rows: TimelineEntry[]) =>
    rows.length > 0
      ? Date.parse(
          rows.reduce(
            (a, b) => (Date.parse(a.atIso) < Date.parse(b.atIso) ? a : b),
            rows[0],
          ).atIso,
        )
      : undefined;
  const oldestChange = oldest(changeEntries);
  const oldestActivity = oldest(activityEntries);
  return (
    changesHaveMore &&
    (!activitiesHaveMore ||
      oldestChange === undefined ||
      oldestActivity === undefined ||
      oldestChange >= oldestActivity)
  );
}

/**
 * chronologyNotice keeps four things apart that all render as an empty list
 * if you let them: still loading, the read failed, the section was never in
 * the payload, and the record genuinely has nothing to show. Only the last
 * one may say so — the other three would have a rep conclude nobody has ever
 * touched this record.
 *
 * The empty sentence names what the filter was looking for. "Nothing logged
 * on this account" under the Changes filter would be a claim about the
 * activity feed the reader is not looking at. `activitiesEmptyKey` is the
 * caller's own word for the Activities view, for the same reason
 * CHRONOLOGY_EMPTY_KEYS leaves that one out.
 */
export function chronologyNotice(
  activitiesEmptyKey: MessageKey,
  timeline: {
    loading: boolean;
    failed: boolean;
    assembled: boolean;
    filter: TimelineFilter;
  },
  count: number,
  t: ReturnType<typeof useT>,
): ReactNode {
  if (timeline.loading) {
    return <Skeleton width="100%" height={48} />;
  }
  if (timeline.failed || !timeline.assembled) {
    return <EmptyState>{t("co.section.unavailable")}</EmptyState>;
  }
  if (count > 0) {
    return undefined;
  }
  // The caller's own sentence covers the combined cut as well as the narrow
  // one: an empty ALL is an empty RECORD, and what a reader needs then is what
  // would land here and how — which is the sentence the caller wrote about its
  // own record, not the generic one about a merge. Only the changes cut has a
  // fact of its own to state.
  return (
    <EmptyState>
      {t(
        timeline.filter === "changes"
          ? CHRONOLOGY_EMPTY_KEYS.changes
          : activitiesEmptyKey,
      )}
    </EmptyState>
  );
}
