// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { useId, useState } from "react";
import { api } from "../../api/client";
import { ifMatch, requireVersion } from "../../api/version";
import {
  type ActivityGate,
  useActivityGate,
} from "../../components/activitygate";
import { ArchiveAction } from "../../components/archive";
import {
  type ObjectCustomFields,
  useObjectCustomFields,
} from "../../components/customfields.form";
import { EditAction } from "../../components/edit";
import { useOwnerPicker } from "../../components/recordowners";
import { RecordTeam } from "../../components/recordteam";
import {
  snoozedDueAt,
  TaskCompleteCheck,
  TaskQuickActions,
  useTaskUpdate,
} from "../../components/taskactions";
import { Badge, Button, OverflowMenu } from "../../design-system/atoms";
import { QueryGate } from "../../design-system/querystates";
import { RecordView } from "../../design-system/recordview";
import { useT } from "../../i18n";
import { taskWriteKeys } from "../../lib/activitykeys";
import { useActivity } from "../../lib/activityread";
import { useCan } from "../../lib/capability";
import { unwrap } from "../../lib/problem";
import { useRecordZone } from "../../lib/recordzone";
import { navigate } from "../../lib/router";
import { NewDealAction } from "../companies/companyactions";
import { ShareAction } from "../records/share";
import { TaskDetailModal } from "../worklist/taskdetailmodal";
import { project360Key } from "./project360.key";
import { useProjectVerbRefusal } from "./project360.refusal";
import { ProjectCompanies } from "./projectcompanies";
import {
  ProjectDetails,
  ProjectSharedPill,
  ProjectSubtitle,
} from "./projectdetails";
import { ProjectDocuments } from "./projectdocuments";
import { ProjectDrawer } from "./projectdrawer";
import type { ProjectHealthAssessment } from "./projecthealth.queries";
import { ProjectHealthModal } from "./projecthealthmodal";
import { type LogDrawer, ProjectLogVerbs } from "./projectlogverbs";
import { ProjectNext } from "./projectnext";
import { AssignProjectOwnerAction } from "./projectowner";
import { AdvanceProjectModal } from "./projectphase";
import { ProjectKeyChip } from "./projects";
import {
  mapProjectUpdate,
  PHASE_LABEL,
  type Project,
  type ProjectPhase,
  projectEditRecord,
  projectFields,
} from "./projects.form";
import { type Project360, StakeholdersCard } from "./projectsections";
import { nextCommitment } from "./projectstanding";
import { ProjectStanding } from "./projectstandingcard";
import { ProjectTabs } from "./projecttabs";
import "./projects.css";
import { ProjectHistory, ProjectLatestInbound } from "./project360.chronology";
import { ProjectCards } from "./projectcards";

// The project page: one composite read (GET /projects/{id}/360) drawn as two
// cards. On the left, what the project is; on the right, the next task, how it
// stands, and the cards it rests on. What the design leaves off the page (the
// full history, the files, the companies and contacts on it) opens in a drawer.
// `sections_omitted` marks what a card cannot show: a section the reader's role
// cannot read says so instead of drawing an empty list.

export function useProject360(id: string) {
  return useQuery({
    queryKey: project360Key(id),
    queryFn: async () => {
      return unwrap(
        await api.GET("/projects/{id}/360", {
          params: { path: { id } },
        }),
      );
    },
  });
}

export function ProjectScreen({ id }: Readonly<{ id: string }>) {
  const t = useT();
  const view = useProject360(id);
  return (
    <div className="wrap">
      <QueryGate query={view} pendingLabel={t("nav.projects")}>
        {(data) => <ProjectPage view={data} />}
      </QueryGate>
    </div>
  );
}

// The drawers the page opens; "tasks" is the history narrowed to tasks.
type ProjectDrawerKind = "history" | "tasks" | "documents" | "contacts";

function ProjectPage({ view }: Readonly<{ view: Project360 }>) {
  const t = useT();
  const recordZone = useRecordZone();
  const project = view.project;
  const readOnlyReasonId = useId();
  const [moveTo, setMoveTo] = useState<ProjectPhase | null>(null);
  const [drawer, setDrawer] = useState<ProjectDrawerKind | null>(null);
  // Every write affordance on this page answers one question, asked once. An
  // archived project takes no changes, and one this caller cannot write takes
  // none from them. One answer and one rendered sentence, so every disabled
  // control describes itself by pointing at the same explanation.
  const readOnlyReason = useProjectVerbRefusal(project);
  const readOnly = Boolean(readOnlyReason);
  // The task detail and the health dialog are owned here rather than by the
  // cards. One modal per page, so two cards cannot both put one on screen.
  const [openTask, setOpenTask] = useState<string | null>(null);
  const [healthEdit, setHealthEdit] = useState<
    ProjectHealthAssessment | "new" | null
  >(null);
  const taskUpdate = useTaskUpdate(taskWriteKeys("project", project.id));
  // The task's own permission, not the project's. The task verbs PATCH
  // /activities/{id}, which asks activity:update and the activity's own row
  // authority, whatever the reader may do to the project.
  const canUpdateTask = useCan("activity", "update");
  // Read here as well as in the details. The schema request then runs beside
  // the project's rather than after it, and both share its one cache entry.
  const cf = useObjectCustomFields("project");
  const nextVerbs = useNextTaskVerbs(view, taskUpdate, canUpdateTask);
  // Log in the strip and Add task in the menu open one drawer. The page holds
  // that drawer and the gate both verbs read.
  const activity = useActivityGate();
  const [logDrawer, setLogDrawer] = useState<LogDrawer>(null);
  return (
    <RecordView
      aside={<ProjectDetails view={view} />}
      name={project.name}
      identity={project.id}
      zone={recordZone}
      nameBadge={<ProjectSubtitle view={view} />}
      pulse={
        <>
          {/* Neutral here: the phase is a fact about the project, and How
              it stands carries the reading of how it is going. */}
          <Badge>{t(PHASE_LABEL[project.phase])}</Badge>
          {project.key && <ProjectKeyChip projectKey={project.key} dense />}
          <ProjectSharedPill projectId={project.id} />
        </>
      }
      actions={
        <>
          <ProjectLogVerbs
            projectId={project.id}
            refusedReasonId={readOnly ? readOnlyReasonId : undefined}
            activity={activity}
            drawer={logDrawer}
            onDrawer={setLogDrawer}
          />
          <ProjectActions
            project={project}
            refusedReasonId={readOnly ? readOnlyReasonId : undefined}
            activity={activity}
            onAddTask={() => setLogDrawer("task")}
            cf={cf}
            health={view.health?.current}
            onOpen={setDrawer}
            onHealth={setHealthEdit}
          />
        </>
      }
      tabs={<ProjectTabs />}
      band={
        // One sentence and nothing else: why this record takes no changes, so
        // every control it refuses points at it.
        readOnlyReason ? (
          <p id={readOnlyReasonId}>{readOnlyReason}</p>
        ) : undefined
      }
    >
      <ProjectLatestInbound projectId={project.id} />
      <ProjectNext view={view} onOpenTask={setOpenTask} {...nextVerbs} />
      <ProjectStanding
        view={view}
        refusedReasonId={readOnly ? readOnlyReasonId : undefined}
        onMove={setMoveTo}
        onOpenDocuments={() => setDrawer("documents")}
      />
      <ProjectCards
        view={view}
        newDeal={
          // New Deal from here binds the deal to this project. Binding is held
          // to project write authority rather than deal permission alone.
          // Winning the deal later advances the project's phase without
          // re-checking, so `EnsureAttachable` proves the authority at the
          // moment of attaching.
          !readOnly &&
          project.company_id && (
            <NewDealAction
              companyId={project.company_id}
              companyName={view.company?.name ?? project.name}
              projectId={project.id}
              link
            />
          )
        }
        tick={
          canUpdateTask
            ? (activityId) => (
                <CommitmentTick activityId={activityId} update={taskUpdate} />
              )
            : undefined
        }
        onOpenTask={setOpenTask}
        onOpenTasks={() => setDrawer("tasks")}
        onOpenDocuments={() => setDrawer("documents")}
      />
      <AdvanceProjectModal
        projectId={project.id}
        version={project.version}
        to={moveTo}
        onClose={() => setMoveTo(null)}
      />
      <ProjectHealthModal
        open={healthEdit !== null}
        onClose={() => setHealthEdit(null)}
        projectId={project.id}
        correcting={healthEdit && healthEdit !== "new" ? healthEdit : undefined}
      />
      <ProjectDrawer
        open={drawer === "history" || drawer === "tasks"}
        title={t("record.fullHistory")}
        onClose={() => setDrawer(null)}
      >
        <ProjectHistory
          view={view}
          kind={drawer === "tasks" ? "task" : undefined}
        />
      </ProjectDrawer>
      <ProjectDrawer
        open={drawer === "documents"}
        title={t("project.documents.title")}
        onClose={() => setDrawer(null)}
      >
        <ProjectDocuments view={view} />
      </ProjectDrawer>
      <ProjectDrawer
        open={drawer === "contacts"}
        title={t("project.more.contacts")}
        onClose={() => setDrawer(null)}
      >
        <div className="project-drawer-stack">
          <ProjectCompanies
            projectId={project.id}
            companies={project.companies}
            readOnly={readOnly}
          />
          <RecordTeam
            recordType="project"
            recordId={project.id}
            readOnly={readOnly}
          />
          <StakeholdersCard
            view={view}
            projectId={project.id}
            readOnly={readOnly}
          />
        </div>
      </ProjectDrawer>
      {openTask && (
        <TaskDetailModal
          activityId={openTask}
          readOnly={!canUpdateTask}
          onClose={() => setOpenTask(null)}
          update={taskUpdate}
        />
      )}
    </RecordView>
  );
}

// A commitment carries no row version, and a task write must name one. So
// the verbs read their task whole first and offer themselves once they have.
function useNextTaskVerbs(
  view: Project360,
  update: ReturnType<typeof useTaskUpdate>,
  enabled: boolean,
) {
  const zone = useRecordZone();
  const next = nextCommitment(view);
  const task = useActivity(next?.activityId, enabled).data;
  if (!next || !task) {
    return {};
  }
  const nextDue = snoozedDueAt(next.dueAt, zone);
  return {
    // No due date here: the bar draws its own Snooze beside Done.
    action: (
      <TaskQuickActions
        activityId={next.activityId}
        version={task.version}
        dueAt={null}
        update={update}
      />
    ),
    snooze: nextDue
      ? {
          onSnooze: () =>
            update.mutate({
              id: next.activityId,
              version: task.version,
              body: { due_at: nextDue },
            }),
          pending: update.isPending && update.variables?.id === next.activityId,
        }
      : undefined,
  };
}

function CommitmentTick({
  activityId,
  update,
}: Readonly<{
  activityId: string;
  update: ReturnType<typeof useTaskUpdate>;
}>) {
  const task = useActivity(activityId).data;
  if (!task) {
    return null;
  }
  return (
    <TaskCompleteCheck
      activityId={activityId}
      version={task.version}
      update={update}
    />
  );
}

// The record's rare verbs, behind the overflow. Archived leads the refusal,
// since restoring is the step a reader can take. An unowned project is
// refused: `ClaimRecord` refuses the type and `writeAuthorityPredicate` reads
// an ownerless row as nobody's. refusedReasonId comes from the band, because a
// sentence minted in the menu panel would not exist until the menu opened.
function ProjectActions({
  project,
  refusedReasonId,
  activity,
  onAddTask,
  cf,
  health,
  onOpen,
  onHealth,
}: Readonly<{
  project: Project;
  refusedReasonId?: string;
  activity: ActivityGate;
  onAddTask: () => void;
  cf: ObjectCustomFields;
  // The reading that stands, which Correct reading corrects.
  health?: ProjectHealthAssessment;
  onOpen: (drawer: ProjectDrawerKind) => void;
  onHealth: (edit: ProjectHealthAssessment | "new") => void;
}>) {
  const t = useT();
  const owner = useOwnerPicker(project.owner_id);
  return (
    <OverflowMenu label={t("record.moreActions")} word={t("record.verb.more")}>
      {/* First: the most frequent verb in the menu. */}
      <Button
        disabled={activity.pending}
        reasonId={refusedReasonId ?? activity.refusedId}
        onClick={onAddTask}
      >
        {t("log.addTask")}
      </Button>
      {/* Worded: a bare pencil among sentences names nothing to a reader. */}
      <EditAction<Project>
        labelled
        disabledReasonId={refusedReasonId}
        label={t("project.edit")}
        savedMessage={(saved) => t("record.saveDone", { name: saved.name })}
        fields={[
          ...projectFields(t, {
            owner,
            mode: "edit",
          }),
          ...cf.formFields,
        ]}
        record={{ ...projectEditRecord(project), ...cf.recordSlice(project) }}
        update={async (values, _rows, opened) => {
          return unwrap(
            await api.PATCH("/projects/{id}", {
              params: {
                path: { id: project.id },
                ...ifMatch(requireVersion(opened?.version)),
              },
              // Diffed against what the form opened on. No cf_ column is clearable, so a
              // snapshot diff would send every untouched empty one as a clear and the
              // save would be refused.
              body: {
                ...mapProjectUpdate(values),
                ...cf.toPatch(values, opened ?? {}),
              },
            }),
          );
        }}
        invalidate="projects"
        recordKey="project"
      />
      <ShareAction
        recordType="project"
        recordId={project.id}
        disabledReasonId={refusedReasonId}
      />
      <AssignProjectOwnerAction
        project={project}
        disabledReasonId={refusedReasonId}
      />
      <Button onClick={() => onOpen("contacts")}>
        {t("project.more.contacts")}
      </Button>
      {/* How it stands draws the reading; these are the ways to change it. */}
      <Button reasonId={refusedReasonId} onClick={() => onHealth("new")}>
        {t("projectHealth.record")}
      </Button>
      {health && (
        <Button reasonId={refusedReasonId} onClick={() => onHealth(health)}>
          {t("projectHealth.correct")}
        </Button>
      )}
      <Button onClick={() => onOpen("history")}>
        {t("record.fullHistory")}
      </Button>
      {/* Last, and set apart by the panel's own seam (atoms.css): the one
          verb a reader cannot walk back does not sit in the routine run. */}
      <ArchiveAction
        requires={{ object: "project", action: "delete" }}
        disabledReasonId={refusedReasonId}
        label={t("project.archive")}
        confirmText={t("project.archiveConfirm")}
        archivedMessage={t("record.archiveDone", { name: project.name })}
        archive={async () => {
          // Archive answers 204: the archived record is the one the page
          // already holds, so its id is what the shared choreography gets.
          unwrap(
            await api.DELETE("/projects/{id}", {
              params: {
                path: { id: project.id },
                ...ifMatch(requireVersion(project.version)),
              },
            }),
          );
          return { id: project.id };
        }}
        invalidate="projects"
        recordKey="project"
        onArchived={() => navigate({ screen: "projects" })}
      />
    </OverflowMenu>
  );
}
