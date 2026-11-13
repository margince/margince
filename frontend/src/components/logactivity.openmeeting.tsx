// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type QueryKey, useQuery } from "@tanstack/react-query";
import { type ReactNode, useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { ChoiceList } from "../design-system/choicelist";
import { isISODate } from "../design-system/dateinput";
import { dayStartIso } from "../design-system/recordtimeline";
import { activityDate, formatActivityWhen } from "../format/activitywhen";
import type { ISODate } from "../format/datepattern";
import { useLocale, useT } from "../i18n";
import {
  type ActivityDraft,
  activityRequestBody,
  canHaveTranscript,
  NO_ATTENDEES,
  outgoingBody,
} from "../lib/activitybody";
import type { EntityKind } from "../lib/entity";
import { unwrap } from "../lib/problem";
import { useViewerId } from "../lib/session";
import { writeMeetingOutcome } from "./taskactions.rowwrites";

// A meeting logged on a record may be the record of one already booked there.
// Logged as a second meeting, the booked one keeps owing its outcome and a held
// count counts the meeting twice. So the Log form asks, and on Yes writes the
// notes and the outcome on the meeting itself.

type Activity = components["schemas"]["Activity"];

/** The booked meeting a logged meeting may be the record of. */
export type OpenMeeting = Pick<
  Activity,
  | "id"
  | "version"
  | "links"
  | "body"
  | "calendar_managed"
  | "subject"
  | "occurred_at"
  | "occurred_precision"
>;

/** The prefix every open-meeting lookup sits under, for a write to refresh. */
export function openMeetingKey(entityType: EntityKind, entityId: string) {
  return ["activities", "open-meeting", entityType, entityId] as const;
}

/** Who the entry is about, and who is logging it. */
export type MeetingAsker = Readonly<{
  // The contacts picked as attendees on the form.
  attendeeIds: readonly string[];
  // The reader's own user id; undefined while the session is read.
  viewerId: string | undefined;
}>;

type Candidate = Pick<
  Activity,
  | "awaiting_outcome"
  | "writable"
  | "host_user_id"
  | "colleague_ids"
  | "links"
  | "guests"
>;

// The contacts a meeting was with: its contact links and the guests the
// calendar matched to a contact.
function participantsOf(meeting: Candidate): Set<string> {
  const ids = new Set<string>();
  for (const link of meeting.links ?? []) {
    if (link.entity_type === "contact") ids.add(link.entity_id);
  }
  for (const guest of meeting.guests ?? []) {
    if (guest.contact_id) ids.add(guest.contact_id);
  }
  return ids;
}

/**
 * Whether the reader may record this meeting from this form. It must owe its
 * outcome. The reader must host it or attend it as a colleague. It must be
 * with the contact the entry is about: on a contact, that contact itself.
 * A company reaches the meetings of everyone who works there. So on any other
 * record it is a picked attendee, and with none picked nothing is asked.
 */
export function meetingIsAsked(
  meeting: Candidate,
  entityType: EntityKind,
  entityId: string,
  asker: MeetingAsker,
): boolean {
  if (!meeting.awaiting_outcome || !meeting.writable) return false;
  const viewer = asker.viewerId;
  if (
    !viewer ||
    (meeting.host_user_id !== viewer &&
      !(meeting.colleague_ids ?? []).includes(viewer))
  ) {
    return false;
  }
  const participants = participantsOf(meeting);
  return entityType === "contact"
    ? participants.has(entityId)
    : asker.attendeeIds.some((id) => participants.has(id));
}

/**
 * The asked meeting on the chosen day, by calendar day in the reader's zone.
 * The question is about that day, so a meeting the day before or after is not
 * asked. A date-only meeting is stored at noon UTC, so its instant says nothing
 * about the hour. Of several on the day, the earliest wins.
 */
export function closestMeeting<
  T extends Pick<Activity, "occurred_at" | "occurred_precision">,
>(meetings: readonly T[], day: ISODate, zone: string): T | undefined {
  return meetings
    .filter(
      (meeting) =>
        activityDate(meeting.occurred_at, meeting.occurred_precision, zone) ===
        day,
    )
    .sort((a, b) => Date.parse(a.occurred_at) - Date.parse(b.occurred_at))[0];
}

/**
 * The meeting on this record that still owes its outcome and that this reader
 * may record for these contacts, on the chosen day (closestMeeting).
 * `enabled` is false for every case the form does not ask about.
 */
export function useOpenMeetingOnDay(
  entityType: EntityKind | undefined,
  entityId: string | undefined,
  day: string,
  zone: string,
  enabled: boolean,
  asker: MeetingAsker,
): OpenMeeting | undefined {
  const asks =
    enabled &&
    entityType !== undefined &&
    entityId !== undefined &&
    isISODate(day);
  const read = useQuery({
    queryKey: [
      ...(asks
        ? openMeetingKey(entityType, entityId)
        : ["activities", "open-meeting"]),
      day,
    ] as QueryKey,
    enabled: asks,
    queryFn: async () => {
      if (!asks) return [];
      return unwrap(
        await api.GET("/activities", {
          params: {
            query: {
              kind: ["meeting"],
              entity_type: entityType,
              entity_id: entityId,
              occurred_after: dayStartIso(day, zone, -1),
              occurred_before: dayStartIso(day, zone, 2),
            },
          },
        }),
      ).data;
    },
  });
  if (!asks || !read.data || !isISODate(day)) return undefined;
  return closestMeeting(
    read.data.filter((meeting) =>
      meetingIsAsked(meeting, entityType, entityId, asker),
    ),
    day,
    zone,
  );
}

/**
 * Recording an entry on a meeting, step by step. The form keeps it across a
 * failed save, so a retry resumes at the first step not done and never writes
 * a step twice.
 */
export type MeetingRecord = {
  meeting: OpenMeeting;
  // The outcome is written (with the notes, for a meeting the form may edit).
  outcome: boolean;
  linkedDealIds?: string[];
  // The linked note is posted (a calendar-managed meeting only).
  note: boolean;
  // The follow-up task is posted.
  followUp: boolean;
  // The body each POST step first sent. A retry sends it again unchanged, so
  // its key always travels with the same body and the server can replay it.
  sent: Partial<Record<RecordStep, ActivityRequest>>;
};

type RecordStep = "note" | "follow-up";
type ActivityRequest = ReturnType<typeof activityRequestBody>;

/**
 * The open meeting this entry records, and the question that asks. Yes is the
 * default, and a different meeting asks again from Yes. A save that wrote the
 * outcome and then failed swaps the question for a line saying so. The next
 * save resumes that record. A transcript is saved in one create, so it never
 * resumes one.
 */
export function useRecordOfMeeting(
  entityType: EntityKind | undefined,
  entityId: string | undefined,
  draft: ActivityDraft,
  zone: string,
  attendeeIds: readonly string[],
): Readonly<{
  recordingFiledMeeting: boolean;
  start: () => MeetingRecord | undefined;
  keep: (record: MeetingRecord | undefined) => void;
  clear: () => void;
  question: ReactNode;
}> {
  const viewerId = useViewerId();
  const [resume, setResume] = useState<MeetingRecord>();
  const meetingKind = draft.kind === "meeting";
  const open = useOpenMeetingOnDay(
    entityType,
    entityId,
    draft.day,
    zone,
    meetingKind && !resume,
    { attendeeIds, viewerId },
  );
  const [notRecordOf, setNotRecordOf] = useState<string | undefined>();
  const keep = (record: MeetingRecord | undefined) =>
    setResume(
      record?.outcome ? { ...record, sent: { ...record.sent } } : undefined,
    );
  const clear = () => setResume(undefined);
  if (resume && meetingKind && !draft.asTranscript) {
    return {
      recordingFiledMeeting: Boolean(
        resume.meeting.links?.some((link) =>
          ["deal", "project", "lead"].includes(link.entity_type),
        ),
      ),
      start: () => ({ ...resume, sent: { ...resume.sent } }),
      keep,
      clear,
      question: <RecordedMeetingLine meeting={resume.meeting} />,
    };
  }
  if (!open)
    return {
      recordingFiledMeeting: false,
      start: () => undefined,
      keep,
      clear,
      question: null,
    };
  const recordsIt = notRecordOf !== open.id;
  return {
    recordingFiledMeeting:
      recordsIt &&
      Boolean(
        open.links?.some((link) =>
          ["deal", "project", "lead"].includes(link.entity_type),
        ),
      ),
    start: () =>
      recordsIt
        ? {
            meeting: open,
            outcome: false,
            note: false,
            followUp: false,
            sent: {},
          }
        : undefined,
    keep,
    clear,
    question: (
      <RecordOfMeetingQuestion
        meeting={open}
        value={recordsIt}
        onChange={(yes) => setNotRecordOf(yes ? undefined : open.id)}
        zone={zone}
        transcript={draft.asTranscript}
      />
    ),
  };
}

function RecordedMeetingLine({ meeting }: Readonly<{ meeting: OpenMeeting }>) {
  const t = useT();
  return (
    <p className="t-caption" role="status">
      {t("log.recordOfMeetingRecorded", {
        subject: meeting.subject || t("log.recordOfMeetingUntitled"),
      })}
    </p>
  );
}

/** Is this entry the record of the open meeting? Yes, unless the reader says no. */
export function RecordOfMeetingQuestion({
  meeting,
  value,
  onChange,
  zone,
  transcript = false,
}: Readonly<{
  meeting: OpenMeeting;
  value: boolean;
  onChange: (recordsIt: boolean) => void;
  zone: string;
  // The entry is a transcript, which is saved as its own entry either way.
  transcript?: boolean;
}>) {
  const t = useT();
  const { locale } = useLocale();
  return (
    <>
      <ChoiceList
        legend={t("log.recordOfMeeting", {
          subject: meeting.subject || t("log.recordOfMeetingUntitled"),
          time: formatActivityWhen(
            meeting.occurred_at,
            meeting.occurred_precision,
            locale,
            zone,
          ),
        })}
        value={value ? "yes" : "no"}
        onChange={(answer) => onChange(answer === "yes")}
        choices={[
          { value: "yes", label: t("log.recordOfMeetingYes") },
          { value: "no", label: t("log.recordOfMeetingNo") },
        ]}
      />
      {value && transcript && (
        <p className="t-caption">{t("log.recordOfMeetingTranscript")}</p>
      )}
      {value && !transcript && meeting.calendar_managed && (
        <p className="t-caption">{t("log.recordOfMeetingManaged")}</p>
      )}
    </>
  );
}

/**
 * The meeting a transcript records, when the entry is one. A transcript stays
 * its own entry, so its create names the meeting rather than writing on it.
 */
export function transcriptRecords(
  record: MeetingRecord | undefined,
  draft: ActivityDraft,
): string | undefined {
  return record && canHaveTranscript(draft.kind) && draft.asTranscript
    ? record.meeting.id
    : undefined;
}

/** The notes a meeting keeps: its own, then the typed ones after a blank line. */
export function appendedNotes(
  existing: string | null | undefined,
  typed: string,
): string | null {
  const notes = outgoingBody(typed, false);
  if (!notes) return existing ?? null;
  return existing?.trim() ? `${existing.trimEnd()}\n\n${notes}` : notes;
}

/**
 * Records the entry on the open meeting; the picked files attach to its id.
 * The version-pinned outcome goes first, so a refused version writes nothing.
 * Each later POST has its own key, the form's key plus the step name. A retry
 * resends the body that step first sent, so a key never meets a new body.
 * `record` marks each step as it lands; a retry sends only what is missing.
 */
export async function recordOnMeeting(
  input: Readonly<{
    record: MeetingRecord;
    draft: ActivityDraft;
    entityType: EntityKind | undefined;
    entityId: string | undefined;
    zone: string;
    idempotencyKey: string;
  }>,
  t: Parameters<typeof writeMeetingOutcome>[1],
): Promise<string> {
  const { record, draft } = input;
  const { meeting } = record;
  const managed = meeting.calendar_managed === true;
  const bodyOf = (entry: ActivityDraft) =>
    activityRequestBody(
      entry,
      input.entityType,
      input.entityId,
      input.zone,
      NO_ATTENDEES,
    );
  const post = async (step: RecordStep, entry: ActivityDraft) => {
    const body = record.sent[step] ?? bodyOf(entry);
    record.sent[step] = body;
    unwrap(
      await api.POST("/activities", {
        params: {
          header: { "Idempotency-Key": `${input.idempotencyKey}:${step}` },
        },
        body,
      }),
      t,
    );
  };
  if (!record.outcome) {
    const updated = await writeMeetingOutcome(
      {
        id: meeting.id,
        version: meeting.version,
        status: "held",
        ...(managed
          ? {}
          : {
              body: appendedNotes(meeting.body, draft.body),
              aiAssisted: draft.aiAssisted,
            }),
        // The fields typed on the form describe the meeting they record.
        custom: draft.custom,
        dealFiling: draft.dealFiling?.context,
      },
      t,
    );
    record.linkedDealIds = (updated?.links ?? [])
      .filter((link) => link.entity_type === "deal")
      .map((link) => link.entity_id);
    record.outcome = true;
  }
  if (managed && !record.note) {
    // The calendar keeps the meeting's own notes, so the typed ones become a
    // note. No follow_up_task on it: the task's source would be the note.
    await post("note", {
      ...draft,
      kind: "note",
      followUp: undefined,
      custom: undefined,
    });
    record.note = true;
  }
  if (draft.followUp && !record.followUp) {
    await post("follow-up", {
      ...draft.followUp,
      kind: "task",
      // What the server reads for an untyped task a rep files.
      taskType: "reach_out",
      asTranscript: false,
      callOutcome: "",
      sourceActivityId: meeting.id,
    });
    record.followUp = true;
  }
  return meeting.id;
}
