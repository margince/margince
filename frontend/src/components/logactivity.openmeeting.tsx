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
import { DAY_MS } from "../format/timezone";
import { useLocale, useT } from "../i18n";
import {
  type ActivityDraft,
  activityRequestBody,
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

// Whole calendar days between two ISO days.
function daysApart(a: string, b: string): number {
  return (
    Math.abs(Date.parse(`${a}T00:00:00Z`) - Date.parse(`${b}T00:00:00Z`)) /
    DAY_MS
  );
}

/**
 * The asked meeting closest to the chosen day. Closeness is by calendar day in
 * the reader's zone first. A date-only meeting is stored at noon UTC, so its
 * instant says nothing about the hour. The instant only breaks a tie.
 */
export function closestMeeting<
  T extends Pick<Activity, "occurred_at" | "occurred_precision">,
>(meetings: readonly T[], day: ISODate, zone: string): T | undefined {
  const target = Date.parse(dayStartIso(day, zone));
  const rank = (meeting: T) => ({
    days: daysApart(
      activityDate(meeting.occurred_at, meeting.occurred_precision, zone),
      day,
    ),
    ms: Math.abs(Date.parse(meeting.occurred_at) - target),
  });
  return [...meetings]
    .map((meeting) => ({ meeting, rank: rank(meeting) }))
    .sort((a, b) => a.rank.days - b.rank.days || a.rank.ms - b.rank.ms)[0]
    ?.meeting;
}

/**
 * The meeting on this record that still owes its outcome and that this reader
 * may record for these contacts. Of those within a day of the chosen day, the
 * closest wins. `enabled` is false for every case the form does not ask about.
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
              kind: "meeting",
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
 * The open meeting this entry records, and the question that asks. A transcript
 * is not asked about: a write to the meeting cannot mark its notes as one. Yes
 * is the default, and a different meeting asks again from Yes. A save that
 * wrote the outcome and then failed swaps the question for a line saying so.
 * The next save resumes that record.
 */
export function useRecordOfMeeting(
  entityType: EntityKind | undefined,
  entityId: string | undefined,
  draft: ActivityDraft,
  zone: string,
  attendeeIds: readonly string[],
): Readonly<{
  start: () => MeetingRecord | undefined;
  keep: (record: MeetingRecord | undefined) => void;
  clear: () => void;
  question: ReactNode;
}> {
  const viewerId = useViewerId();
  const [resume, setResume] = useState<MeetingRecord>();
  const meetingKind = draft.kind === "meeting" && !draft.asTranscript;
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
  if (resume && meetingKind) {
    return {
      start: () => ({ ...resume, sent: { ...resume.sent } }),
      keep,
      clear,
      question: <RecordedMeetingLine meeting={resume.meeting} />,
    };
  }
  if (!open) return { start: () => undefined, keep, clear, question: null };
  const recordsIt = notRecordOf !== open.id;
  return {
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
}: Readonly<{
  meeting: OpenMeeting;
  value: boolean;
  onChange: (recordsIt: boolean) => void;
  zone: string;
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
      {value && meeting.calendar_managed && (
        <p className="t-caption">{t("log.recordOfMeetingManaged")}</p>
      )}
    </>
  );
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
    await writeMeetingOutcome(
      {
        id: meeting.id,
        version: meeting.version,
        status: "held",
        ...(managed ? {} : { body: appendedNotes(meeting.body, draft.body) }),
      },
      t,
    );
    record.outcome = true;
  }
  if (managed && !record.note) {
    // The calendar keeps the meeting's own notes, so the typed ones become a
    // note. No follow_up_task on it: the task's source would be the note.
    await post("note", { ...draft, kind: "note", followUp: undefined });
    record.note = true;
  }
  if (draft.followUp && !record.followUp) {
    await post("follow-up", {
      ...draft.followUp,
      kind: "task",
      asTranscript: false,
      sourceActivityId: meeting.id,
    });
    record.followUp = true;
  }
  return meeting.id;
}
