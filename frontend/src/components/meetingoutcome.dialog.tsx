// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Outcome dialogs patch the existing meeting so its next step cites the meeting itself.

import {
  type QueryKey,
  useMutation,
  useQueryClient,
} from "@tanstack/react-query";
import { useId, useState } from "react";
import { Button, Modal } from "../design-system/atoms";
import { isoOrBlank } from "../design-system/dateinput";
import { ErrorLine } from "../design-system/errorline";
import { refuseUnreadableField } from "../design-system/formvalidity";
import { Heading } from "../design-system/heading";
import { useToast } from "../design-system/toast";
import { isTranscriptActivity } from "../design-system/transcriptread";
import {
  type ActivityMoment,
  activityDate,
  latestHeldDay,
  movedActivityStart,
  type OccurredPrecision,
} from "../format/activitywhen";
import { calendarDay } from "../format/calendarday";
import type { ISODate } from "../format/datepattern";
import { useT } from "../i18n";
import { outgoingBody, taskRequestFields } from "../lib/activitybody";
import { activityFollowUpsKey } from "../lib/activitykeys";
import { useActivity } from "../lib/activityread";
import { useViewerZone } from "../lib/viewerzone";
import { FollowUpTaskFields } from "./followuptaskfields";
import { useMeetingCustomFields } from "./meetingoutcome.custom";
import { type OutcomeDraft, OutcomeEditFields } from "./meetingoutcome.fields";
import { writeMeetingOutcome } from "./taskactions.rowwrites";

function followUpFor(draft: OutcomeDraft, zone: string) {
  return draft.status === "held" && draft.followUp
    ? taskRequestFields(draft.followUp, zone)
    : undefined;
}

function outcomeSaveDisabled(
  draft: OutcomeDraft,
  editsWords: boolean,
): boolean {
  if (editsWords && !draft.subject.trim()) return true;
  return (
    draft.status === "held" &&
    draft.followUp !== undefined &&
    !draft.followUp.subject.trim()
  );
}

/**
 * The one read the draft was seeded from. The save is checked against its
 * version. A meeting that changed while the dialog was open is refused, so
 * words written against the older copy are not stored on the newer one.
 */
type OutcomeSeed = {
  start: ActivityMoment;
  day: string;
  version: number | undefined;
};

type MovedStart = {
  occurred_at: string;
  occurred_precision: OccurredPrecision;
};

/**
 * The start the save sends: none when the day stayed, so the meeting keeps
 * its real start. A moved meeting keeps its time of day in the record zone,
 * capped just before now, because a held meeting cannot start later. A
 * date-only meeting names its new date alone, which the date box caps.
 */
function movedStart(
  seed: OutcomeSeed,
  day: string,
  zone: string,
  now: Date,
): MovedStart | undefined {
  if (day === seed.day) {
    return undefined;
  }
  const { at, precision } = movedActivityStart(seed.start, day, zone, now);
  return { occurred_at: at, occurred_precision: precision };
}

/**
 * The date box's ceiling, and the day it marks as today. Held means taken
 * place: a date-only meeting once its date is over, a timed one once it began.
 */
function heldDays(
  seed: OutcomeSeed | null,
  zone: string,
): { latest: ISODate | undefined; today: ISODate | undefined } {
  const now = new Date();
  const precision = seed?.start.precision ?? "instant";
  return {
    latest: isoOrBlank(latestHeldDay(precision, now, zone)) || undefined,
    today: isoOrBlank(calendarDay(now, zone)) || undefined,
  };
}

/**
 * The composer, bound to one meeting.
 *
 * It reads the meeting first rather than drawing an empty form. The subject and
 * the captured body are what the reader is amending. A blank form would invite
 * them to retype what the calendar knew, or to save an empty body over it.
 */
export function MeetingOutcomeDialog({
  id,
  title,
  invalidateKeys,
  onClose,
  outcomeOnly,
}: Readonly<{
  id: string;
  title: string | undefined;
  invalidateKeys: readonly QueryKey[];
  onClose: () => void;
  outcomeOnly: boolean;
}>) {
  const t = useT();
  const toast = useToast();
  const titleId = useId();
  const formId = useId();
  const readerZone = useViewerZone();
  const queryClient = useQueryClient();
  const [draft, setDraft] = useState<OutcomeDraft | null>(null);
  const [seed, setSeed] = useState<OutcomeSeed | null>(null);
  const [dayRefused, setDayRefused] = useState(false);
  // The read the task detail makes, keyed the same way. A write that
  // invalidates ["activity", id] refreshes whichever of the two is open.
  const meeting = useActivity(id);
  const transcript = meeting.data ? isTranscriptActivity(meeting.data) : false;
  const managed = meeting.data?.calendar_managed === true;
  const editsWords = !managed && !outcomeOnly;
  const custom = useMeetingCustomFields(meeting.data);
  // Seeded during render, not in an effect: an effect paints an empty form first
  // and replaces words typed into that frame.
  if (draft === null && meeting.data) {
    const day = activityDate(
      meeting.data.occurred_at,
      meeting.data.occurred_precision,
      readerZone,
    );
    setDraft({
      status: "held",
      subject: meeting.data.subject ?? "",
      body: meeting.data.body ?? "",
      day,
    });
    setSeed({
      start: {
        at: meeting.data.occurred_at,
        precision: meeting.data.occurred_precision,
      },
      day,
      version: meeting.data.version,
    });
  }
  const days = heldDays(seed, readerZone);
  const save = useMutation({
    // Fields travel as variables: a closure would read the last committed render.
    mutationFn: async (input: {
      draft: OutcomeDraft;
      seed: OutcomeSeed;
      moved: MovedStart | undefined;
      transcript: boolean;
      editsWords: boolean;
      zone: string;
      custom: Record<string, unknown>;
    }) => {
      await writeMeetingOutcome(
        {
          id,
          version: input.seed.version,
          status: input.draft.status,
          // The workspace's own fields, which no calendar holds.
          custom: input.custom,
          followUpTask: followUpFor(input.draft, input.zone),
          // A calendar-managed meeting keeps the calendar's subject, notes
          // and start; only its outcome is written.
          ...(!input.editsWords
            ? {}
            : {
                subject: input.draft.subject.trim(),
                // An emptied box clears the note rather than storing "": null
                // is what the column holds for a meeting nobody wrote about.
                body: outgoingBody(input.draft.body, input.transcript),
                start: input.moved,
              }),
        },
        t,
      );
    },
    onSuccess: () => {
      for (const queryKey of invalidateKeys) {
        queryClient.invalidateQueries({ queryKey });
      }
      queryClient.invalidateQueries({ queryKey: ["activity", id] });
      queryClient.invalidateQueries({ queryKey: activityFollowUpsKey(id) });
      toast.show(t("worklist.verb.meetingOutcomeRecorded"));
      onClose();
    },
  });
  return (
    <Modal open onClose={onClose} labelledBy={titleId} intent="form">
      <Heading size="large" id={titleId} className="t-h2 modal-title">
        {outcomeOnly
          ? t("worklist.verb.meetingNextStep")
          : (title ?? t("worklist.verb.meetingUpdateTitle"))}
      </Heading>
      {meeting.isPending && (
        <p className="t-caption">{t("worklist.verb.meetingReading")}</p>
      )}
      <ErrorLine error={meeting.error} />
      {/* Only while the read holds the meeting: a refused re-read withdraws
          what the draft was seeded from, and the version a save needs. */}
      {draft && seed && meeting.data && (
        <>
          <form
            id={formId}
            className="form-stack"
            onSubmit={(event) => {
              event.preventDefault();
              if (
                save.isPending ||
                refuseUnreadableField(event.currentTarget)
              ) {
                return;
              }
              let moved: MovedStart | undefined;
              try {
                moved =
                  managed || outcomeOnly
                    ? undefined
                    : movedStart(seed, draft.day, readerZone, new Date());
              } catch (error) {
                // A day the record zone skipped has no start to keep, so
                // nothing is sent and the reader picks another date.
                if (!(error instanceof RangeError)) throw error;
                setDayRefused(true);
                return;
              }
              setDayRefused(false);
              save.mutate({
                draft,
                seed,
                moved,
                transcript,
                editsWords,
                zone: readerZone,
                custom: outcomeOnly ? {} : custom.patch(),
              });
            }}
          >
            {!outcomeOnly && (
              <OutcomeEditFields
                draft={draft}
                managed={managed}
                transcript={transcript}
                days={days}
                onChange={setDraft}
                onDateChange={() => setDayRefused(false)}
              >
                {custom.controls}
              </OutcomeEditFields>
            )}
            {draft.status === "held" && (
              <FollowUpTaskFields
                value={draft.followUp}
                onChange={(followUp) => setDraft({ ...draft, followUp })}
              />
            )}
            {dayRefused && (
              <ErrorLine>{t("worklist.verb.meetingDayMissing")}</ErrorLine>
            )}
            <ErrorLine error={save.error} />
          </form>
          <div className="actions">
            <Button variant="ghost" type="button" onClick={onClose}>
              {t("common.close")}
            </Button>
            {outcomeOnly && (
              <Button
                pending={save.isPending}
                onClick={() =>
                  save.mutate({
                    draft: { ...draft, followUp: undefined },
                    seed,
                    moved: undefined,
                    transcript,
                    editsWords: false,
                    zone: readerZone,
                    custom: {},
                  })
                }
              >
                {t("worklist.verb.meetingSkipNextStep")}
              </Button>
            )}
            <Button
              variant="primary"
              type="submit"
              form={formId}
              disabled={
                !save.isPending && outcomeSaveDisabled(draft, editsWords)
              }
              pending={save.isPending}
              busyLabel={t("log.saving")}
            >
              {t("log.save")}
            </Button>
          </div>
        </>
      )}
    </Modal>
  );
}
