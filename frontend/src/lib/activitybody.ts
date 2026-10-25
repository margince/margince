// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { RecordPickerCandidate } from "../design-system/recordpicker";
import { dayOccurredAt, type OccurredPrecision } from "../format/activitywhen";
import { calendarDay, dueInstant } from "../format/calendarday";
import type { EntityKind } from "./entity";

// One drafted entry and the wire body it becomes: the shape the composer holds
// and the `POST /activities` payload it projects to. The React form that edits
// the draft lives in logactivity.tsx; this is the projection alone, so a test
// can pin the body the server sees without mounting a component.

/** Whether a value the kind picker handed back is a kind the draft holds. */
export function isActivityKind(value: string): value is ActivityDraft["kind"] {
  return (
    value === "note" ||
    value === "task" ||
    value === "call" ||
    value === "meeting"
  );
}

export type ActivityDraft = {
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
  // Who this task is for, "" for nobody. Meaningless outside kind: task — a
  // note or meeting is not held by a colleague — so it only reaches the wire on
  // a task, the same way `day` only becomes a due date there.
  assigneeId: string;
  sourceActivityId?: string;
  followUp?: TaskDraft;
  // The workspace's custom-field values, as the create body's cf_ keys.
  custom?: Record<string, unknown>;
};

export type TaskDraft = Pick<
  ActivityDraft,
  "subject" | "body" | "day" | "assigneeId"
>;

export function taskRequestFields(input: TaskDraft, recordZone: string) {
  return {
    subject: input.subject.trim(),
    body: outgoingBody(input.body, false),
    ...(input.day ? { due_at: dueInstant(input.day, recordZone) } : {}),
    ...(input.assigneeId ? { assignee_id: input.assigneeId } : {}),
  };
}

// Who was in a meeting or call besides whoever logs it: contacts on
// their side, colleagues on ours.
export type MeetingAttendees = {
  contacts: readonly RecordPickerCandidate[];
  colleagueIds: readonly string[];
};

export const NO_ATTENDEES: MeetingAttendees = {
  contacts: [],
  colleagueIds: [],
};

// A meeting and a call are WITH A CONTACT, and the server refuses either one
// filed against a company — per link, so naming the company alongside the
// contact is refused too, and the company is reached through the attendee's
// employer instead (activities/activitylinks.go, migration
// "a meeting is with a contact again").
//
// So a form opened on a company has to ask WHO was in the room before it can
// send one of these kinds at all. It offered no way to say, and the reader met
// a 422 with no field to correct.
export const KINDS_WITH_A_CONTACT = new Set(["meeting", "call"]);

// The time a logged activity carries. A day left on today, or a note's future
// day, means the moment of logging, so entries keep their timeline order. A
// backdated day is sent as a `day`, since nobody said at what time. Today is the
// record zone's. A task's picked day is its due date; the task occurred now.
export function occurredFields(
  input: ActivityDraft,
  recordZone: string,
): { occurred_at: string; occurred_precision: OccurredPrecision } {
  const now = new Date();
  const today = calendarDay(now, recordZone);
  if (input.kind === "task" || input.day === "" || input.day >= today) {
    return { occurred_at: now.toISOString(), occurred_precision: "instant" };
  }
  return { occurred_at: dayOccurredAt(input.day), occurred_precision: "day" };
}

// The body as it goes on the wire, for every form that writes one. Empty is
// null. A transcript is sent untrimmed, because the server's normalizer
// (transcriptnorm.go) numbers its lines. A leading blank line stripped here
// would shift the line numbers a citation points at.
export function outgoingBody(body: string, transcript: boolean): string | null {
  return (transcript ? body : body.trim()) || null;
}

// The wire body one drafted entry becomes.
export function activityRequestBody(
  input: ActivityDraft,
  entityType: EntityKind | undefined,
  entityId: string | undefined,
  recordZone: string,
  // Who was in the room. Read only for the kinds in KINDS_WITH_A_CONTACT.
  attendees: MeetingAttendees,
) {
  const withAttendees = KINDS_WITH_A_CONTACT.has(input.kind);
  // source_system: transcript is what routes the body through the
  // server's ADR-0058 normalizer and what the activity/transcript
  // retention scope keys its sweep on (see backend logActivity's
  // `transcript` example) — only when the writer has explicitly marked
  // this text as one (asTranscript), never inferred from kind: meeting
  // alone, or ordinary meeting notes would carry a marker meaning
  // something else and sweep on a different retention schedule.
  const isTranscript = input.kind === "meeting" && input.asTranscript;
  return {
    kind: input.kind,
    ...occurredFields(input, recordZone),
    ...(input.kind === "task"
      ? {
          ...taskRequestFields(input, recordZone),
          source_activity_id: input.sourceActivityId,
        }
      : {
          subject: input.subject.trim(),
          body: outgoingBody(input.body, isTranscript),
          ...(input.followUp
            ? { follow_up_task: taskRequestFields(input.followUp, recordZone) }
            : {}),
        }),
    // Held: a hand-logged meeting already took place (the date caps at
    // today), and held is what the lead ladder reads as engagement.
    ...(input.kind === "meeting" ? { meeting_status: "held" as const } : {}),
    ...(isTranscript ? { source_system: "transcript" } : {}),
    // Only for the kinds that ask who attended. Switching to a note hides the
    // fields but keeps their picks in state. A note must not be filed against them.
    links:
      entityType && entityId
        ? withAttendees
          ? meetingLinks(entityType, entityId, attendees.contacts)
          : [{ entity_type: entityType, entity_id: entityId }]
        : [],
    ...(withAttendees && attendees.colleagueIds.length > 0
      ? { attendee_user_ids: [...attendees.colleagueIds] }
      : {}),
    ...input.custom,
    source: "manual",
  };
}

// One contact link per attendee, beside the open record. On a company the
// attendees REPLACE its link, which the server refuses on a meeting or a call.
// The company still reaches the entry through each attendee's employer. With
// nobody named the record link stands alone; on a company, Save is disabled.
function meetingLinks(
  entityType: EntityKind,
  entityId: string,
  contacts: readonly RecordPickerCandidate[],
): { entity_type: EntityKind; entity_id: string }[] {
  const record = { entity_type: entityType, entity_id: entityId };
  const links = entityType === "company" ? [] : [record];
  for (const contact of contacts) {
    if (!(entityType === "contact" && contact.id === entityId)) {
      links.push({ entity_type: "contact", entity_id: contact.id });
    }
  }
  return links.length > 0 ? links : [record];
}

const EMPTY_DRAFT: ActivityDraft = {
  kind: "note",
  subject: "",
  body: "",
  day: "",
  asTranscript: false,
  assigneeId: "",
};

// The record zone keeps the entered day aligned with the stored deadline.
export function todayDay(
  _kind: ActivityDraft["kind"],
  recordZone: string,
): string {
  return calendarDay(new Date(), recordZone);
}

export function freshDraft(
  recordZone: string,
  kind: ActivityDraft["kind"] = "note",
): ActivityDraft {
  return { ...EMPTY_DRAFT, kind, day: todayDay(kind, recordZone) };
}
