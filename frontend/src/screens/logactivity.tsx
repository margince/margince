import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useId, useState } from "react";
import { api } from "../api/client";
import type { EntityKind } from "../app/entity";
import { useRecordZone } from "../app/recordzone";
import {
  Button,
  Card,
  Checkbox,
  Field,
  Modal,
  Textarea,
  TextInput,
} from "../design-system/atoms";
import {
  RecordPicker,
  type RecordPickerCandidate,
} from "../design-system/recordpicker";
import { Select } from "../design-system/select";
import { calendarDay, dueInstant, middayInstant } from "../format/calendarday";
import { viewerZone } from "../format/timezone";
import { useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { entityTimelineKeys, taskWriteKeys } from "./activitykeys";
import { problemMessageOf, throwProblem, useSorMode } from "./common";

// Log a note or task from a 360 (person/company/deal/lead): the contract's
// logActivity POST, linked to the record being viewed, occurred_at stamped
// at submit, source=manual. On success every read that renders this record's
// timeline is invalidated (see activitykeys) so the fresh entry appears
// without a reload. Server-side validation is the truth — a 422 renders its
// RFC 7807 detail verbatim.

type ActivityDraft = {
  kind: "note" | "task" | "meeting" | "call";
  subject: string;
  body: string;
  // yyyy-mm-dd from the date input. Its meaning follows the kind: a task's
  // due date, otherwise the day the note or meeting happened.
  day: string;
  // A meeting's body is ordinary notes UNLESS this is explicitly checked —
  // otherwise "discussed pricing, follow up Tuesday" typed while logging a
  // meeting would silently carry source_system: transcript, which the
  // backend documents as meaning pasted/uploaded transcript TEXT and which
  // the activity/transcript retention scope sweeps on a different schedule
  // than an ordinary meeting note. Meaningless outside kind: meeting.
  asTranscript: boolean;
};

const EMPTY_DRAFT: ActivityDraft = {
  kind: "note",
  subject: "",
  body: "",
  day: "",
  asTranscript: false,
};

// Only a plain-text paste round-trips through normalizeTranscript's line
// splitting the way ADR-0058's line-addressing promises: a `.vtt` file's cue
// timestamps and header would themselves become "transcript lines", pointing
// any future line citation at a timestamp instead of what was said.
const ACCEPTED_TRANSCRIPT_EXTENSION = ".txt";

// The kinds this form starts on when a caller already knows which one the
// reader asked for. `meeting` is absent on purpose: a meeting arrives through
// the calendar rather than as an address somebody follows.
type OpeningKind = "note" | "task" | "call";

// "Today", in the zone the picked day will be READ back in. The composer's date
// field starts here — the day that WOULD apply is shown where the writer can
// change it instead of being assumed at submit behind an empty box.
//
// Which zone that is follows what the day MEANS, the same split the draft's
// `day` field carries. A note or meeting files under a heading on the record's
// timeline, grouped in the record zone, so the day it can be offered is the record's
// today; a task's day is a personal due date, minted by `dueInstant` in the
// browser's zone and rendered there, so its today is the writer's own. Offer a
// day from the other zone and the composer names a day the entry does not land
// on: an afternoon in Los Angeles is already tomorrow on a Berlin clock, so a
// writer offered their own today, accepting it, watched the entry file under the
// day after.
function todayDay(kind: ActivityDraft["kind"], recordZone: string): string {
  return calendarDay(new Date(), kind === "task" ? viewerZone() : recordZone);
}

// Whether a Select's answer is a kind this form writes.
function isKind(value: string): value is ActivityDraft["kind"] {
  return (
    value === "note" ||
    value === "task" ||
    value === "call" ||
    value === "meeting"
  );
}

function freshDraft(
  recordZone: string,
  kind: ActivityDraft["kind"] = "note",
): ActivityDraft {
  return { ...EMPTY_DRAFT, kind, day: todayDay(kind, recordZone) };
}

// The instant a logged activity carries. The picked day left on today — or, for
// a note, pushed into the future, which nothing can have occurred in — means the
// actual moment of logging, so entries logged in sequence keep their timeline
// order. A backdated day becomes that day's noon in the record zone. Either way the
// entry files under the day the writer picked, because both branches and the
// timeline's day headings read the same clock. A task's picked day is its DUE
// date instead — the task itself occurred now.
function occurredInstant(input: ActivityDraft, recordZone: string): string {
  const now = new Date();
  const today = calendarDay(now, recordZone);
  if (input.kind === "task" || input.day === "" || input.day >= today) {
    return now.toISOString();
  }
  return middayInstant(input.day, recordZone);
}

// A meeting and a call are WITH A PERSON, and the server refuses either one
// filed against a company — per link, so naming the company alongside the
// person is refused too, and the company is reached through the attendee's
// employer instead (activities/activitylinks.go, migration
// "a meeting is with a person again").
//
// So a form opened on a company has to ask WHO was in the room before it can
// send one of these kinds at all. It offered no way to say, and the reader met
// a 422 with no field to correct.
const KINDS_WITH_A_PERSON = new Set(["meeting", "call"]);

// The company's own contacts, narrowed by what the reader typed.
//
// Scoped to the company rather than searching every person in the installation:
// the question is who from THIS account was in the room, and an unscoped search
// would offer contacts of other companies as equally likely answers to it.
async function searchCompanyContacts(
  organizationID: string,
  q: string,
): Promise<RecordPickerCandidate[]> {
  const { data, error } = await api.GET("/people", {
    params: { query: { organization_id: organizationID, q, limit: 20 } },
  });
  if (error) {
    throwProblem(error);
  }
  return data.data.map((person) => ({
    id: person.id,
    // full_name, which the contract documents as always present. display_name
    // belongs to a USER; a person has neither the field nor a fallback for it.
    name: person.full_name,
  }));
}

// The wire body one drafted entry becomes.
function activityRequestBody(
  input: ActivityDraft,
  entityType: EntityKind,
  entityId: string,
  recordZone: string,
  // Who was in the room, when the form is open on a company and the kind is one
  // that needs a person. Null everywhere else.
  attendee: RecordPickerCandidate | null,
) {
  // source_system: transcript is what routes the body through the
  // server's ADR-0058 normalizer and what the activity/transcript
  // retention scope keys its sweep on (see backend logActivity's
  // `transcript` example) — only when the writer has explicitly marked
  // this text as one (asTranscript), never inferred from kind: meeting
  // alone, or ordinary meeting notes would carry a marker meaning
  // something else and sweep on a different retention schedule.
  const isTranscript = input.kind === "meeting" && input.asTranscript;
  // A transcript is sent RAW, not trimmed: the server's normalizer
  // (transcriptnorm.go) is the one place line-1-indexing gets decided,
  // and it only trims trailing whitespace per line — a leading blank
  // line or leading indentation the client stripped first would make a
  // transcript pasted here normalize to different stored text (and
  // different line numbers) than the identical paste sent by an agent
  // or another client straight to the API.
  const outgoingBody = isTranscript ? input.body : input.body.trim();
  return {
    kind: input.kind,
    subject: input.subject.trim(),
    body: outgoingBody || null,
    occurred_at: occurredInstant(input, recordZone),
    // A due date becomes the instant that day ENDS in the writer's
    // own zone (format/calendarday). Handing the bare `yyyy-mm-dd` to
    // `new Date` reads it as UTC midnight instead, which is neither the
    // end of the day nor, west of UTC, the day the writer picked: the task
    // arrived already overdue, and the tasks list — which buckets in the
    // reader's zone — filed it under yesterday.
    ...(input.kind === "task" && input.day
      ? { due_at: dueInstant(input.day) }
      : {}),
    // Held: a hand-logged meeting already took place (the date caps at
    // today), and held is what the lead ladder reads as engagement.
    ...(input.kind === "meeting" ? { meeting_status: "held" as const } : {}),
    ...(isTranscript ? { source_system: "transcript" } : {}),
    // The attendee REPLACES the company link rather than joining it. The
    // server refuses an organization link on a meeting or a call whichever
    // else are present, and the company still reaches the activity: the
    // employer walk carries it there through the person who was named.
    links: attendee
      ? [{ entity_type: "person" as const, entity_id: attendee.id }]
      : [{ entity_type: entityType, entity_id: entityId }],
    source: "manual",
  };
}

/**
 * LogActivityForm is the composer itself, without a frame, so the same fields
 * serve the standing card on the person and deal screens and the modal the
 * company screen opens.
 */
export function LogActivityForm({
  entityType,
  entityId,
  onLogged,
  askedKind,
}: Readonly<{
  entityType: EntityKind;
  entityId: string;
  onLogged?: () => void;
  // The kind the reader asked for, when the caller knows which one. Absent
  // means note, the ordinary case. It is the kind ASKED rather than the kind
  // the form starts on: it may change while this form stands.
  askedKind?: OpeningKind;
}>) {
  const t = useT();
  const queryClient = useQueryClient();
  const recordZone = useRecordZone();
  const [draft, setDraft] = useState<ActivityDraft>(() =>
    freshDraft(recordZone, askedKind),
  );
  // An initializer runs once, and the ask can arrive later: a reader already
  // reading this record presses a second link that names a kind, the address
  // changes under a form that never unmounted, and the initializer's kind is
  // what they keep looking at. So the ask is FOLLOWED, not merely seeded.
  //
  // Only the kind moves, exactly as far as the reader's own Select moves it,
  // so a subject half-typed against the old kind survives — and it moves
  // during render rather than in an effect, so the stale kind is never painted.
  const [lastAsked, setLastAsked] = useState(askedKind);
  if (askedKind !== lastAsked) {
    setLastAsked(askedKind);
    if (askedKind !== undefined) {
      setDraft((current) => ({ ...current, kind: askedKind }));
    }
  }
  const [fileError, setFileError] = useState<string | null>(null);
  // Who was in the room. Only ever asked on a company, and only for the kinds
  // that are with a person — see KINDS_WITH_A_PERSON.
  const [attendee, setAttendee] = useState<RecordPickerCandidate | null>(null);
  const needsAttendee =
    entityType === "organization" && KINDS_WITH_A_PERSON.has(draft.kind);

  const log = useMutation({
    // Keyed on entityId, the record this form is open on, not the created
    // activity's own id: the reader wants the name of what the activity is
    // ABOUT, and the record on screen is always already named in the cache.
    mutationKey: ["activity-log", entityId],
    // The zone travels as a VARIABLE rather than being read from the closure.
    // A mutationFn closing over render state runs against whatever the last
    // committed render held, and what this one decides with the zone is the
    // instant the entry is STORED at — so a stale read would not misdraw a
    // page, it would file the activity on the wrong day, permanently.
    mutationFn: async (input: {
      draft: ActivityDraft;
      zone: string;
      attendee: RecordPickerCandidate | null;
    }) => {
      const { data, error } = await api.POST("/activities", {
        body: activityRequestBody(
          input.draft,
          entityType,
          entityId,
          input.zone,
          input.attendee,
        ),
      });
      if (error) {
        throwProblem(error, t);
      }
      return data;
    },
    onSuccess: (_data, input) => {
      const keys =
        input.draft.kind === "task"
          ? taskWriteKeys(entityType, entityId)
          : entityTimelineKeys(entityType, entityId);
      for (const queryKey of keys) {
        queryClient.invalidateQueries({ queryKey });
      }
      // The attendee's OWN timeline too. The activity is filed against the
      // person, so the company screen this form sits on reaches it through the
      // employer walk while the person's page holds it directly — and a reader
      // who logs a meeting here and opens the contact expects to find it.
      if (input.attendee) {
        for (const queryKey of entityTimelineKeys(
          "person",
          input.attendee.id,
        )) {
          queryClient.invalidateQueries({ queryKey });
        }
      }
      setDraft(freshDraft(input.zone));
      setAttendee(null);
      onLogged?.();
    },
  });

  const setField = (patch: Partial<ActivityDraft>) =>
    setDraft((current) => ({ ...current, ...patch }));

  return (
    <form
      className="form-stack"
      onSubmit={(event) => {
        event.preventDefault();
        log.mutate({ draft, zone: recordZone, attendee });
      }}
    >
      <div className="form-row">
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
              // The clamp is the narrowing, not a validation: the Select can
              // only hand back one of the values above, and anything else is
              // the ordinary kind rather than a refusal.
              onChange={(value) =>
                setField({
                  kind: isKind(value) ? value : "note",
                })
              }
            />
          )}
        </Field>
        {/* One date box for every kind, prefilled with today, so nothing about
            the entry's date is assumed invisibly. The picked day carries what
            the kind needs it to: a task's due date, otherwise the day the
            note or meeting happened (occurredInstant). */}
        <Field label={draft.kind === "task" ? t("log.dueAt") : t("log.date")}>
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
      {/* WHO was in the room, asked before what was said. A meeting or a call
          is with a person and the server refuses one filed against a company,
          so on a company this is the field that decides whether the entry can
          be sent at all — not a refinement of one that could. */}
      {needsAttendee && (
        <RecordPicker
          label={t("log.attendee")}
          searchTargets={(q) => searchCompanyContacts(entityId, q)}
          selected={attendee}
          onPick={setAttendee}
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
      <Field
        label={draft.asTranscript ? t("log.transcriptLabel") : t("log.body")}
        hint={draft.asTranscript ? t("log.transcriptHint") : undefined}
      >
        {(control) => (
          <Textarea
            {...control}
            rows={draft.asTranscript ? 10 : 3}
            value={draft.body}
            onChange={(event) => setField({ body: event.target.value })}
          />
        )}
      </Field>
      {draft.kind === "meeting" && draft.asTranscript && (
        <Field label={t("log.transcriptUpload")} hint={fileError ?? undefined}>
          {(control) => (
            <TextInput
              {...control}
              type="file"
              accept={ACCEPTED_TRANSCRIPT_EXTENSION}
              onChange={async (event) => {
                const file = event.target.files?.[0];
                event.target.value = "";
                if (!file) {
                  return;
                }
                if (
                  !file.name
                    .toLowerCase()
                    .endsWith(ACCEPTED_TRANSCRIPT_EXTENSION)
                ) {
                  setFileError(t("log.transcriptUploadRejected"));
                  return;
                }
                try {
                  const text = await file.text();
                  setFileError(null);
                  setField({ body: text });
                } catch {
                  setFileError(t("log.transcriptUploadFailed"));
                }
              }}
            />
          )}
        </Field>
      )}
      {log.isError && (
        <p className="t-caption form-error">{problemMessageOf(log.error, t)}</p>
      )}
      <div className="form-actions">
        <Button
          small
          variant="primary"
          type="submit"
          // An unnamed attendee is refused by the server with a 422 the reader
          // cannot act on from here, so the button says no first. Never
          // auto-selecting the company's first contact to make the submit
          // work: filing a meeting against somebody who was not there is worse
          // than refusing to file it.
          disabled={
            !log.isPending &&
            (!draft.subject.trim() || (needsAttendee && !attendee))
          }
          pending={log.isPending}
          busyLabel={t("log.saving")}
        >
          {t("log.save")}
        </Button>
      </div>
    </form>
  );
}

/**
 * LogActivity is the standing composer card the person and deal screens keep
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
  // Logging an activity writes to a mirrored record; in overlay every write
  // answers unsupported_by_sor, so the form would only fail on submit. Guarded
  // to render nothing rather than an affordance that can't work (P1/A107,
  // ADR-0018).
  const overlay = useSorMode() === "overlay";
  if (overlay) {
    return null;
  }
  return (
    <Card className="card-stack" title={t("log.title")} sub={t("log.sub")}>
      <LogActivityForm
        entityType={entityType}
        entityId={entityId}
        askedKind={askedKind}
        onLogged={onLogged}
      />
    </Card>
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
  triggerLabel,
  disabled,
  disabledReasonId,
  onClose,
}: Readonly<{
  entityType: EntityKind;
  entityId: string;
  // The kind the form starts on. A suggestion that says "no task says what
  // happens next" opens straight onto a task rather than making the reader
  // pick the kind the advice already named.
  askedKind?: OpeningKind;
  // Rendered already open, with no trigger button — for a caller that IS the
  // trigger (a suggestion's action), rather than a toolbar offering the verb.
  openOnMount?: boolean;
  // What the trigger says. A header offering two ways into this form — log
  // what happened, and set what happens next — needs each button to name its
  // own verb; two buttons both reading "Log activity" is a toolbar that has
  // stopped telling the reader anything.
  triggerLabel?: MessageKey;
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
}>) {
  const t = useT();
  const titleId = useId();
  const [open, setOpen] = useState(Boolean(openOnMount));
  const overlay = useSorMode() === "overlay";
  const close = () => {
    setOpen(false);
    onClose?.();
  };
  if (overlay) {
    return null;
  }
  return (
    <>
      {!openOnMount && (
        <Button
          small
          disabled={disabled}
          reasonId={disabledReasonId}
          onClick={() => setOpen(true)}
        >
          {t(triggerLabel ?? "log.title")}
        </Button>
      )}
      <Modal open={open} onClose={close} labelledBy={titleId}>
        <h2 id={titleId} className="t-h2 modal-title">
          {/* The heading answers the verb that opened it. Titled "log an
              activity" regardless, a reader who pressed "Add task" was shown
              a different form's name and read it as the wrong dialog. */}
          {t(triggerLabel ?? "log.title")}
        </h2>
        <LogActivityForm
          entityType={entityType}
          entityId={entityId}
          askedKind={askedKind}
          onLogged={close}
        />
      </Modal>
    </>
  );
}
