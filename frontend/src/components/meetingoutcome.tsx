// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// "Held" and "Mark as canceled" write the status from the card. "Add notes"
// opens a dialog that PATCHES the existing meeting, because a second activity
// would leave the original unanswered.

import { type QueryKey, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import type { components } from "../api/schema";
import { Button } from "../design-system/atoms";
import { useToast } from "../design-system/toast";
import { useT } from "../i18n";
import { useCanWrite, useCanWriteRecord } from "../lib/capability";
import { problemFieldErrorsOf } from "../lib/problem";
import { worklistKey } from "../lib/workkeys";
import { MeetingNextStep } from "./meetingnextstep";
import { MeetingOutcomeDialog } from "./meetingoutcome.dialog";
import { useMeetingOutcome } from "./taskactions";

type Activity = components["schemas"]["Activity"];

/** The outcome choice on a record page, for a meeting the server stamps as awaiting one. */
export function RecordMeetingOutcome({
  activity,
  invalidateKeys,
}: Readonly<{
  activity: Activity;
  invalidateKeys: readonly QueryKey[];
}>) {
  const writable = useCanWriteRecord("activity", activity);
  if (!(activity.awaiting_outcome && writable)) {
    return null;
  }
  return (
    <MeetingOutcome
      id={activity.id}
      version={activity.version}
      title={activity.subject ?? undefined}
      cancellable={activity.calendar_managed !== true}
      // The Worklist asks about the same meeting, so an answer here clears it there.
      invalidateKeys={[...invalidateKeys, worklistKey]}
    />
  );
}

/**
 * MeetingOutcome is the row's answer to How did it go? It happened (held), it
 * did not happen (canceled), or the reader says more about what came of it.
 *
 * "It happened" is the commonest answer and a fact with nothing to add, so it writes
 * at once, as Cancel does. "Add notes" opens the dialog, which also offers
 * No-show and moves the date.
 */
export function MeetingOutcome({
  id,
  version,
  title,
  invalidateKeys,
  cancellable,
}: Readonly<{
  id: string;
  version: number | undefined;
  // The reads that show this meeting, so each surface refreshes its own list.
  invalidateKeys: readonly QueryKey[];
  // The row's own heading for the meeting, so the dialog is titled before its read answers.
  title: string | undefined;
  // False where a live invitation holds the meeting: the server refuses the cancel.
  cancellable: boolean;
}>) {
  const t = useT();
  const toast = useToast();
  const [open, setOpen] = useState(false);
  // An invitation can arrive after the row was read, so its refusal hides Cancel too.
  const [refusedAsManaged, setRefusedAsManaged] = useState(false);
  // Marking it held asks for the next step at once, of a reader who may file
  // one. The lists showing this meeting refresh only once that is answered:
  // the refresh drops the answered meeting, and this control with it.
  const [askNext, setAskNext] = useState(false);
  const mayFile = useCanWrite("activity", "create");
  const queryClient = useQueryClient();
  const refresh = () => {
    for (const queryKey of [...invalidateKeys, ["activity", id]]) {
      queryClient.invalidateQueries({ queryKey });
    }
  };
  const asksNext = (status: string) => status === "held" && mayFile;
  const record = useMeetingOutcome(invalidateKeys, (input) =>
    asksNext(input.status),
  );
  const answer = (status: "held" | "canceled") =>
    record.mutate(
      { id, version, status },
      {
        onSuccess: () => {
          toast.show(t("worklist.verb.meetingOutcomeRecorded"));
          if (asksNext(status)) setAskNext(true);
        },
        // A refused write leaves the row as it was, which looks like a
        // click that did nothing.
        onError: (error) => {
          const managed =
            status === "canceled" &&
            problemFieldErrorsOf(error).some(
              (fault) => fault.code === "calendar_managed",
            );
          if (status === "canceled") setRefusedAsManaged(managed);
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
  return (
    <>
      <span className="t-caption">{t("worklist.verb.meetingHowDidItGo")}</span>
      <Button
        pending={record.isPending}
        aria-label={
          title
            ? t("worklist.verb.meetingHeldFor", { subject: title })
            : undefined
        }
        onClick={() => answer("held")}
      >
        {t("worklist.verb.meetingHeld")}
      </Button>
      <Button
        pending={record.isPending}
        aria-label={
          title
            ? t("worklist.verb.meetingAddNotesFor", { subject: title })
            : undefined
        }
        onClick={() => setOpen(true)}
      >
        {t("worklist.verb.meetingAddNotes")}
      </Button>
      {cancellable && !refusedAsManaged && (
        <Button
          pending={record.isPending}
          aria-label={
            title
              ? t("worklist.verb.meetingCanceledFor", { subject: title })
              : undefined
          }
          onClick={() => answer("canceled")}
        >
          {t("worklist.verb.meetingCanceled")}
        </Button>
      )}
      {open && (
        <MeetingOutcomeDialog
          id={id}
          title={title}
          onClose={() => setOpen(false)}
          onSaved={(status) =>
            asksNext(status) ? setAskNext(true) : refresh()
          }
        />
      )}
      {askNext && (
        <MeetingNextStep
          meetingId={id}
          invalidateKeys={[]}
          onClose={() => {
            setAskNext(false);
            refresh();
          }}
        />
      )}
    </>
  );
}
