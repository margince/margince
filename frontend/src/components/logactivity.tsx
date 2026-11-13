import { useMutation, useQueryClient } from "@tanstack/react-query";
import { type ReactNode, useId, useRef, useState } from "react";
import { api } from "../api/client";
import { useIdempotencyKey } from "../api/idempotencykey";
import { ifMatch, requireVersion } from "../api/version";
import { Button, Checkbox, Field, TextInput } from "../design-system/atoms";
import {
  DateInput,
  type ISODate,
  isISODate,
  isoOrBlank,
} from "../design-system/dateinput";
import { ErrorLine } from "../design-system/errorline";
import { refuseUnreadableField } from "../design-system/formvalidity";
import { Panel, PanelBody } from "../design-system/panel";
import { useSinglePress } from "../design-system/presslatch";
import { useToast } from "../design-system/toast";
import { useT } from "../i18n";
import {
  type ActivityDraft,
  activityRequestBody,
  canHaveTranscript,
  draftHoldsWork,
  draftSaysSomething,
  freshDraft,
  KINDS_WITH_A_CONTACT,
  type MeetingAttendees,
  NO_ATTENDEES,
  todayDay,
} from "../lib/activitybody";
import { useCanWrite } from "../lib/capability";
import type { EntityKind } from "../lib/entity";
import { unwrap } from "../lib/problem";
import { useMe } from "../lib/session";
import { ActivityBodyField } from "./activitybodyfield";
import { TaskAssigneeField } from "./assigneepicker";
import { LogActivityEntries } from "./extensionentries";
import { ActivityKindField } from "./logactivity.kindfield";
import { TaskCompletionChoice } from "./logactivity.taskcompletion";
import { assignedRecord } from "./recordassignments.queries";
import { TaskTypeField } from "./tasktype";
import "./logactivity.css";
import { useViewerZone } from "../lib/viewerzone";
import { FollowUpTaskFields, TaskTimeField } from "./followuptaskfields";
import { CallOutcomeField } from "./logactivity.calloutcome";
import { useActivityCustomFields } from "./logactivity.custom";
import {
  activityDealFilingKey,
  useLogDealFiling,
} from "./logactivity.dealfiling";
import { activityLogKeys } from "./logactivity.keys";
import {
  type MeetingRecord,
  openMeetingKey,
  recordOnMeeting,
  transcriptRecords,
  useRecordOfMeeting,
} from "./logactivity.openmeeting";
import { PolishWording } from "./logactivity.polish";
import {
  attachLoggedFiles,
  LogActivityFilesField,
  type PickedFile,
  TranscriptUploadField,
  useReportUnattached,
} from "./logactivityfiles";
import { MeetingAttendeeFields } from "./meetingattendees";
import { useUnsavedGuard } from "./unsaved";

export type OpeningKind = "note" | "task" | "call";

function latestDayOf(
  kind: ActivityDraft["kind"],
  recordZone: string,
): ISODate | undefined {
  const day = todayDay(kind, recordZone);
  return kind !== "task" && isISODate(day) ? day : undefined;
}

export function LogActivityForm({
  entityType,
  entityId,
  onLogged,
  askedKind,
  frame,
  onlyTasks = false,
  sourceActivityId,
  taskToComplete,
  inheritSourceLinks = false,
}: Readonly<{
  entityType?: EntityKind;
  entityId?: string;
  onLogged?: () => void;
  frame?: (body: ReactNode, submit: ReactNode) => ReactNode;
  // A new requested kind can arrive while the form stays mounted.
  askedKind?: OpeningKind;
  // Opened as Add task: no other kind is offered, so no date stops at today.
  onlyTasks?: boolean;
  sourceActivityId?: string;
  taskToComplete?: Readonly<{ id: string; version?: number }>;
  inheritSourceLinks?: boolean;
}>) {
  const t = useT();
  const toast = useToast();
  const formId = useId();
  const [markDone, setMarkDone] = useState(true);
  const uploadedFiles = useRef(new Set<File>());
  const queryClient = useQueryClient();
  const recordZone = useViewerZone();
  const reportUnattached = useReportUnattached();
  const [draft, setDraft] = useState<ActivityDraft>(() =>
    freshDraft(recordZone, askedKind),
  );
  // Follow a changed request before paint, preserving the words already entered.
  const [lastAsked, setLastAsked] = useState(askedKind);
  if (askedKind !== lastAsked) {
    setLastAsked(askedKind);
    if (askedKind !== undefined) {
      setDraft((current) => ({ ...current, kind: askedKind }));
    }
  }
  const [files, setFiles] = useState<readonly PickedFile[]>([]);
  const custom = useActivityCustomFields(draft.kind);
  const [attendees, setAttendees] = useState<MeetingAttendees>(NO_ATTENDEES);
  const asksAttendees = KINDS_WITH_A_CONTACT.has(draft.kind);
  const needsAttendee = entityType === "company" && asksAttendees;
  // The same test activityRequestBody sends source_system on.
  const transcript = canHaveTranscript(draft.kind) && draft.asTranscript;
  const banded = frame !== undefined;
  const recordOf = useRecordOfMeeting(
    entityType,
    entityId,
    draft,
    recordZone,
    attendees.contacts.map((contact) => contact.id),
  );
  const dealFiling = useLogDealFiling(
    entityType,
    entityId,
    asksAttendees && !recordOf.recordingFiledMeeting && !taskToComplete,
  );
  const bodyRows = transcript ? 10 : banded ? 8 : 3;

  const { key: idempotencyKey, renew: renewKey } = useIdempotencyKey();
  const log = useMutation({
    mutationKey: ["activity-log", entityId],
    // Pass the zone with the mutation so a stale render cannot store the wrong day.
    mutationFn: async (input: {
      draft: ActivityDraft;
      zone: string;
      attendees: MeetingAttendees;
      idempotencyKey: string;
      files: readonly File[];
      record: MeetingRecord | undefined;
      completion: Readonly<{ id: string; version?: number }> | undefined;
      inheritSourceLinks: boolean;
    }) => {
      const recordsMeeting = transcriptRecords(input.record, input.draft);
      if (input.record && !recordsMeeting) {
        const id = await recordOnMeeting(
          { ...input, record: input.record, entityType, entityId },
          t,
        );
        return {
          id,
          unattached: await attachLoggedFiles(id, input.files),
          linkedDealIds: input.record.linkedDealIds ?? [],
        };
      }
      const created = unwrap(
        await api.POST("/activities", {
          params: { header: { "Idempotency-Key": input.idempotencyKey } },
          body: {
            ...activityRequestBody(
              input.draft,
              entityType,
              entityId,
              input.zone,
              input.attendees,
            ),
            ...(input.inheritSourceLinks ? { links: undefined } : {}),
            ...(recordsMeeting ? { records_meeting_id: recordsMeeting } : {}),
          },
        }),
        t,
      );
      // After the create: a file needs the activity's id as its parent.
      const unattached = await attachLoggedFiles(
        created.id,
        input.files,
        uploadedFiles.current,
      );
      if (input.completion && unattached.length === 0) {
        const current = unwrap(
          await api.GET("/activities/{id}", {
            params: { path: { id: input.completion.id } },
          }),
          t,
        );
        if (!current.is_done) {
          unwrap(
            await api.PATCH("/activities/{id}", {
              params: {
                path: { id: input.completion.id },
                ...ifMatch(requireVersion(current.version)),
              },
              body: { is_done: true, version: current.version },
            }),
            t,
          );
        }
      }
      return {
        id: created.id,
        unattached,
        linkedDealIds: (created.links ?? [])
          .filter((link) => link.entity_type === "deal")
          .map((link) => link.entity_id),
      };
    },
    onError: (_error, input) => {
      if (input.completion)
        queryClient.invalidateQueries({
          queryKey: ["activity", input.completion.id],
        });
      recordOf.keep(input.record);
      const filing = input.draft.dealFiling?.context;
      if (filing)
        queryClient.invalidateQueries({
          queryKey: activityDealFilingKey(filing.record_type, filing.record_id),
        });
      if (entityType && entityId) {
        queryClient.invalidateQueries({
          queryKey: openMeetingKey(entityType, entityId),
        });
      }
    },
    onSuccess: (logged, input) => {
      reportUnattached(logged.unattached);
      if (input.completion)
        queryClient.invalidateQueries({
          queryKey: ["activity", input.completion.id],
        });
      recordOf.clear();
      dealFiling.clear();
      for (const queryKey of activityLogKeys(
        logged.id,
        input,
        entityType,
        entityId,
        logged.linkedDealIds,
      ))
        queryClient.invalidateQueries({ queryKey });
      if (input.completion && logged.unattached.length > 0) {
        toast.show(t("tasks.log.leftOpen"), { tone: "danger", sticky: true });
        return;
      }
      setDraft(freshDraft(input.zone, onlyTasks ? "task" : undefined));
      setAttendees(NO_ATTENDEES);
      setFiles([]);
      uploadedFiles.current.clear();
      custom.reset();
      renewKey();
      onLogged?.();
    },
  });

  useUnsavedGuard(draftHoldsWork(draft));

  // Enter in a field submits past the Save button, so the form is latched too.
  const singlePress = useSinglePress(log.isPending);
  const setField = (patch: Partial<ActivityDraft>) =>
    setDraft((current) => ({ ...current, ...patch }));

  const submit = (
    <Button
      variant="primary"
      type="submit"
      form={formId}
      disabled={
        !log.isPending &&
        (dealFiling.blocked ||
          !draftSaysSomething(draft) ||
          (draft.kind !== "task" &&
            draft.followUp !== undefined &&
            !draft.followUp.subject.trim()) ||
          (needsAttendee && attendees.contacts.length === 0))
      }
      reason={dealFiling.reason}
      pending={log.isPending}
      busyLabel={t("log.saving")}
    >
      {t(onlyTasks ? "log.addTask" : "log.save")}
    </Button>
  );
  const completionControl = (
    <TaskCompletionChoice
      enabled={taskToComplete !== undefined}
      checked={markDone}
      onChange={setMarkDone}
      pending={log.isPending}
    />
  );
  const form = (actions: ReactNode) => (
    <form
      id={formId}
      className="form-stack"
      onSubmit={singlePress((event) => {
        event.preventDefault();
        if (dealFiling.blocked || refuseUnreadableField(event.currentTarget)) {
          return;
        }
        log.mutate({
          draft: {
            ...draft,
            dealFiling: dealFiling.request,
            sourceActivityId,
            custom: custom.body(),
          },
          zone: recordZone,
          attendees,
          idempotencyKey,
          files: files.map((held) => held.file),
          record: taskToComplete ? undefined : recordOf.start(),
          completion: markDone ? taskToComplete : undefined,
          inheritSourceLinks,
        });
      })}
    >
      <div className="form-row">
        {!onlyTasks && (
          <ActivityKindField
            value={draft.kind}
            offersTask={!taskToComplete}
            onChange={(kind) => setField({ kind })}
          />
        )}
        <TaskTypeField
          active={draft.kind === "task"}
          value={draft.taskType}
          onChange={(taskType) => setField({ taskType })}
        />
        {/* One date box, prefilled with today: a task's due date, otherwise the
            day it happened (occurredFields), whose hint explains the cap. */}
        <Field
          label={draft.kind === "task" ? t("log.dueAt") : t("log.date")}
          hint={draft.kind === "task" ? undefined : t("log.dateHint")}
        >
          {(control) => (
            <DateInput
              {...control}
              value={isoOrBlank(draft.day)}
              // A note or meeting cannot have happened in the future, and the
              // cap makes the picker say so; a due date has no such ceiling.
              // occurredFields clamps a typed-in future day the same way, on the
              // same clock. So the box refuses the days the submit would move.
              max={latestDayOf(draft.kind, recordZone)}
              today={isoOrBlank(todayDay(draft.kind, recordZone)) || undefined}
              onChange={(event) => setField({ day: event.target.value })}
            />
          )}
        </Field>
        {draft.kind === "task" && (
          <TaskTimeField
            day={draft.day}
            value={draft.time}
            onChange={(time) => setField({ time })}
          />
        )}
      </div>
      {recordOf.question}
      <TaskAssigneeField
        active={draft.kind === "task"}
        value={draft.assigneeId}
        onChange={(value) => setField({ assigneeId: value })}
        records={assignedRecord(entityType, entityId)}
      />
      {asksAttendees && entityType && entityId && (
        <MeetingAttendeeFields
          entityType={entityType}
          entityId={entityId}
          value={attendees}
          onChange={setAttendees}
          disabled={log.isPending}
        />
      )}
      {dealFiling.field}
      {draft.kind === "call" && (
        <CallOutcomeField
          value={draft.callOutcome}
          onChange={(callOutcome) => setField({ callOutcome })}
        />
      )}
      <Field label={t("log.subject")} required={draft.kind !== "call"}>
        {(control) => (
          <TextInput
            {...control}
            value={draft.subject}
            onChange={(event) => setField({ subject: event.target.value })}
          />
        )}
      </Field>
      {canHaveTranscript(draft.kind) && (
        <Checkbox
          label={t("log.asTranscript")}
          checked={draft.asTranscript}
          onChange={(event) => setField({ asTranscript: event.target.checked })}
        />
      )}
      {/* In the drawer the body is the form's subject, so it takes the room
          left under the fields; the standing card keeps it compact. */}
      <ActivityBodyField
        label={transcript ? t("log.transcriptLabel") : t("log.body")}
        hint={transcript ? t("log.transcriptHint") : undefined}
        transcript={transcript}
        rows={bodyRows}
        grow={banded}
        className="logactivity-body-field"
        value={draft.body}
        onChange={(body) => setField({ body })}
      />
      <PolishWording
        words={draft}
        onChange={setField}
        offered={!transcript}
        entityType={entityType}
        entityId={entityId}
      />
      {custom.controls}
      {draft.kind !== "task" && (
        <FollowUpTaskFields
          value={draft.followUp}
          onChange={(followUp) => setField({ followUp })}
          records={assignedRecord(entityType, entityId)}
        />
      )}
      {transcript && (
        <TranscriptUploadField onText={(body) => setField({ body })} />
      )}
      <LogActivityEntries
        onlyTasks={onlyTasks}
        entityType={entityType}
        entityId={entityId}
      />
      <LogActivityFilesField
        files={files}
        onChange={setFiles}
        disabled={log.isPending}
      />
      {completionControl}
      <ErrorLine error={log.error} />
      {actions}
    </form>
  );
  return frame
    ? frame(form(null), submit)
    : form(<div className="form-actions">{submit}</div>);
}

/**
 * LogActivity is the standing composer card the contact and deal screens keep
 * open in their rail.
 */
export function LogActivity({
  entityType,
  entityId,
  onLogged,
  askedKind,
}: Readonly<{
  entityType: EntityKind;
  entityId: string;
  onLogged?: () => void;
  askedKind?: OpeningKind;
}>) {
  const t = useT();
  const me = useMe();
  const canLog = useCanWrite("activity", "create");
  const logRefused = me.data?.authorization !== undefined && !canLog;
  if (logRefused) {
    return (
      <Panel title={t("log.title")}>
        <PanelBody>
          <p>{t("record.logActivityRefused")}</p>
        </PanelBody>
      </Panel>
    );
  }
  return (
    <Panel title={t("log.title")}>
      <PanelBody>
        <LogActivityForm
          entityType={entityType}
          entityId={entityId}
          askedKind={askedKind}
          onLogged={onLogged}
        />
      </PanelBody>
    </Panel>
  );
}
