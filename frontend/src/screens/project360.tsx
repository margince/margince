// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { type ReactNode, useId, useState } from "react";
import { api } from "../api/client";
import { ifMatch, requireVersion } from "../api/version";
import { PageAside, PageAsideToggle } from "../app/pageaside";
import { useRecordZone } from "../app/recordzone";
import { navigate } from "../app/router";
import { OverflowMenu } from "../design-system/atoms";
import { RecordView } from "../design-system/composed";
import {
  hasTimelineFilters,
  useRecordTimeline,
  useTimelineFilters,
} from "../design-system/recordtimeline";
import { SurfaceState, sectionState } from "../design-system/surfacestate";
import { TimelineFilterBar } from "../design-system/timelinefilterbar";
import { formatDate } from "../format/format";
import { useLocale, useT } from "../i18n";
import { ArchiveAction } from "./archive";
import { QueryGate, throwProblem, useMe, useSorMode } from "./common";
import { NewDealAction } from "./companyactions";
import { TimelineActions } from "./compose";
import { EditAction } from "./edit";
import { EntityRef, OwnerName } from "./entityref";
import { ProjectCompanies } from "./projectcompanies";
import { AssignProjectOwnerAction } from "./projectowner";
import { AdvanceProjectModal, PhaseStepper } from "./projectphase";
import { RollupsStrip } from "./projectreadings";
import { PhaseBadge, ProjectKeyChip, useCompanyOptions } from "./projects";
import {
  mapProjectUpdate,
  type Project,
  type ProjectPhase,
  projectEditRecord,
  projectFields,
} from "./projects.form";
import {
  CommitmentsCard,
  PhaseHistoryCard,
  type Project360,
  ProjectContractsCard,
  ProjectDealsCard,
  ProjectDocumentsCard,
  StakeholdersCard,
} from "./projectsections";
import {
  ChronologyFilter,
  ChronologyFooter,
  chronologyNotice,
  useChronologyFilter,
  useRecordChronology,
} from "./recordchronology";
import { ShareAction } from "./share";
import { groupChronology } from "./timelinegroups";
import "./projects.css";

// The project page: one composite read (GET /projects/{id}/360) drawn in the
// company page's three zones — what the project IS on the left (its phase
// history, its people, its paper), what is HAPPENING in the middle (deals,
// open commitments), and the chronology underneath. `sections_omitted` is
// what keeps every card honest: a section the reader's role cannot read says
// so instead of drawing an empty list.

export function useProject360(id: string) {
  return useQuery({
    // Under the ["project", id] prefix the edit and archive actions
    // invalidate, so a saved name reaches the page without a second key.
    queryKey: ["project", id, "360"],
    queryFn: async () => {
      const { data, error } = await api.GET("/projects/{id}/360", {
        params: { path: { id } },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
  });
}

export function ProjectScreen({ id }: Readonly<{ id: string }>) {
  const view = useProject360(id);
  return (
    <div className="wrap">
      <QueryGate query={view}>
        {(data) => <ProjectPage view={data} />}
      </QueryGate>
    </div>
  );
}

function ProjectPage({ view }: Readonly<{ view: Project360 }>) {
  const t = useT();
  const recordZone = useRecordZone();
  const project = view.project;
  const archivedReasonId = useId();
  const [moveTo, setMoveTo] = useState<ProjectPhase | null>(null);
  const overlay = useSorMode() === "overlay";
  const chronology = useProjectChronology(view, overlay);
  const refusedByArchive = project.archived_at ? archivedReasonId : undefined;
  return (
    <RecordView
      name={project.name}
      subtitle={<ProjectSubtitle view={view} />}
      zone={recordZone}
      badges={
        <>
          <PhaseBadge phase={project.phase} />
          {project.key && <ProjectKeyChip projectKey={project.key} />}
        </>
      }
      actions={
        <>
          <ProjectActions
            project={project}
            refusedReasonId={refusedByArchive}
          />
          <PageAsideToggle />
        </>
      }
      // In the header row, where a reader looks for a record's verbs — as the
      // company, contact and lead pages already put them. Without it the row
      // fell to the full-width strip UNDER the header, so the one record page
      // with no primary action was also the one whose verbs were somewhere
      // else.
      actionsInline
      band={
        <div className="project-band">
          {project.archived_at && (
            <p id={archivedReasonId} className="t-caption">
              {t("project.archivedReadOnly")}
            </p>
          )}
          <PhaseStepper
            phase={project.phase}
            refusedReasonId={refusedByArchive}
            pending={false}
            onMove={setMoveTo}
          />
          <RollupsStrip view={view} />
        </div>
      }
      {...chronology}
    >
      {/* WHO is on this work comes first, then the paperwork. The column used
          to open with the phase history — a log of moves the stepper above
          already shows the current state of — so a reader scanning for "whose
          project is this" read a changelog first. The three record-keeping
          cards below answer questions a reader comes looking for on purpose;
          the two above answer the one they arrive with.

          It is the PAGE's own column, beside the work rather than inside the
          record's grid: same column, same fold and same memory of it as every
          other record page. */}
      <PageAside>
        <div className="project-rail">
          <ProjectCompanies
            projectId={project.id}
            companies={project.organizations}
            readOnly={Boolean(project.archived_at)}
          />
          <StakeholdersCard
            view={view}
            projectId={project.id}
            readOnly={Boolean(project.archived_at)}
          />
          <ProjectContractsCard view={view} />
          <ProjectDocumentsCard view={view} />
          <PhaseHistoryCard view={view} />
        </div>
      </PageAside>
      <div className="project-main">
        <ProjectDealsCard
          view={view}
          actions={
            !overlay &&
            !project.archived_at &&
            project.organization_id && (
              <NewDealAction
                orgId={project.organization_id}
                orgName={project.name}
                projectId={project.id}
              />
            )
          }
        />
        <CommitmentsCard view={view} />
      </div>
      <AdvanceProjectModal
        projectId={project.id}
        version={project.version}
        to={moveTo}
        onClose={() => setMoveTo(null)}
      />
    </RecordView>
  );
}

/** The company and the owner, under the name. */
function ProjectSubtitle({ view }: Readonly<{ view: Project360 }>) {
  const t = useT();
  const { locale } = useLocale();
  const recordZone = useRecordZone();
  const project = view.project;
  const company = view.organization;
  const companyState = sectionState(
    view,
    "organization",
    Boolean(company),
    company ? 1 : 0,
  );
  return (
    <span className="project-subtitle">
      {companyState === "withheld" ? (
        // The grant refused the company: say so rather than leave the name
        // out, which would read as a project with no company.
        <span data-testid="project-company-withheld">
          {t("state.withheld")}
        </span>
      ) : (
        <EntityRef
          kind="organization"
          id={project.organization_id}
          name={company?.name}
        />
      )}
      <span aria-hidden="true">·</span>
      <OwnerName ownerId={project.owner_id} unowned={t("list.unowned")} />
      {project.target_end_date && (
        <>
          <span aria-hidden="true">·</span>
          <span>
            {t("project.targetEndShort", {
              date: formatDate(project.target_end_date, locale, recordZone),
            })}
          </span>
        </>
      )}
    </span>
  );
}

/**
 * Edit, archive and share — the record's rare verbs, beside its identity.
 *
 * Edit in the row and the other two behind the overflow, as on the deal page.
 * A project has no primary action, so a labelled Archive left the DESTRUCTIVE
 * verb as the loudest control on the page: it draws in the danger colour, and
 * with nothing green beside it the eye went there first.
 */
function ProjectActions({
  project,
  refusedReasonId,
}: Readonly<{ project: Project; refusedReasonId?: string }>) {
  const t = useT();
  const me = useMe();
  const companies = useCompanyOptions();
  const overlay = useSorMode() === "overlay";
  return (
    <>
      <EditAction<Project>
        disabledReasonId={refusedReasonId}
        label={t("project.edit")}
        savedMessage={(saved) => t("record.saveDone", { name: saved.name })}
        fields={projectFields(t, {
          companies,
          me: me.data?.user.id ?? "",
          currentOwner: project.owner_id ?? null,
          mode: "edit",
        })}
        record={projectEditRecord(project)}
        update={async (values) => {
          const { data, error } = await api.PATCH("/projects/{id}", {
            params: {
              path: { id: project.id },
              ...ifMatch(requireVersion(project.version)),
            },
            body: mapProjectUpdate(values),
          });
          if (error) {
            throwProblem(error);
          }
          return data;
        }}
        invalidate="projects"
        recordKey="project"
      />
      <OverflowMenu label={t("record.moreActions")}>
        <ArchiveAction
          disabledReasonId={refusedReasonId}
          label={t("project.archive")}
          confirmText={t("project.archiveConfirm")}
          archivedMessage={t("record.archiveDone", { name: project.name })}
          archive={async () => {
            // Archive answers 204: the archived record is the one the page
            // already holds, so its id is what the shared choreography gets.
            const { error } = await api.DELETE("/projects/{id}", {
              params: {
                path: { id: project.id },
                ...ifMatch(requireVersion(project.version)),
              },
            });
            if (error) {
              throwProblem(error);
            }
            return { id: project.id };
          }}
          invalidate="projects"
          recordKey="project"
          onArchived={() => navigate({ screen: "projects" })}
        />
        <AssignProjectOwnerAction
          project={project}
          disabledReasonId={refusedReasonId}
        />
        {!overlay && (
          <ShareAction
            recordType="project"
            recordId={project.id}
            disabledReasonId={refusedReasonId}
          />
        )}
      </OverflowMenu>
    </>
  );
}

type ChronologySlots = Readonly<{
  timeline?: ReturnType<typeof useRecordChronology>["entries"];
  timelineGroups?: ReturnType<typeof groupChronology>;
  timelineHeader?: ReactNode;
  timelineFooter?: ReactNode;
  timelineNotice?: ReactNode;
}>;

/**
 * The chronology zone: the activities the 360 already carries (capped, with
 * `has_more` beside them), folded with the field-history feed under the same
 * Activities / Changes / All filter the company page offers. The 360's own
 * page of activities is what is drawn, so the list cannot disagree with the
 * rollup figures read in the same transaction.
 */
function useProjectChronology(
  view: Project360,
  overlay: boolean,
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
  const [filters, setFilters] = useTimelineFilters(view.project.id);
  // The 360's own page seeds the list; older pages and every narrowed read
  // come from the activity list itself.
  const timeline = useRecordTimeline("project", view.project.id, {
    filters,
    firstPage: activities,
  });
  const history = useRecordChronology({
    kind: "project",
    recordId: view.project.id,
    filter,
    // A narrowed read is a question about what was said, so the record's own
    // edits stand down: they are not meetings, and not what the reader asked.
    narrowed: hasTimelineFilters(filters),
    activities: timeline.activities,
    activitiesHaveMore: timeline.hasNextPage,
    loadMore: timeline,
    // What a stored value needs to be read as what it MEANS: this record holds
    // no money of its own, so the currency is absent and a minor-unit column
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
  if (overlay) {
    return { timeline: history.entries, timelineNotice: <span /> };
  }
  // A withheld activities section is not an empty timeline, and the change
  // feed is a separate grant: Activities and All say withheld, Changes still
  // reads.
  if (activitiesState === "withheld" && filter !== "changes") {
    return {
      timeline: [],
      timelineHeader: <ChronologyFilter filter={filter} onFilter={setFilter} />,
      timelineNotice: (
        <SurfaceState state="withheld" emptyLabel="">
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
    timelineFooter: <ChronologyFooter filter={filter} chronology={history} />,
    timelineNotice: chronologyNotice(
      "project.timeline.empty",
      {
        // The 360 is already on screen here, so for the unfiltered read only
        // the change feed can still be loading or failed; a narrowed read is
        // the list's own and has its own wait.
        loading: history.loading || timeline.isPending,
        failed: history.failed || timeline.isError,
        assembled:
          filter === "changes" ||
          (hasTimelineFilters(filters)
            ? timeline.isSuccess
            : Boolean(activities)),
        filter,
      },
      history.entries.length,
      t,
    ),
  };
}
