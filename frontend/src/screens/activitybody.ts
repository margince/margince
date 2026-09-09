// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { EntityKind } from "../app/entity";
import type { RecordPickerCandidate } from "../design-system/recordpicker";
import { calendarDay, dueInstant, middayInstant } from "../format/calendarday";

// One drafted entry and the wire body it becomes: the shape the composer holds
// and the `POST /activities` payload it projects to. The React form that edits
// the draft lives in logactivity.tsx; this is the projection alone, so a test
// can pin the body the server sees without mounting a component.

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
};

// A meeting and a call are WITH A PERSON, and the server refuses either one
// filed against a company — per link, so naming the company alongside the
// person is refused too, and the company is reached through the attendee's
// employer instead (activities/activitylinks.go, migration
// "a meeting is with a person again").
//
// So a form opened on a company has to ask WHO was in the room before it can
// send one of these kinds at all. It offered no way to say, and the reader met
// a 422 with no field to correct.
export const KINDS_WITH_A_PERSON = new Set(["meeting", "call"]);

// The instant a logged activity carries. The picked day left on today — or, for
// a note, pushed into the future, which nothing can have occurred in — means the
// actual moment of logging, so entries logged in sequence keep their timeline
// order. A backdated day becomes that day's noon in the record zone. Either way the
// entry files under the day the writer picked, because both branches and the
// timeline's day headings read the same clock. A task's picked day is its DUE
// date instead — the task itself occurred now.
export function occurredInstant(
  input: ActivityDraft,
  recordZone: string,
): string {
  const now = new Date();
  const today = calendarDay(now, recordZone);
  if (input.kind === "task" || input.day === "" || input.day >= today) {
    return now.toISOString();
  }
  return middayInstant(input.day, recordZone);
}

// The wire body one drafted entry becomes.
export function activityRequestBody(
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
    // A due date becomes the instant that day ENDS on the RECORD's clock
    // (format/calendarday), which is the same zone the worklist buckets
    // overdue in and the same one the task detail renders. Minting it in the
    // writer's own zone instead is what let an approved 9 September come back
    // as a task due the 10th for a colleague sitting further east.
    ...(input.kind === "task" && input.day
      ? { due_at: dueInstant(input.day, recordZone) }
      : {}),
    // Only a task is held by somebody, and only when the writer named them:
    // an unassigned task is a legitimate landing state (the worklist scopes
    // unowned work on its own), so "" sends no field rather than a null the
    // form would have to invent.
    ...(input.kind === "task" && input.assigneeId
      ? { assignee_id: input.assigneeId }
      : {}),
    // Held: a hand-logged meeting already took place (the date caps at
    // today), and held is what the lead ladder reads as engagement.
    ...(input.kind === "meeting" ? { meeting_status: "held" as const } : {}),
    ...(isTranscript ? { source_system: "transcript" } : {}),
    // The attendee REPLACES the company link rather than joining it. The
    // server refuses an organization link on a meeting or a call whichever
    // else are present, and the company still reaches the activity: the
    // employer walk carries it there through the person who was named.
    //
    // Only for the kinds that ask for one. The picker stops rendering when the
    // reader switches to a note or a task, but the person they had already
    // chosen stays in state — and filing a company note against that person
    // takes it off the company screen it was written on.
    links:
      attendee && KINDS_WITH_A_PERSON.has(input.kind)
        ? [{ entity_type: "person" as const, entity_id: attendee.id }]
        : [{ entity_type: entityType, entity_id: entityId }],
    source: "manual",
  };
}
