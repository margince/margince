// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type QueryKey, useQuery } from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { ChoiceList } from "../design-system/choicelist";
import { isISODate } from "../design-system/dateinput";
import { dayStartIso } from "../design-system/recordtimeline";
import { formatActivityWhen } from "../format/activitywhen";
import { useLocale, useT } from "../i18n";
import {
  type ActivityDraft,
  activityRequestBody,
  NO_ATTENDEES,
  outgoingBody,
} from "../lib/activitybody";
import type { EntityKind } from "../lib/entity";
import { unwrap } from "../lib/problem";
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

/**
 * The meeting on this record that still owes its outcome and that the reader
 * may answer. Of those within a day of the chosen day, the closest wins.
 * `enabled` is false for every case the form does not ask about.
 */
export function useOpenMeetingOnDay(
  entityType: EntityKind | undefined,
  entityId: string | undefined,
  day: string,
  zone: string,
  enabled: boolean,
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
  if (!asks || !read.data) return undefined;
  const target = Date.parse(dayStartIso(day, zone));
  return read.data
    .filter((meeting) => meeting.awaiting_outcome && meeting.writable)
    .sort(
      (a, b) =>
        Math.abs(Date.parse(a.occurred_at) - target) -
        Math.abs(Date.parse(b.occurred_at) - target),
    )[0];
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
 * Records the typed entry on the open meeting, and returns the activity the
 * picked files belong to.
 *
 * A meeting the calendar manages keeps its own subject and notes. So the notes
 * become a note on the record, written first and under the form's key. A retry
 * after a refused version then replays that note rather than making a second.
 * Every other meeting takes the notes after its own, with its outcome, in one
 * write.
 */
export async function recordOnMeeting(
  input: Readonly<{
    meeting: OpenMeeting;
    draft: ActivityDraft;
    entityType: EntityKind | undefined;
    entityId: string | undefined;
    zone: string;
    idempotencyKey: string;
  }>,
  t: Parameters<typeof writeMeetingOutcome>[1],
): Promise<string> {
  const { meeting, draft } = input;
  const post = async (body: ReturnType<typeof activityRequestBody>) =>
    unwrap(
      await api.POST("/activities", {
        params: { header: { "Idempotency-Key": input.idempotencyKey } },
        body,
      }),
      t,
    ).id;
  if (meeting.calendar_managed) {
    const note = await post(
      activityRequestBody(
        { ...draft, kind: "note" },
        input.entityType,
        input.entityId,
        input.zone,
        NO_ATTENDEES,
      ),
    );
    await writeMeetingOutcome(
      { id: meeting.id, version: meeting.version, status: "held" },
      t,
    );
    return note;
  }
  await writeMeetingOutcome(
    {
      id: meeting.id,
      version: meeting.version,
      status: "held",
      body: appendedNotes(meeting.body, draft.body),
    },
    t,
  );
  if (draft.followUp) {
    await post(
      activityRequestBody(
        {
          ...draft.followUp,
          kind: "task",
          asTranscript: false,
          sourceActivityId: meeting.id,
        },
        input.entityType,
        input.entityId,
        input.zone,
        NO_ATTENDEES,
      ),
    );
  }
  return meeting.id;
}
