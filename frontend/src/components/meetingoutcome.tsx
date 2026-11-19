// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { QueryKey } from "@tanstack/react-query";
import { useState } from "react";
import type { components } from "../api/schema";
import { Button } from "../design-system/atoms";
import { undoAction, useToast } from "../design-system/toast";
import { useT } from "../i18n";
import { useCanWriteRecord } from "../lib/capability";
import { isVersionSkewOf, problemFieldErrorsOf } from "../lib/problem";
import { worklistKey } from "../lib/workkeys";
import {
  type MeetingDialogMode,
  MeetingOutcomeDialog,
} from "./meetingoutcome.dialog";
import { readMeetingStatus, useMeetingOutcome } from "./taskactions";

const readPrior = (id: string) => readMeetingStatus(id).catch(() => null);

type Activity = components["schemas"]["Activity"];

/**
 * The meeting verbs on a record page. The outcome choice shows while the
 * server stamps the meeting as awaiting one; its notes show once it was held.
 */
export function RecordMeetingOutcome({
  activity,
  invalidateKeys,
}: Readonly<{
  activity: Activity;
  invalidateKeys: readonly QueryKey[];
}>) {
  const writable = useCanWriteRecord("activity", activity);
  if (!writable) {
    return null;
  }
  if (!activity.awaiting_outcome) {
    return (
      <HeldMeetingNotes activity={activity} invalidateKeys={invalidateKeys} />
    );
  }
  return (
    <MeetingOutcome
      id={activity.id}
      version={activity.version}
      title={activity.subject ?? undefined}
      cancellable={activity.calendar_managed !== true}
      priorStatus={activity.meeting_status === "booked" ? "booked" : null}
      // The Worklist asks about the same meeting, so an answer here clears it there.
      invalidateKeys={[...invalidateKeys, worklistKey]}
    />
  );
}

/**
 * "Add notes" or "Edit notes" on a held meeting, so what was discussed can be
 * written the day after. A live invitation keeps the meeting's notes, and the
 * server refuses them from here, so such a meeting offers none.
 */
function HeldMeetingNotes({
  activity,
  invalidateKeys,
}: Readonly<{
  activity: Activity;
  invalidateKeys: readonly QueryKey[];
}>) {
  const t = useT();
  const [open, setOpen] = useState(false);
  if (
    activity.kind !== "meeting" ||
    activity.meeting_status !== "held" ||
    activity.calendar_managed === true ||
    activity.content_state === "withheld"
  ) {
    return null;
  }
  const title = activity.subject ?? undefined;
  const edits = Boolean(activity.body?.trim());
  return (
    <>
      <Button
        aria-label={
          title
            ? t(
                edits
                  ? "worklist.verb.meetingEditNotesFor"
                  : "worklist.verb.meetingAddNotesFor",
                { subject: title },
              )
            : undefined
        }
        onClick={() => setOpen(true)}
      >
        {t(
          edits
            ? "worklist.verb.meetingEditNotes"
            : "worklist.verb.meetingAddNotes",
        )}
      </Button>
      {open && (
        <MeetingOutcomeDialog
          id={activity.id}
          title={title}
          invalidateKeys={invalidateKeys}
          mode="notes"
          onClose={() => setOpen(false)}
        />
      )}
    </>
  );
}

/** The shared outcome controls for the Worklist and meeting panel. */
export function MeetingOutcome({
  id,
  version,
  title,
  invalidateKeys,
  cancellable,
  priorStatus,
}: Readonly<{
  id: string;
  version: number | undefined;
  // What the meeting holds now, for the Undo of a cancel. Left out where the
  // surface does not know it, and read before the cancel then.
  priorStatus?: "booked" | null;
  // The reads that show this meeting, so each surface refreshes its own list.
  invalidateKeys: readonly QueryKey[];
  // The row's own heading for the meeting, so the dialog is titled before its read answers.
  title: string | undefined;
  // False where a live invitation holds the meeting: the server refuses the cancel.
  cancellable: boolean;
}>) {
  const t = useT();
  const toast = useToast();
  const [open, setOpen] = useState<MeetingDialogMode | undefined>();
  // An invitation can arrive after the row was read, so its refusal hides Cancel too.
  const [refusedAsManaged, setRefusedAsManaged] = useState(false);
  const [reading, setReading] = useState(false);
  const record = useMeetingOutcome(invalidateKeys);
  const cancel = async () => {
    // The row may not carry the status, and an Undo that guesses would write
    // "booked" over a meeting that had none.
    setReading(true);
    const prior = priorStatus === undefined ? await readPrior(id) : priorStatus;
    setReading(false);
    record.mutate(
      { id, version, status: "canceled" },
      {
        onSuccess: (saved) =>
          toast.show(t("worklist.verb.meetingCanceledRecorded"), {
            action: undoAction(t("common.undo"), () =>
              // The version is the one the cancel produced. The row may be
              // gone by then, so a refusal is told here.
              record
                .mutateAsync({ id, version: saved.version, status: prior })
                .catch((error: unknown) =>
                  toast.show(
                    t(
                      isVersionSkewOf(error)
                        ? "common.versionSkew"
                        : "worklist.verb.meetingOutcomeFailed",
                    ),
                    { tone: "danger" },
                  ),
                ),
            ),
          }),
        // A refused write leaves the row as it was, which looks like a
        // click that did nothing.
        onError: (error) => {
          const managed = problemFieldErrorsOf(error).some(
            (fault) => fault.code === "calendar_managed",
          );
          setRefusedAsManaged(managed);
          toast.show(
            t(
              managed
                ? "worklist.verb.meetingCancelInCalendar"
                : "worklist.verb.meetingOutcomeFailed",
            ),
            { tone: "danger" },
          );
        },
      },
    );
  };
  return (
    <>
      <span className="t-caption">{t("worklist.verb.meetingHowDidItGo")}</span>
      {/* Only "Did not happen" writes from here; the other two open a
          dialog, so they wait on its write without showing it as theirs. */}
      <Button
        disabled={record.isPending}
        aria-label={
          title
            ? t("worklist.verb.meetingHeldFor", { subject: title })
            : undefined
        }
        onClick={() => setOpen("held")}
      >
        {t("worklist.verb.meetingHeld")}
      </Button>
      <Button
        disabled={record.isPending}
        aria-label={
          title
            ? t("worklist.verb.meetingAddNotesFor", { subject: title })
            : undefined
        }
        onClick={() => setOpen("edit")}
      >
        {t("worklist.verb.meetingAddNotes")}
      </Button>
      {cancellable && !refusedAsManaged && (
        <Button
          pending={record.isPending || reading}
          aria-label={
            title
              ? t("worklist.verb.meetingCanceledFor", { subject: title })
              : undefined
          }
          onClick={() => void cancel()}
        >
          {t("worklist.verb.meetingCanceled")}
        </Button>
      )}
      {open && (
        <MeetingOutcomeDialog
          id={id}
          title={title}
          invalidateKeys={invalidateKeys}
          mode={open}
          onClose={() => setOpen(undefined)}
        />
      )}
    </>
  );
}
