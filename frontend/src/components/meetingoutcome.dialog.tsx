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
import { ActivityBodyField } from "./activitybodyfield";
import { FollowUpTaskFields } from "./followuptaskfields";
import { useMeetingCustomFields } from "./meetingoutcome.custom";
import { type OutcomeDraft, OutcomeEditFields } from "./meetingoutcome.fields";
import {
  writeMeetingNotes,
  writeMeetingOutcome,
} from "./taskactions.rowwrites";

/**
 * What the dialog was opened to do. `held` answers "It happened": notes and
 * the next step. `edit` is the whole outcome. `notes` writes only the notes
 * of a meeting whose outcome already stands.
 */
export type MeetingDialogMode = "held" | "edit" | "notes";

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
  body: string;
  version: number | undefined;
};

/** The notes an outcome carries: none when untouched, so a bare answer leaves them be. */
function changedNotes(
  draft: OutcomeDraft,
  seed: OutcomeSeed,
  transcript: boolean,
): { body?: string | null } {
  return draft.body === seed.body
    ? {}
    : { body: outgoingBody(draft.body, transcript) };
}

/** The meeting's subject, notes and start, which only the edit mode rewrites. */
function editedWords(
  draft: OutcomeDraft,
  transcript: boolean,
  moved: MovedStart | undefined,
) {
  return {
    subject: draft.subject.trim(),
    // An emptied box clears the note rather than storing "": null
    // is what the column holds for a meeting nobody wrote about.
    body: outgoingBody(draft.body, transcript),
    start: moved,
  };
}

function dialogHeading(
  mode: MeetingDialogMode,
  title: string | undefined,
  t: ReturnType<typeof useT>,
): string {
  if (mode === "held") return t("worklist.verb.meetingHowDidItGo");
  return title ?? t("worklist.verb.meetingUpdateTitle");
}

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

type SaveInput = {
  mode: MeetingDialogMode;
  draft: OutcomeDraft;
  seed: OutcomeSeed;
  moved: MovedStart | undefined;
  transcript: boolean;
  managed: boolean;
  zone: string;
  custom: Record<string, unknown>;
};

async function saveMeeting(
  id: string,
  input: SaveInput,
  t: ReturnType<typeof useT>,
): Promise<void> {
  const { mode, draft, seed, transcript } = input;
  if (mode === "notes") {
    await writeMeetingNotes(
      {
        id,
        version: seed.version,
        body: outgoingBody(draft.body, transcript),
      },
      t,
    );
    return;
  }
  // A calendar-managed meeting keeps the calendar's subject, notes and
  // start; only its outcome is written.
  let words = {};
  if (!input.managed) {
    words =
      mode === "edit"
        ? editedWords(draft, transcript, input.moved)
        : changedNotes(draft, seed, transcript);
  }
  await writeMeetingOutcome(
    {
      id,
      version: seed.version,
      status: draft.status,
      // The workspace's own fields, which no calendar holds.
      custom: input.custom,
      followUpTask: followUpFor(draft, input.zone),
      ...words,
    },
    t,
  );
}

/** The notes alone, as "It happened" and a held meeting's "Edit notes" ask for them. */
function MeetingNotesField({
  draft,
  transcript,
  saving,
  onChange,
}: Readonly<{
  draft: OutcomeDraft;
  transcript: boolean;
  // The save carries the notes as they were pressed, so typing waits for it.
  saving: boolean;
  onChange: (draft: OutcomeDraft) => void;
}>) {
  const t = useT();
  return (
    <ActivityBodyField
      label={t("worklist.verb.meetingNotes")}
      hint={t("worklist.verb.meetingBodyHint")}
      transcript={transcript}
      rows={4}
      disabled={saving}
      value={draft.body}
      onChange={(body) => onChange({ ...draft, body })}
    />
  );
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
  mode,
}: Readonly<{
  id: string;
  title: string | undefined;
  invalidateKeys: readonly QueryKey[];
  onClose: () => void;
  mode: MeetingDialogMode;
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
  const editsWords = !managed && mode === "edit";
  const custom = useMeetingCustomFields(meeting.data);
  // Seeded during render, not in an effect: an effect paints an empty form first
  // and replaces words typed into that frame.
  if (draft === null && meeting.data) {
    const day = activityDate(
      meeting.data.occurred_at,
      meeting.data.occurred_precision,
      readerZone,
    );
    const body = meeting.data.body ?? "";
    setDraft({
      status: "held",
      subject: meeting.data.subject ?? "",
      body,
      day,
    });
    setSeed({
      start: {
        at: meeting.data.occurred_at,
        precision: meeting.data.occurred_precision,
      },
      day,
      body,
      version: meeting.data.version,
    });
  }
  const days = heldDays(seed, readerZone);
  const save = useMutation({
    // Fields travel as variables: a closure would read the last committed render.
    mutationFn: (input: SaveInput) => saveMeeting(id, input, t),
    onSuccess: () => {
      for (const queryKey of invalidateKeys) {
        queryClient.invalidateQueries({ queryKey });
      }
      queryClient.invalidateQueries({ queryKey: ["activity", id] });
      queryClient.invalidateQueries({ queryKey: activityFollowUpsKey(id) });
      toast.show(
        t(
          mode === "notes"
            ? "worklist.verb.meetingNotesSaved"
            : "worklist.verb.meetingOutcomeRecorded",
        ),
      );
      onClose();
    },
  });
  return (
    <Modal open onClose={onClose} labelledBy={titleId} intent="form">
      <Heading size="large" id={titleId} className="t-h2 modal-title">
        {dialogHeading(mode, title, t)}
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
                moved = editsWords
                  ? movedStart(seed, draft.day, readerZone, new Date())
                  : undefined;
              } catch (error) {
                // A day the record zone skipped has no start to keep, so
                // nothing is sent and the reader picks another date.
                if (!(error instanceof RangeError)) throw error;
                setDayRefused(true);
                return;
              }
              setDayRefused(false);
              save.mutate({
                mode,
                draft,
                seed,
                moved,
                transcript,
                managed,
                zone: readerZone,
                custom: mode === "edit" ? custom.patch() : {},
              });
            }}
          >
            {mode === "edit" && (
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
            {mode !== "edit" &&
              (managed ? (
                <p className="t-caption">
                  {t("worklist.verb.meetingCalendarManaged")}
                </p>
              ) : (
                <MeetingNotesField
                  draft={draft}
                  transcript={transcript}
                  saving={save.isPending}
                  onChange={setDraft}
                />
              ))}
            {mode !== "notes" && draft.status === "held" && (
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
            {mode === "held" && (
              <Button
                pending={save.isPending}
                onClick={() =>
                  save.mutate({
                    mode,
                    draft: { ...draft, followUp: undefined },
                    seed,
                    moved: undefined,
                    transcript,
                    managed,
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
