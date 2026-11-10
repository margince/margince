import type { QueryKey } from "@tanstack/react-query";
import type { ActivityDraft, MeetingAttendees } from "../lib/activitybody";
import {
  activityFileStaleKeys,
  activityFollowUpsKey,
  entityTimelineKeys,
  loggedActivityKeys,
  TASK_QUEUE_KEY,
} from "../lib/activitykeys";
import type { EntityKind } from "../lib/entity";
import { worklistKey } from "../lib/workkeys";
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
  if (input.files.length > 0) keys.push(...activityFileStaleKeys(loggedId));
  for (const id of linkedDealIds) {
    if (entityType !== "deal" || id !== entityId)
      keys.push(...entityTimelineKeys("deal", id));
  }
  keys.push(worklistKey);
  return keys;
}
