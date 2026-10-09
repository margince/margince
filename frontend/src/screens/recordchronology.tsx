import type { ReactNode } from "react";
import { useState } from "react";
import type { components } from "../api/schema";
import type { EntityKind } from "../app/entity";
import { activityTimeline } from "../design-system/activitytimeline";
import { EmptyState, PendingBody } from "../design-system/atoms";
import type { TimelineEntry } from "../design-system/composed";
import { FilterPills } from "../design-system/filterpills";
import type { RecordTimeline } from "../design-system/recordtimeline";
import { useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { LoadMoreButton, useViewerId } from "./common";
import { changeTimeline, useFieldHistory } from "./history";
import { historyFieldLabel } from "./historyfieldlabels";
import type { HistoryValueCtx } from "./historyvalues";

// A record's History, for every record page. All is what was said: threads,
// mail, calls, meetings, notes and tasks. Field changes stand under Changes
// alone. A reader opens History for the conversation, and an edited name above
// every mail pushes it out of view.

type Activity = components["schemas"]["Activity"];
type ChangesQuery = ReturnType<typeof useFieldHistory>;

// `conversations` is in the vocabulary but not in the base row. A page offers
// it through `ChronologyFilter`'s own flag once it renders the cut. A pill
// whose press changes nothing reads as a broken row.
export const TIMELINE_FILTERS = ["all", "changes"] as const;
export type TimelineFilter =
  | (typeof TIMELINE_FILTERS)[number]
  | "conversations";

// Whether the cut reads the exchanges alone, which is every cut but Changes.
// One predicate answers whether the change feed is read, what draws as loading
// or failed, and which feed "load older" lengthens. A cut added to the
// vocabulary is then answered everywhere.
export function readsExchangesOnly(filter: TimelineFilter): boolean {
  return filter !== "changes";
}

/**
 * useChronologyFilter owns the filter for ONE record rather than for the
 * session. When both records are already cached the route swaps one for
 * another without ever unmounting the section, so a reader who checked
 * Changes once met Changes on every record afterwards.
 */
export function useChronologyFilter(
  recordId: string,
): [TimelineFilter, (next: TimelineFilter) => void] {
  // All by default: the conversation is what a reader opens History for.
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
  onFilter: (next: TimelineFilter) => void;
}>) {
  const t = useT();
  const labels: Record<TimelineFilter, string> = {
    all: t("chronology.all"),
    conversations: t("chronology.conversations"),
    changes: t("chronology.changes"),
  };
  // Conversations sits between the whole and the parts: it is a READING of
  // the exchanges, narrower than All and wider than one kind.
  const cuts: readonly TimelineFilter[] = conversations
    ? ["all", "conversations", "changes"]
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
  // What the current filter is waiting on or failed at. A query that is
  // switched off reports pending forever. A caller reading the query's own
  // flags would draw a skeleton under All that never becomes a timeline.
  loading: boolean;
  failed: boolean;
  // The activity feed's own next page, when the caller can fetch one.
  activities?: RecordTimeline;
};

/**
 * useRecordChronology draws the activities the caller already holds, or the
 * record's change feed under Changes. The activities arrive as a prop because
 * the composite record read (the 360) has already fetched them. A second fetch
 * here could show a different moment than the rest of the page.
 */
export function useRecordChronology({
  kind,
  recordId,
  filter,
  activities,
  activitiesHaveMore,
  loadMore,
  renderActions,
  onOpenEmail,
  values,
}: Readonly<{
  kind: EntityKind;
  recordId: string;
  filter: TimelineFilter;
  /**
   * Opens one message in the record's drawer. Handed in rather than mounted
   * here, because the drawer belongs to the page — one over the record, with
   * the record still legible behind it — and this hook builds a list.
   *
   * Absent leaves the email rows readable and not openable, which is what a
   * surface with nowhere to open them should draw.
   */
  onOpenEmail?: (activityId: string) => void;
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
  // Only the Changes cut reads the change feed. The kind, word and date dials
  // narrow the exchanges, so they never hide a change.
  const wantsChanges = !readsExchangesOnly(filter);
  const changes = useFieldHistory(kind, recordId, { enabled: wantsChanges });
  // `page.data ?? []`: a 200 with no body is a shape the contract permits, and
  // flattening it would hand the mapper below an `undefined` row.
  const changeRows =
    changes.data?.pages.flatMap((page) => page.data ?? []) ?? [];
  // The contacts on each exchange, named through the resolver the change rows
  // use for their stored ids. Both cuts then name a contact the same way.
  const activityEntries = activityTimeline(
    activities,
    viewerId,
    renderActions,
    values.nameOf
      ? {
          nameOf: (entityType, entityId) =>
            entityType === "contact" ? values.nameOf?.(entityId) : undefined,
          t,
          locale: values.locale,
        }
      : undefined,
  ).map((entry) =>
    // Only an email has a drawer to open, and only when the caller has one to
    // open it in.
    onOpenEmail && entry.emailSummary
      ? { ...entry, onOpenEmail: () => onOpenEmail(entry.id) }
      : entry,
  );
  // historyFieldLabel, not coldFieldLabel: changeRows is the same
  // FieldHistoryEntry feed the Changes tab renders, and COLD_FIELD_LABELS
  // names a different vocabulary (site-read/enrichment fields) that has no
  // reason to know an ordinary record field's word.
  const changeEntries = changeTimeline(
    changeRows,
    (field) => historyFieldLabel(field, t),
    values,
    t("timeline.fieldUpdated"),
    viewerId,
  );
  if (readsExchangesOnly(filter)) {
    // The renderer, not this hook, narrows these rows to threads.
    return {
      entries: activityEntries,
      // A capped list that says nothing reads as the whole history: a reader
      // at the oldest of 25 rows takes it for the day the relationship began.
      truncated: activitiesHaveMore,
      changes,
      loading: false,
      failed: false,
      activities: loadMore,
    };
  }
  return {
    entries: changeEntries,
    truncated: false,
    changes,
    loading: wantsChanges && changes.isPending,
    failed: wantsChanges && changes.isError,
    activities: undefined,
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
    activitiesCanGrow(filter, chronology)
  );
}

/**
 * ChronologyFooter is what the list owes the reader underneath it: whether it
 * stops short of the record's whole history, and the button that lengthens it.
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
        <p className="t-caption">{t("chronology.truncatedActivities")}</p>
      )}
      {filter === "changes" && <LoadMoreButton query={chronology.changes} />}
      {activitiesCanGrow(filter, chronology) && chronology.activities && (
        <LoadMoreButton query={chronology.activities} />
      )}
    </>
  );
}

function activitiesCanGrow(
  filter: TimelineFilter,
  chronology: RecordChronology,
): boolean {
  return (
    readsExchangesOnly(filter) && Boolean(chronology.activities?.hasNextPage)
  );
}

/**
 * CHRONOLOGY_EMPTY_KEYS is what "there is none" says under each filter. All
 * takes the caller's own word, because it is about a relationship somebody has
 * to recognise; the other two read the same on every record.
 */
export const CHRONOLOGY_EMPTY_KEYS: Readonly<
  Record<Exclude<TimelineFilter, "all">, MessageKey>
> = {
  changes: "chronology.changesEmpty",
  conversations: "chronology.conversationsEmpty",
};

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
 * caller's own word for the All view, for the same reason
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
    // What is being waited for, not a mute bar: the reader is waiting on one
    // named feed, and the placeholder can say which.
    return <PendingBody label={t("record.chronologyLoading")} />;
  }
  if (timeline.failed || !timeline.assembled) {
    return <EmptyState>{t("co.section.unavailable")}</EmptyState>;
  }
  if (count > 0) {
    return undefined;
  }
  // An empty All is an empty record, and the caller's own sentence says what
  // would land here and how. Only the Changes cut has a fact of its own.
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
