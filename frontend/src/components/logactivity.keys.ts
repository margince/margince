import type { QueryKey } from "@tanstack/react-query";
import type { ActivityDraft, MeetingAttendees } from "../lib/activitybody";
import {
  activityFollowUpsKey,
  entityTimelineKeys,
  loggedActivityKeys,
  TASK_QUEUE_KEY,
} from "../lib/activitykeys";
import { fileListKeys } from "../lib/attachmentkeys";
import type { EntityKind } from "../lib/entity";
import { worklistKey } from "../lib/workkeys";
import { activityDealFilingKey } from "./logactivity.dealfiling";
import { type MeetingRecord, openMeetingKey } from "./logactivity.openmeeting";

// Every read a logged entry makes stale.
export function activityLogKeys(
  loggedId: string,
  input: Readonly<{
    draft: ActivityDraft;
    attendees: MeetingAttendees;
    files: readonly File[];
    record: MeetingRecord | undefined;
  }>,
  entityType: EntityKind | undefined,
  entityId: string | undefined,
  linkedDealIds: readonly string[],
): QueryKey[] {
  const { draft } = input;
  const keys: QueryKey[] =
    entityType && entityId
      ? [
          ...loggedActivityKeys(
            draft.followUp && draft.kind !== "task" ? "task" : draft.kind,
            entityType,
            entityId,
            input.attendees.contacts.map((contact) => contact.id),
          ),
          openMeetingKey(entityType, entityId),
        ]
      : [TASK_QUEUE_KEY];
  if (draft.sourceActivityId)
    keys.push(activityFollowUpsKey(draft.sourceActivityId));
  if (input.record) keys.push(["activity", input.record.meeting.id]);
  if (input.files.length > 0)
    keys.push(...fileListKeys({ entityType: "activity", entityId: loggedId }));
  for (const id of linkedDealIds) {
    if (entityType !== "deal" || id !== entityId)
      keys.push(...entityTimelineKeys("deal", id));
  }
  keys.push(worklistKey);
  return keys;
}

// The reads a failed log may have left stale. The task, the deal filing and
// the open meeting can each have moved on under the form.
export function failedLogKeys(
  input: Readonly<{
    draft: ActivityDraft;
    completion: Readonly<{ id: string }> | undefined;
  }>,
  entityType: EntityKind | undefined,
  entityId: string | undefined,
): QueryKey[] {
  const filing = input.draft.dealFiling?.context;
  return [
    ...(input.completion ? [["activity", input.completion.id]] : []),
    ...(filing
      ? [activityDealFilingKey(filing.record_type, filing.record_id)]
      : []),
    ...(entityType && entityId ? [openMeetingKey(entityType, entityId)] : []),
  ];
}
