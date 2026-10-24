import { useMutation, useQueryClient } from "@tanstack/react-query";
import { type ReactNode, useId, useState } from "react";
import { api } from "../api/client";
import { useIdempotencyKey } from "../api/idempotencykey";
import {
  Button,
  Checkbox,
  Field,
  Modal,
  TextInput,
} from "../design-system/atoms";
import {
  DrawerBody,
  DrawerFoot,
  DrawerHead,
} from "../design-system/drawerbands";
import { ErrorLine } from "../design-system/errorline";
import { Heading } from "../design-system/heading";
import { Panel, PanelBody } from "../design-system/panel";
import { useSinglePress } from "../design-system/presslatch";
import { Select } from "../design-system/select";
import { useT } from "../i18n";
import {
  type ActivityDraft,
  activityRequestBody,
  freshDraft,
  isActivityKind,
  KINDS_WITH_A_CONTACT,
  type MeetingAttendees,
  NO_ATTENDEES,
  todayDay,
} from "../lib/activitybody";
import {
  activityFileStaleKeys,
  activityFollowUpsKey,
  loggedActivityKeys,
  TASK_QUEUE_KEY,
} from "../lib/activitykeys";
import { useCanWrite } from "../lib/capability";
import type { EntityKind } from "../lib/entity";
import { unwrap } from "../lib/problem";
import { useRecordZone } from "../lib/recordzone";
import { useMe } from "../lib/session";
import { worklistKey } from "../lib/workkeys";
import { ActivityBodyField } from "./activitybodyfield";
import { TaskAssigneeField } from "./assigneepicker";
import { LogActivityEntries } from "./extensionentries";
import "./logactivity.css";
import { FollowUpTaskFields } from "./followuptaskfields";
import { useActivityCustomFields } from "./logactivity.custom";
import {
  attachLoggedFiles,
  LogActivityFilesField,
  type PickedFile,
  TranscriptUploadField,
  useReportUnattached,
} from "./logactivityfiles";
import { MeetingAttendeeFields } from "./meetingattendees";
import { useUnsavedGuard } from "./unsaved";

// Meetings arrive through the calendar rather than a preselected form address.
type OpeningKind = "note" | "task" | "call";

// One form serves record cards and drawers; banded callers own its frame.
export function LogActivityForm({
  entityType,
  entityId,
  onLogged,
  askedKind,
  frame,
  onlyTasks = false,
  sourceActivityId,
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
}>) {
  const t = useT();
  const formId = useId();
  const queryClient = useQueryClient();
  const recordZone = useRecordZone();
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
  const custom = useActivityCustomFields();
  // Who was in the room, asked for the kinds that are with a contact (see
  // KINDS_WITH_A_CONTACT). Only a company page cannot send one without a name.
  const [attendees, setAttendees] = useState<MeetingAttendees>(NO_ATTENDEES);
  const asksAttendees = KINDS_WITH_A_CONTACT.has(draft.kind);
  const needsAttendee = entityType === "company" && asksAttendees;
  // The same test activityRequestBody sends source_system on.
  const transcript = draft.kind === "meeting" && draft.asTranscript;
  const banded = frame !== undefined;
  const bodyRows = transcript ? 10 : banded ? 8 : 3;

  const { key: idempotencyKey, renew: renewKey } = useIdempotencyKey();
  const log = useMutation({
    // The parent record is already named in the cache.
    mutationKey: ["activity-log", entityId],
    // Pass the zone with the mutation so a stale render cannot store the wrong day.
    mutationFn: async (input: {
      draft: ActivityDraft;
      zone: string;
      attendees: MeetingAttendees;
      idempotencyKey: string;
      files: readonly File[];
    }) => {
      const created = unwrap(
        await api.POST("/activities", {
          params: { header: { "Idempotency-Key": input.idempotencyKey } },
          body: activityRequestBody(
            input.draft,
            entityType,
            entityId,
            input.zone,
            input.attendees,
          ),
        }),
        t,
      );
      // After the create: a file needs the activity's id as its parent.
      const unattached = await attachLoggedFiles(created.id, input.files);
      return { id: created.id, unattached };
    },
    onSuccess: (logged, input) => {
      reportUnattached(logged.unattached);
      const keys =
        entityType && entityId
          ? loggedActivityKeys(
              input.draft.followUp && input.draft.kind !== "task"
                ? "task"
                : input.draft.kind,
              entityType,
              entityId,
              input.attendees.contacts.map((contact) => contact.id),
            )
          : [TASK_QUEUE_KEY];
      if (input.draft.sourceActivityId)
        keys.push(activityFollowUpsKey(input.draft.sourceActivityId));
      keys.push(worklistKey);
      if (input.files.length > 0)
        keys.push(...activityFileStaleKeys(logged.id));
      for (const queryKey of keys) queryClient.invalidateQueries({ queryKey });
      setDraft(freshDraft(input.zone, onlyTasks ? "task" : undefined));
      setAttendees(NO_ATTENDEES);
      setFiles([]);
      custom.reset();
      renewKey();
      onLogged?.();
    },
  });

  // The words are the work a reader would retype; the kind and day are a pick.
  useUnsavedGuard(draft.subject.trim() !== "" || draft.body.trim() !== "");

  // Enter in a field submits past the Save button, so the form is latched too.
  const singlePress = useSinglePress(log.isPending);
  const setField = (patch: Partial<ActivityDraft>) =>
    setDraft((current) => ({ ...current, ...patch }));

  const submit = (
    <Button
      variant="primary"
      type="submit"
      form={formId}
      // An unnamed attendee is refused by the server with a 422 the reader
      // cannot act on from here, so the button says no first. Never
      // auto-selecting the company's first contact to make the submit work:
      // filing a meeting against somebody who was not there is worse.
      disabled={
        !log.isPending &&
        (!draft.subject.trim() ||
          (draft.kind !== "task" &&
            draft.followUp !== undefined &&
            !draft.followUp.subject.trim()) ||
          (needsAttendee && attendees.contacts.length === 0))
      }
      pending={log.isPending}
      busyLabel={t("log.saving")}
    >
      {t(onlyTasks ? "log.addTask" : "log.save")}
    </Button>
  );
  const form = (actions: ReactNode) => (
    <form
      id={formId}
      className="form-stack"
      onSubmit={singlePress((event) => {
        event.preventDefault();
        log.mutate({
          draft: { ...draft, sourceActivityId, custom: custom.body() },
          zone: recordZone,
          attendees,
          idempotencyKey,
          files: files.map((held) => held.file),
        });
      })}
    >
      <div className="form-row">
        {!onlyTasks && (
          <Field label={t("log.kind")}>
            {(control) => (
              <Select
                {...control}
                options={[
                  { value: "note", label: t("log.kindNote") },
                  { value: "task", label: t("log.kindTask") },
                  { value: "call", label: t("log.kindCall") },
                  { value: "meeting", label: t("log.kindMeeting") },
                ]}
                value={draft.kind}
                // The clamp narrows the type and validates nothing. The Select
                // only hands back the values above; anything else falls back
                // to the ordinary kind rather than a refusal.
                onChange={(value) =>
                  setField({
                    kind: isActivityKind(value) ? value : "note",
                  })
                }
              />
            )}
          </Field>
        )}
        {/* One date box, prefilled with today: a task's due date, otherwise the
            day it happened (occurredInstant), whose hint explains the cap. */}
        <Field
          label={draft.kind === "task" ? t("log.dueAt") : t("log.date")}
          hint={draft.kind === "task" ? undefined : t("log.dateHint")}
        >
          {(control) => (
            <TextInput
              {...control}
              type="date"
              value={draft.day}
              // A note or meeting cannot have happened in the future, and the
              // cap makes the picker say so; a due date has no such ceiling.
              // occurredInstant clamps a typed-in future day the same way, and
              // against the same clock, so the box refuses exactly the days the
              // submit would have moved.
              max={
                draft.kind === "task"
                  ? undefined
                  : todayDay(draft.kind, recordZone)
              }
              onChange={(event) => setField({ day: event.target.value })}
              // A native date input opens its calendar only from the tiny
              // icon; a click on the value just places a caret. Opening on
              // any click is what a writer reaching for "the date" expects.
              onClick={(event) => event.currentTarget.showPicker?.()}
            />
          )}
        </Field>
      </div>
      {/* WHO owes this task, asked beside when it is due. Only a task is held
          by a colleague; the field renders nothing for a note or meeting, and
          defers its roster walk to that same condition. */}
      <TaskAssigneeField
        active={draft.kind === "task"}
        value={draft.assigneeId}
        onChange={(value) => setField({ assigneeId: value })}
      />
      {/* WHO was in the room, asked before what was said. A meeting or a call
          is with a contact and the server refuses one filed against a company,
          so on a company this is the field that decides whether the entry can
          be sent at all — not a refinement of one that could. */}
      {asksAttendees && entityType && entityId && (
        <MeetingAttendeeFields
          entityType={entityType}
          entityId={entityId}
          value={attendees}
          onChange={setAttendees}
          disabled={log.isPending}
        />
      )}
      <Field label={t("log.subject")} required>
        {(control) => (
          <TextInput
            {...control}
            value={draft.subject}
            onChange={(event) => setField({ subject: event.target.value })}
          />
        )}
      </Field>
      {draft.kind === "meeting" && (
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
      {custom.controls}
      {draft.kind !== "task" && (
        <FollowUpTaskFields
          value={draft.followUp}
          onChange={(followUp) => setField({ followUp })}
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
  // The kind the card opens on, for a reader who arrived at an address that
  // named one. The card is standing rather than opened, so bringing it into
  // view is the rest of what "arrive ready" means.
  askedKind?: OpeningKind;
}>) {
  const t = useT();
  // useCanWrite, not useCan: a read seat is refused the POST before RBAC. The
  // card says so rather than vanishing, and only once /me has answered.
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

/**
 * LogActivityAction is the same composer as a header button, for a screen
 * whose header strip is a row of actions rather than a column of cards.
 */
export function LogActivityAction({
  entityType,
  entityId,
  askedKind,
  openOnMount,
  triggerIcon,
  disabled,
  disabledReasonId,
  onClose,
  onLogged,
  about,
}: Readonly<{
  entityType: EntityKind;
  entityId: string;
  // A suggestion opens the kind it already asked for.
  askedKind?: OpeningKind;
  // Rendered already open, with no trigger button — for a caller that IS the
  // trigger (a suggestion's action), rather than a toolbar offering the verb.
  openOnMount?: boolean;
  // The glyph beside the trigger's words, `aria-hidden` at the call site: a
  // verb is found by its shape first. Optional: `openOnMount` draws no trigger.
  triggerIcon?: ReactNode;
  // Blocks the press while carrying no explanation — for a caller whose grant
  // has not resolved yet. Claiming a refusal the server has not decided is
  // worse than a control that is briefly quiet; separate from
  // `disabledReasonId` so a caller can hold the two apart.
  disabled?: boolean;
  // The sentence that refuses this verb, already on the page. A record that
  // takes no new activity must still SHOW the verb it will not accept — a
  // reader who cannot tell "this record is archived" from "this build has no
  // such button" learns nothing from the absence.
  disabledReasonId?: string;
  onClose?: () => void;
  // Only a successful write asks the caller to refresh; dismissal does not.
  onLogged?: () => void;
  // The record named under the title, where the page behind does not show it.
  about?: ReactNode;
}>) {
  const t = useT();
  const titleId = useId();
  const aboutId = useId();
  // Asked for a task, this is Add task: trigger, heading and form all say so.
  const addTask = askedKind === "task";
  const verb = addTask ? "log.addTask" : "log.title";
  const [open, setOpen] = useState(Boolean(openOnMount));
  const close = () => {
    setOpen(false);
    onClose?.();
  };
  return (
    <>
      {!openOnMount && (
        <Button
          disabled={disabled}
          reasonId={disabledReasonId}
          onClick={() => setOpen(true)}
        >
          {triggerIcon}
          {t(verb)}
        </Button>
      )}
      <Modal
        open={open}
        onClose={close}
        labelledBy={titleId}
        describedBy={about ? aboutId : undefined}
        intent="drawer"
      >
        <DrawerHead className="logactivity-head">
          <Heading size="large" id={titleId} className="t-h2">
            {/* The heading answers the verb that opened it: a reader who
                pressed "Add task" reads "Log activity" as the wrong dialog. */}
            {t(verb)}
          </Heading>
          {about && (
            <p id={aboutId} className="t-caption">
              {about}
            </p>
          )}
        </DrawerHead>
        <LogActivityForm
          entityType={entityType}
          entityId={entityId}
          askedKind={askedKind}
          frame={(body, submit) => (
            <>
              <DrawerBody className="logactivity-drawer-body">
                {body}
              </DrawerBody>
              <DrawerFoot className="actions">{submit}</DrawerFoot>
            </>
          )}
          onlyTasks={addTask}
          onLogged={() => {
            onLogged?.();
            close();
          }}
        />
      </Modal>
    </>
  );
}
