// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import type { RecordPickerCandidate } from "../design-system/recordpicker";
import {
  dayOccurredAt,
  latestHeldMoment,
  type OccurredPrecision,
} from "../format/activitywhen";
import { calendarDay, dueInstant } from "../format/calendarday";
import type { EntityKind } from "./entity";
import type { TaskType } from "./tasktype";

// One drafted entry and the wire body it becomes: the shape the composer holds
// and the `POST /activities` payload it projects to. The React form that edits
// the draft lives in logactivity.tsx; this is the projection alone, so a test
// can pin the body the server sees without mounting a component.

export function canHaveTranscript(kind: ActivityDraft["kind"]): boolean {
  return kind === "call" || kind === "meeting";
}

/** Whether a value the kind picker handed back is a kind the draft holds. */
export function isActivityKind(value: string): value is ActivityDraft["kind"] {
  return (
    value === "note" ||
    value === "task" ||
    value === "call" ||
    value === "meeting"
  );
}

/** How a call ended, from the contract's fixed list. */
export type CallOutcome = NonNullable<
  components["schemas"]["Activity"]["call_outcome"]
>;

/** The outcomes in the order a rep reads them. */
export const CALL_OUTCOMES: readonly CallOutcome[] = [
  "reached",
  "not_reached",
  "voicemail",
  "call_back_later",
  "put_through",
];

export type ActivityDealSelection = Readonly<{
  context: components["schemas"]["ActivityDealFiling"];
  dealId?: string;
}>;

export type ActivityDraft = {
  dealFiling?: ActivityDealSelection;
  kind: "note" | "task" | "meeting" | "call";
  subject: string;
  body: string;
  // yyyy-mm-dd from the date input. Its meaning follows the kind: a task's
  // due date, otherwise the day the note or meeting happened.
  day: string;
  // A task's callback time, `HH:MM` on the record's clock, "" for none. It
  // means nothing without a day.
  time: string;
  // Transcript text is opt-in because its processing and retention differ from ordinary notes.
  asTranscript: boolean;
  // Who this task is for, "" for nobody. Meaningless outside kind: task — a
  // note or meeting is not held by a colleague — so it only reaches the wire on
  // a task, the same way `day` only becomes a due date there.
  assigneeId: string;
  // The kind of work a task asks for, which decides its one button. Reaches
  // the wire on a task only; a follow-up keeps the type its maker reads.
  taskType: TaskType;
  // How a call ended, "" for no outcome. Reaches the wire on a call only, so a
  // draft switched from Call to Note sends none.
  callOutcome: CallOutcome | "";
  sourceActivityId?: string;
  followUp?: TaskDraft;
  // The workspace's custom-field values, as the create body's cf_ keys.
  custom?: Record<string, unknown>;
};

export type TaskDraft = Pick<
  ActivityDraft,
  "subject" | "body" | "day" | "time" | "assigneeId"
>;

export function taskRequestFields(input: TaskDraft, recordZone: string) {
  return {
    subject: input.subject.trim(),
    body: outgoingBody(input.body, false),
    ...(input.day ? taskDueFields(input.day, input.time, recordZone) : {}),
    ...(input.assigneeId ? { assignee_id: input.assigneeId } : {}),
  };
}

/**
 * The due fields of a task picked as a day and an optional time of day. The
 * flag says which one it is; the server reads it, never the clock. A task with
 * a time also reminds its assignee then, and that rule is the server's, for
 * every writer, so this sends no reminder of its own.
 */
export function taskDueFields(
  day: string,
  time: string,
  recordZone: string,
): { due_at: string; due_has_time: boolean } {
  return {
    due_at: dueInstant(day, recordZone, time),
    due_has_time: time !== "",
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

// The server's cap on a send's `links`; activitybody.linkcap.test.ts holds it.
export const MAX_ACTIVITY_LINKS = 25;

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
  // A meeting goes out as held, so it reads its today off the held clock.
  const now =
    input.kind === "meeting" ? latestHeldMoment(new Date()) : new Date();
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
  const isTranscript = canHaveTranscript(input.kind) && input.asTranscript;
  return {
    kind: input.kind,
    source_activity_id: input.sourceActivityId,
    ...callAndMeetingFields(input),
    ...occurredFields(input, recordZone),
    ...(input.kind === "task"
      ? {
          ...taskRequestFields(input, recordZone),
          task_type: input.taskType,
        }
      : {
          subject: input.subject.trim(),
          body: outgoingBody(input.body, isTranscript),
          ...(input.followUp
            ? { follow_up_task: taskRequestFields(input.followUp, recordZone) }
            : {}),
        }),
    ...(isTranscript ? { source_system: "transcript" } : {}),
    // Only for the kinds that ask who attended. Switching to a note hides the
    // fields but keeps their picks in state. A note must not be filed against them.
    links:
      entityType && entityId
        ? withAttendees
          ? meetingLinks(
              entityType,
              entityId,
              attendees.contacts,
              input.dealFiling?.dealId,
            )
          : [{ entity_type: entityType, entity_id: entityId }]
        : [],
    ...(withAttendees && attendees.colleagueIds.length > 0
      ? { attendee_user_ids: [...attendees.colleagueIds] }
      : {}),
    ...input.custom,
    source: "manual",
  };
}

function callAndMeetingFields(input: ActivityDraft) {
  return {
    ...(KINDS_WITH_A_CONTACT.has(input.kind) && input.dealFiling
      ? { deal_filing: input.dealFiling.context }
      : {}),
    // Held: a hand-logged meeting already took place (the date caps at
    // today), and held is what the lead ladder reads as engagement.
    ...(input.kind === "meeting" ? { meeting_status: "held" as const } : {}),
    // A call always names its outcome; "No outcome" is an explicit null.
    ...(input.kind === "call"
      ? { call_outcome: input.callOutcome === "" ? null : input.callOutcome }
      : {}),
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
  dealId?: string,
): { entity_type: EntityKind; entity_id: string }[] {
  const record = { entity_type: entityType, entity_id: entityId };
  const links = entityType === "company" ? [] : [record];
  for (const contact of contacts) {
    if (!(entityType === "contact" && contact.id === entityId)) {
      links.push({ entity_type: "contact", entity_id: contact.id });
    }
  }
  if (dealId) links.push({ entity_type: "deal", entity_id: dealId });
  return links.length > 0 ? links : [record];
}

const EMPTY_DRAFT: ActivityDraft = {
  kind: "note",
  subject: "",
  body: "",
  day: "",
  time: "",
  asTranscript: false,
  assigneeId: "",
  // A task a rep files on a record is most often a mail to write.
  taskType: "write",
  callOutcome: "",
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

/**
 * Whether the draft says what happened. A call does so by its outcome or its
 * body alone; every other kind needs a subject. The server's
 * CheckEntryHasContent holds the same rule for a call.
 */
export function draftSaysSomething(draft: ActivityDraft): boolean {
  if (draft.subject.trim() !== "") {
    return true;
  }
  return (
    draft.kind === "call" &&
    (draft.callOutcome !== "" || draft.body.trim() !== "")
  );
}

/**
 * Whether closing the form would lose work: typed words, or a call outcome
 * the rep picked. The kind and day are a pick and are not counted.
 */
export function draftHoldsWork(draft: ActivityDraft): boolean {
  return (
    draft.subject.trim() !== "" ||
    draft.body.trim() !== "" ||
    (draft.kind === "call" && draft.callOutcome !== "")
  );
}
