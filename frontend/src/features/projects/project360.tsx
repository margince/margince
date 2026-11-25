// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useId, useState } from "react";
import { api } from "../../api/client";
import { ifMatch, requireVersion } from "../../api/version";
import {
  type ActivityGate,
  useActivityGate,
  writableTarget,
} from "../../components/activitygate";
import { ArchiveAction } from "../../components/archive";
import { EditAccessRequest } from "../../components/editaccessrequest";
import { useRailRename } from "../../components/railfield";
import { RecordTeam } from "../../components/recordteam";
import { useFieldsDialog } from "../../components/sectionedit";
import {
  snoozedDueAt,
  TaskCompleteCheck,
  TaskQuickActions,
  useTaskUpdate,
} from "../../components/taskactions";
import {
  currentDueFields,
  useMoveTask,
} from "../../components/taskduedatepick";
import { TaskWriteFailure } from "../../components/taskwritefailure";
import { Badge, Button, OverflowMenu } from "../../design-system/atoms";
import { QueryGate } from "../../design-system/querystates";
import { RecordView } from "../../design-system/recordview";
import { useT } from "../../i18n";
import { taskWriteKeys } from "../../lib/activitykeys";
import { useCan } from "../../lib/capability";
import { archivedContactRow } from "../../lib/contact.queries";
import { catalogOfRecord } from "../../lib/customfields.logic";
import { labelFor } from "../../lib/labelfor";
import { unwrap } from "../../lib/problem";
import { useShortcut } from "../../lib/shortcuts";
import { useArchiveRefusal } from "../../lib/writerefusal";
import { ShareAction } from "../records/share";
import { TaskDetailModal } from "../worklist/taskdetailmodal";
import { useProjectScreenReads } from "./project360.reads";
import { useProjectVerbRefusal } from "./project360.refusal";
import {
  type ArchivedHistory,
  ArchivedProjectPage,
  isMissingProject,
  ProjectNotFound,
} from "./projectarchived";
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
import { ProjectNewDeal } from "./projectnewdeal";
import { ProjectNext } from "./projectnext";
import { AssignProjectOwnerAction } from "./projectowner";
import { AdvanceProjectModal } from "./projectphase";
import { ProjectKeyChip } from "./projects";
import { PHASE_LABEL, type Project, type ProjectPhase } from "./projects.form";
import {
  type Commitment,
  type Project360,
  StakeholdersCard,
} from "./projectsections";
import { nextCommitment } from "./projectstanding";
import { ProjectStanding } from "./projectstandingcard";
import { ProjectTabs } from "./projecttabs";
import "./projects.css";
import { RecordGone } from "../../components/recordgone";
import { useViewerZone } from "../../lib/viewerzone";
import { ProjectHistory, ProjectLatestInbound } from "./project360.chronology";
import { ProjectCards } from "./projectcards";
import { useProjectEdit } from "./projectedit";

// The project page: one composite read (GET /projects/{id}/360) drawn as two
// cards. On the left, what the project is; on the right, the next task, how it
// stands, and the cards it rests on. What the design leaves off the page (the
// full history, the files, the companies and contacts on it) opens in a drawer.
// `sections_omitted` marks what a card cannot show: a section the reader's role
// cannot read says so instead of drawing an empty list.
export function ProjectScreen({
  id,
  archivedHistory,
}: Readonly<{ id: string; archivedHistory?: ArchivedHistory }>) {
  const t = useT();
  const { view, row } = useProjectScreenReads(id);
  const archived = archivedContactRow(row);
  // A disabled row keeps its last error, so a 360 that answers since then wins.
  const missing = view.isError && row.isError && isMissingProject(row.error);
  if (archived || missing) {
    return (
      <div className="wrap">
        {archived ? (
          <ArchivedProjectPage project={archived} history={archivedHistory} />
        ) : (
          <ProjectNotFound />
        )}
      </div>
    );
  }
  return (
    <div className="wrap">
      <QueryGate
        query={view}
        pendingLabel={t("nav.projects")}
        gone={<RecordGone kind="project" />}
      >
        {(data) => <ProjectPage view={data} />}
      </QueryGate>
    </div>
  );
}

// The drawers the page opens; "tasks" is the history narrowed to tasks.
type ProjectDrawerKind = "history" | "tasks" | "documents" | "contacts";

// The history drawer's title, kept past closing so it does not change while
// the drawer animates out.
function useHistoryDrawerTitle(drawer: ProjectDrawerKind | null): string {
  const t = useT();
  const [kind, setKind] = useState<"history" | "tasks">("history");
  if ((drawer === "history" || drawer === "tasks") && drawer !== kind) {
    setKind(drawer);
  }
  return kind === "tasks"
    ? t("project.commitments.all")
    : t("record.fullHistory");
}

function ProjectPage({ view }: Readonly<{ view: Project360 }>) {
  const t = useT();
  const readerZone = useViewerZone();
  const project = view.project;
  const readOnlyReasonId = useId();
  const [moveTo, setMoveTo] = useState<ProjectPhase | null>(null);
  const [drawer, setDrawer] = useState<ProjectDrawerKind | null>(null);
  const historyTitle = useHistoryDrawerTitle(drawer);
  // Every write affordance on this page answers one question, asked once. An
  // archived project takes no changes, and one this caller cannot write takes
  // none from them. One answer and one rendered sentence, so every disabled
  // control describes itself by pointing at the same explanation.
  const readOnlyReason = useProjectVerbRefusal(project);
  const readOnly = Boolean(readOnlyReason);
  // Logging files an activity under the project rather than changing it, so
  // only the archive refuses it here; the activity grant decides the rest.
  const logRefusedId = project.archived_at ? readOnlyReasonId : undefined;
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
  const nextVerbs = useNextTaskVerbs(view, taskUpdate, canUpdateTask);
  // Log in the strip and Add task in the menu open one drawer. The page holds
  // that drawer and the gate both verbs read.
  const activity = useActivityGate(writableTarget(project));
  const [logDrawer, setLogDrawer] = useState<LogDrawer>(null);
  const edit = useProjectEdit(project);
  const rename = useRailRename(edit, "name", t("project.name"));
  const editAll = useFieldsDialog(
    edit,
    t("project.edit"),
    edit.fields,
    edit.canEdit,
    catalogOfRecord("project"),
  );
  return (
    <RecordView
      aside={<ProjectDetails view={view} />}
      name={project.name}
      rename={rename}
      identity={project.id}
      zone={readerZone}
      nameBadge={<ProjectSubtitle view={view} />}
      pulse={
        <>
          {/* Neutral here: the phase is a fact about the project, and How
              it stands carries the reading of how it is going. */}
          <Badge>{t(labelFor(PHASE_LABEL, project.phase))}</Badge>
          {project.key && <ProjectKeyChip projectKey={project.key} dense />}
          <ProjectSharedPill projectId={project.id} />
        </>
      }
      actions={
        <>
          <ProjectLogVerbs
            projectId={project.id}
            refusedReasonId={logRefusedId}
            activity={activity}
            drawer={logDrawer}
            onDrawer={setLogDrawer}
          />
          <ProjectActions
            project={project}
            refusedReasonId={readOnly ? readOnlyReasonId : undefined}
            logRefusedId={logRefusedId}
            activity={activity}
            onAddTask={() => setLogDrawer("task")}
            onEdit={editAll.start}
            editWaiting={editAll.waiting}
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
          <>
            <p id={readOnlyReasonId}>{readOnlyReason}</p>
            <EditAccessRequest kind="project" record={project} />
          </>
        ) : undefined
      }
    >
      <ProjectLatestInbound projectId={project.id}>
        <ProjectNext view={view} onOpenTask={setOpenTask} {...nextVerbs} />
      </ProjectLatestInbound>
      <ProjectStanding
        view={view}
        refusedReasonId={readOnly ? readOnlyReasonId : undefined}
        onMove={setMoveTo}
        onOpenDocuments={() => setDrawer("documents")}
        onOpenHistory={() => setDrawer("history")}
      />
      <ProjectCards
        view={view}
        canEdit={!readOnly}
        newDeal={
          // New Deal from here binds the deal to this project. Binding is held
          // to project write authority rather than deal permission alone.
          // Winning the deal later advances the project's phase without
          // re-checking, so `EnsureAttachable` proves the authority at the
          // moment of attaching.
          !readOnly && <ProjectNewDeal view={view} />
        }
        tick={
          canUpdateTask
            ? (commitment) => (
                <CommitmentTick commitment={commitment} update={taskUpdate} />
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
      {editAll.element}
      <ProjectHealthModal
        open={healthEdit !== null}
        onClose={() => setHealthEdit(null)}
        projectId={project.id}
        correcting={healthEdit && healthEdit !== "new" ? healthEdit : undefined}
      />
      <ProjectDrawer
        open={drawer === "history" || drawer === "tasks"}
        title={historyTitle}
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
        <ProjectDocuments view={view} readOnly={readOnly} />
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

// The verbs write the task at the version the commitment row carries.
function useNextTaskVerbs(
  view: Project360,
  update: ReturnType<typeof useTaskUpdate>,
  enabled: boolean,
) {
  const zone = useViewerZone();
  const move = useMoveTask(update);
  const next = nextCommitment(view);
  if (!next || !enabled) {
    return {};
  }
  return {
    // No due date here: the bar draws its own Snooze beside Done.
    action: (
      <>
        <TaskQuickActions
          activityId={next.activityId}
          version={next.version}
          dueAt={null}
          update={update}
        />
        <TaskWriteFailure taskId={next.activityId} update={update} />
      </>
    ),
    snooze: next.dueAt
      ? {
          onSnooze: () => {
            const nextDue = snoozedDueAt(
              next.dueAt,
              next.dueHasTime,
              zone,
              new Date(),
              next.dueIntent,
            );
            if (nextDue)
              void move({
                id: next.activityId,
                version: next.version,
                to: nextDue,
                back: currentDueFields(
                  next.dueAt,
                  next.dueHasTime,
                  zone,
                  next.dueIntent,
                ),
              });
          },
          pending: update.isPending,
        }
      : undefined,
  };
}

function CommitmentTick({
  commitment,
  update,
}: Readonly<{
  commitment: Commitment;
  update: ReturnType<typeof useTaskUpdate>;
}>) {
  return (
    <TaskCompleteCheck
      activityId={commitment.activity_id}
      subject={commitment.subject}
      version={commitment.version}
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
  logRefusedId,
  activity,
  onAddTask,
  onEdit,
  editWaiting,
  health,
  onOpen,
  onHealth,
}: Readonly<{
  project: Project;
  refusedReasonId?: string;
  // Add task's own refusal, which only the archive sets; see the page.
  logRefusedId?: string;
  activity: ActivityGate;
  onAddTask: () => void;
  onEdit: () => void;
  // The edit is open but holds its dialog for the custom-field catalog.
  editWaiting: boolean;
  // The reading that stands, which Correct reading corrects.
  health?: ProjectHealthAssessment;
  onOpen: (drawer: ProjectDrawerKind) => void;
  onHealth: (edit: ProjectHealthAssessment | "new") => void;
}>) {
  const t = useT();
  const archiveRefusal = useArchiveRefusal("project");
  const taskRefusedId = logRefusedId ?? activity.refusedId;
  const taskKeys = useShortcut(
    "record.addTask",
    onAddTask,
    !activity.pending && !taskRefusedId,
  );
  return (
    <OverflowMenu label={t("record.moreActions")} word={t("record.verb.more")}>
      {/* First: the most frequent verb in the menu. */}
      <Button
        disabled={activity.pending}
        reasonId={taskRefusedId}
        onClick={onAddTask}
        aria-keyshortcuts={taskKeys}
      >
        {t("log.addTask")}
      </Button>
      {/* Worded: a bare pencil among sentences names nothing to a reader. */}
      <Button reasonId={refusedReasonId} onClick={onEdit} pending={editWaiting}>
        {t("project.edit")}
      </Button>
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
        refusedReason={archiveRefusal}
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
        onArchived={(from) => from.closeOnto({ screen: "projects" })}
      />
    </OverflowMenu>
  );
}
