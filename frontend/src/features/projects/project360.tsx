// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { useId, useState } from "react";
import { api } from "../../api/client";
import { ifMatch, requireVersion } from "../../api/version";
import {
  type ObjectCustomFields,
  useObjectCustomFields,
} from "../../components/customfields.form";
import { EditAction } from "../../components/edit";
import { PageAsideToggle, usePageAside } from "../../components/pageaside";
import { OverflowMenu } from "../../design-system/atoms";
import { QueryGate } from "../../design-system/querystates";
import { RecordView } from "../../design-system/recordview";
import { useLocale, useT } from "../../i18n";
import { taskWriteKeys } from "../../lib/activitykeys";
import { useCan } from "../../lib/capability";
import { unwrap } from "../../lib/problem";
import { useRecordZone } from "../../lib/recordzone";
import { navigate } from "../../lib/router";
import { NewDealAction } from "../companies/companyactions";
import { ArchiveAction } from "../records/archive";
import { CustomFieldsPanel } from "../records/customfields.card";
import { RecordEmailVerb } from "../records/recordemail";
import { useOwnerPicker } from "../records/recordreferences";
import { RecordTeam } from "../records/recordteam";
import { ShareAction } from "../records/share";
import { useTaskUpdate } from "../worklist/taskactions";
import { TaskDetailModal } from "../worklist/taskdetailmodal";
import { useProjectVerbRefusal } from "./project360.refusal";
import { ProjectCompanies } from "./projectcompanies";
import { ProjectIdentityFacts } from "./projectheaderfacts";
import { ProjectHealth } from "./projecthealth";
import type { ProjectHealthAssessment } from "./projecthealth.queries";
import { ProjectHealthModal } from "./projecthealthmodal";
import { AssignProjectOwnerAction } from "./projectowner";
import { AdvanceProjectModal, PhaseStepper } from "./projectphase";
import {
  PROJECT_ACTIVITY_ANCHOR,
  PROJECT_COMMITMENTS_ANCHOR,
  PROJECT_DEALS_ANCHOR,
  RollupsStrip,
} from "./projectreadings";
import { PhaseBadge, ProjectKeyChip } from "./projects";
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
import { ProjectTabs } from "./projecttabs";
import "./projects.css";
import { useProjectChronology } from "./project360.chronology";

// The project page: one composite read (GET /projects/{id}/360) drawn in the
// company page's three zones — what the project IS on the left (its phase
// history, its contacts, its paper), what is HAPPENING in the middle (deals,
// open commitments), and the chronology underneath. `sections_omitted` is
// what keeps every card honest: a section the reader's role cannot read says
// so instead of drawing an empty list.

export function useProject360(id: string) {
  return useQuery({
    // Under the ["project", id] prefix the edit and archive actions
    // invalidate, so a saved name reaches the page without a second key.
    queryKey: ["project", id, "360"],
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

function ProjectPage({ view }: Readonly<{ view: Project360 }>) {
  const recordZone = useRecordZone();
  const { locale } = useLocale();
  const details = usePageAside();
  const project = view.project;
  const readOnlyReasonId = useId();
  const [moveTo, setMoveTo] = useState<ProjectPhase | null>(null);
  const chronology = useProjectChronology(view);
  // Every write affordance on this page answers ONE question, asked once: an
  // archived project takes no changes, and one this caller cannot write takes
  // none from them. The verbs, the stepper and the rail cards used to ask only
  // the first half, which offered a reader a phase move and a stakeholder edit
  // the save would refuse.
  //
  // One answer and one rendered sentence, so the page cannot disagree with
  // itself about whether the record is writable, and every disabled control
  // describes itself by pointing at the same explanation.
  const readOnlyReason = useProjectVerbRefusal(project);
  const readOnly = Boolean(readOnlyReason);
  // The commitments card's task detail, owned HERE rather than by the card:
  // one modal per page, so two cards cannot both put a dialog on the screen.
  // The same arrangement the account page uses for its own step rows.
  const [openTask, setOpenTask] = useState<string | null>(null);
  // The health card's dialog, owned here for the same reason the task detail
  // is: one modal per page. "new" records a reading; a row corrects that one.
  const [healthEdit, setHealthEdit] = useState<
    ProjectHealthAssessment | "new" | null
  >(null);
  const taskUpdate = useTaskUpdate(taskWriteKeys("project", project.id));
  // The TASK's own permission, not the project's. They are different questions
  // with different answers: the modal's verbs PATCH /activities/{id}, which
  // asks activity:update and the activity's own row authority — author,
  // assignee, host, or a writable linked record. Deriving this from the
  // project's writability would offer verbs the server refuses to a reader who
  // may write the project and not its activities, and hide valid ones from a
  // task's own author whenever the project is read-only.
  const canUpdateTask = useCan("activity", "update");
  // Read here and handed down: the schema request then runs BESIDE the
  // project's rather than after it.
  const cf = useObjectCustomFields("project");
  // Sheet, rung and facts are one change, argued in projectheaderfacts.tsx.
  return (
    <RecordView
      // Who is on this work comes first, then the paperwork. The phase history
      // logs moves the stepper above already shows, so it does not open the
      // column. The three record-keeping cards answer questions a reader comes
      // looking for; the two above answer the one they arrive with. The cards
      // stand straight in the pane. `RecordView` gives the aside the rail's
      // rhythm, so a wrapper of ours would be a second answer to card spacing.
      aside={
        <>
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
          <ProjectContractsCard view={view} />
          <ProjectDocumentsCard view={view} />
          <PhaseHistoryCard view={view} />
          <CustomFieldsPanel object="project" record={project} />
        </>
      }
      asideOpen={details.open}
      asideToggle={<PageAsideToggle pane />}
      name={project.name}
      identity={project.id}
      scale="compact"
      badges={<ProjectIdentityFacts view={view} locale={locale} />}
      zone={recordZone}
      // The phase and the key read beside the name, not among the verbs at
      // the far end of the header. Both are tags on the record: where it
      // stands and what a human calls it in a subject line.
      nameBadge={
        <>
          <PhaseBadge phase={project.phase} />
          {project.key ? <ProjectKeyChip projectKey={project.key} /> : null}
        </>
      }
      actions={
        <ProjectActions
          project={project}
          refusedReasonId={readOnly ? readOnlyReasonId : undefined}
          cf={cf}
        />
      }
      // In the header row, where a reader looks for a record's verbs, as the
      // company, contact and lead pages put them. Without it the verbs fall to
      // the full-width strip under the header.
      actionsInline
      // Where the project stands, at the foot of its head, the same row a
      // deal's stage ladder and a lead's stepper stand in. It is the question a
      // reader arrives with, so it is answered before the body rather than as
      // the first card inside it.
      standing={
        <PhaseStepper
          phase={project.phase}
          refusedReasonId={readOnly ? readOnlyReasonId : undefined}
          pending={false}
          onMove={setMoveTo}
        />
      }
      tabs={<ProjectTabs />}
      band={
        // One sentence and nothing else: why this record takes no changes, so
        // every control it refuses points at it. Absent while the project is
        // writable, where a reserved gap reads as a record with nothing said.
        readOnlyReason ? (
          <p id={readOnlyReasonId}>{readOnlyReason}</p>
        ) : undefined
      }
      {...chronology}
      timelineAnchorId={PROJECT_ACTIVITY_ANCHOR}
    >
      {/* The record's work column, at the record's own step. The readings open
            it: they are read as cards and so stand BESIDE the details pane, as on
            every other record, rather than in a band across both columns that the
            pane would reflow. The phase is not one of them — it says where the
            whole project stands, so it rides the head (`standing` above), which
            is full width already and above the pane. */}
      <div className="record-stack">
        <RollupsStrip view={view} />
        <ProjectHealth
          projectId={project.id}
          readOnly={readOnly}
          onRecord={() => setHealthEdit("new")}
          onCorrect={setHealthEdit}
        />
        <div id={PROJECT_DEALS_ANCHOR}>
          <ProjectDealsCard
            view={view}
            actions={
              // New Deal from here binds the deal to this project. Binding needs
              // project write authority, not deal permission alone: winning the
              // deal later advances the project's phase without re-checking. So
              // `EnsureAttachable` proves the authority at the moment of
              // attaching. A caller who may read this project but not write it
              // can still work deals, just not born into it.
              !readOnly &&
              project.company_id && (
                <NewDealAction
                  companyId={project.company_id}
                  companyName={project.name}
                  projectId={project.id}
                />
              )
            }
          />
        </div>
        <div id={PROJECT_COMMITMENTS_ANCHOR}>
          <CommitmentsCard view={view} onOpenTask={setOpenTask} />
        </div>
      </div>
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
      {openTask ? (
        <TaskDetailModal
          activityId={openTask}
          readOnly={!canUpdateTask}
          onClose={() => setOpenTask(null)}
          update={taskUpdate}
        />
      ) : null}
    </RecordView>
  );
}

/**
 * Edit, archive, assign an owner and share — the record's rare verbs, beside
 * its identity.
 *
 * Edit in the row and the rest behind the overflow, as on the deal page. A
 * project has no primary action, so a labelled Archive left the DESTRUCTIVE
 * verb as the loudest control on the page: it draws in the danger colour, and
 * with nothing green beside it the eye went there first.
 */

// Archived comes first because it is the reason a reader can act on, by
// restoring the record.
//
// Unlike a company, an UNOWNED project is refused rather than left open. The
// company page keeps its controls on an ownerless record because the claim door
// is what makes it writable, and shutting the controls would shut the way out.
// A project has no such door — `ClaimRecord` refuses the type outright — and
// `writeAuthorityPredicate` treats an ownerless row as nobody's to change. So
// here `writable` is the whole answer, and second-guessing it would offer
// controls the save refuses with nothing a reader could do about it.

// refusedReasonId is the id of the sentence the BAND renders, passed in rather
// than minted here: every verb but Email lives inside the OverflowMenu, and a
// sentence minted in that panel would not exist until the menu was first
// opened — so until somebody pressed the trigger, each refused item would
// describe itself by an id naming no element at all.
function ProjectActions({
  project,
  refusedReasonId,
  cf,
}: Readonly<{
  project: Project;
  refusedReasonId?: string;
  cf: ObjectCustomFields;
}>) {
  const t = useT();
  const owner = useOwnerPicker(project.owner_id);
  return (
    <>
      {/* A project's mail is a NEW conversation with somebody on the account
          behind it: the composer offers that roster in To and files the send
          under this project, which is the filing a message written from here
          can only mean. No `recordAddress` — a project is not a contact and has
          no address of its own to open with, so the reader picks from the
          account's contacts rather than the page guessing which of them a
          project-wide message is to. */}
      <RecordEmailVerb
        entityType="project"
        entityId={project.id}
        disabledReasonId={refusedReasonId}
      />
      <OverflowMenu label={t("record.moreActions")}>
        {/* Worded — a bare pencil among sentences names nothing to a reader. */}
        <EditAction<Project>
          labelled
          disabledReasonId={refusedReasonId}
          label={t("project.edit")}
          savedMessage={(saved) => t("record.saveDone", { name: saved.name })}
          fields={[
            ...projectFields(t, {
              owner,
              currentOwner: project.owner_id ?? null,
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
        {/* Last, and set apart by the panel's own seam (atoms.css): the one
            verb a reader cannot walk back does not sit in the routine run. */}
        <ArchiveAction
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
    </>
  );
}
