import { type ReactNode, useState } from "react";
import { JustInScope, useLatestInbound } from "../../components/latestinbound";
import { RecordStory } from "../../design-system/recordstory";
import {
  type ActivityKind,
  hasTimelineFilters,
  NO_TIMELINE_FILTERS,
  type TimelineFilters,
  useRecordTimeline,
} from "../../design-system/recordtimeline";
import { SurfaceState, sectionState } from "../../design-system/surfacestate";
import { TimelineFilterBar } from "../../design-system/timelinefilterbar";
import { groupChronology } from "../../design-system/timelinegroups";
import { useLocale, useT } from "../../i18n";
import { useOpenEmail } from "../../lib/openemail";
import { useRecordZone } from "../../lib/recordzone";
import { OpenEmailDrawer } from "../mail/openemaildrawer";
import {
  ChronologyFilter,
  ChronologyFooter,
  chronologyNotice,
  useChronologyFilter,
  useRecordChronology,
} from "../records/recordchronology";
import { RowAnswerActions, TimelineActions } from "../records/timelineactions";
import type { Project360 } from "./projectsections";

type ChronologySlots = Readonly<{
  timeline?: ReturnType<typeof useRecordChronology>["entries"];
  timelineGroups?: ReturnType<typeof groupChronology>;
  timelineHeader?: ReactNode;
  timelineFooter?: ReactNode;
  timelineNotice?: ReactNode;
}>;

/**
 * The project's full history, drawn in the drawer the More menu and the
 * Commitments card open. `kind` starts the list narrowed, so "All tasks"
 * opens on the tasks.
 */
export function ProjectHistory({
  view,
  kind,
}: Readonly<{ view: Project360; kind?: ActivityKind }>) {
  const recordZone = useRecordZone();
  const story = useProjectChronology(view, kind);
  return (
    <RecordStory
      timeline={story.timeline ?? []}
      timelineGroups={story.timelineGroups}
      timelineHeader={story.timelineHeader}
      timelineFooter={story.timelineFooter}
      timelineNotice={story.timelineNotice}
      zone={recordZone}
    />
  );
}

/**
 * The chronology, with the same filter the company page offers. All draws
 * the activities the 360 already carries (capped, with `has_more` beside
 * them), and Changes the field-history feed. Drawing the 360's own page keeps
 * the list in step with the rollup figures read in the same transaction.
 */
function useProjectChronology(
  view: Project360,
  kind: ActivityKind | undefined,
): ChronologySlots {
  const t = useT();
  const { locale } = useLocale();
  const recordZone = useRecordZone();
  const [filter, setFilter] = useChronologyFilter(view.project.id);
  const activities = view.activities;
  const activitiesState = sectionState(
    view,
    "activities",
    Boolean(activities),
    activities?.data.length ?? 0,
  );
  // The drawer mounts per opening, so its narrowing starts fresh each time.
  const [filters, setFilters] = useState<TimelineFilters>(
    kind ? { kind } : NO_TIMELINE_FILTERS,
  );
  // The 360's own page seeds the list; older pages and every narrowed read
  // come from the activity list itself.
  const timeline = useRecordTimeline("project", view.project.id, {
    filters,
    firstPage: activities,
  });
  const [openEmail, setOpenEmail] = useOpenEmail();
  const history = useRecordChronology({
    onOpenEmail: setOpenEmail,
    kind: "project",
    recordId: view.project.id,
    filter,
    activities: timeline.activities,
    activitiesHaveMore: timeline.hasNextPage,
    loadMore: timeline,
    // What a stored value needs to be read as what it means. This record holds
    // no money of its own, so the currency is absent, and a minor-unit column
    // says so rather than printing a bare integer.
    values: { currency: null, locale, zone: recordZone },
    renderActions: (activity) => (
      <TimelineActions
        activity={activity}
        entityType="project"
        entityId={view.project.id}
      />
    ),
  });
  // A withheld activities section is not an empty timeline, and the change
  // feed is a separate grant: All says withheld, Changes still reads.
  if (activitiesState === "withheld" && filter !== "changes") {
    return {
      timeline: [],
      timelineHeader: <ChronologyFilter filter={filter} onFilter={setFilter} />,
      timelineNotice: (
        <SurfaceState
          state="withheld"
          emptyLabel=""
          loadingLabel={t("nav.projects")}
        >
          {null}
        </SurfaceState>
      ),
    };
  }
  return {
    timeline: history.entries,
    timelineGroups: groupChronology(history.entries, timeline.hasNextPage),
    timelineHeader: (
      <>
        <ChronologyFilter filter={filter} onFilter={setFilter} />
        {filter !== "changes" && (
          <TimelineFilterBar value={filters} onChange={setFilters} />
        )}
      </>
    ),
    timelineFooter: (
      <>
        <ChronologyFooter filter={filter} chronology={history} />
        {/* One drawer over the project, beside the chronology it opens from. */}
        <OpenEmailDrawer
          activityId={openEmail}
          zone={recordZone}
          onClose={() => setOpenEmail(null)}
        />
      </>
    ),
    timelineNotice: chronologyNotice(
      "project.timeline.empty",
      filter,
      history,
      {
        // A narrowed read is the list's own and has its own wait; the
        // unfiltered one is the 360's section, already on screen.
        loading: timeline.isPending,
        failed: timeline.isError,
        assembled: hasTimelineFilters(filters)
          ? timeline.isSuccess
          : Boolean(activities),
      },
      t,
    ),
  };
}

/**
 * The newest unanswered thing anyone on the project sent, for the Next bar in
 * `children`, with the verbs its timeline row has.
 */
export function ProjectLatestInbound({
  projectId,
  children,
}: Readonly<{ projectId: string; children: ReactNode }>) {
  const latest = useLatestInbound("project", projectId);
  return (
    <JustInScope
      latest={latest}
      entityType="project"
      entityId={projectId}
      actions={(activity) => (
        <RowAnswerActions
          activity={activity}
          entityType="project"
          entityId={projectId}
        />
      )}
    >
      {children}
    </JustInScope>
  );
}
