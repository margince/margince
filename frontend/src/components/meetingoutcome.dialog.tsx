// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The notes dialog bound to one meeting: its subject, notes, start and outcome.

import { useMutation } from "@tanstack/react-query";
import { useId, useState } from "react";
import { Button, Field, Modal, TextInput } from "../design-system/atoms";
import { DateInput, isoOrBlank } from "../design-system/dateinput";
import { ErrorLine } from "../design-system/errorline";
import { refuseUnreadableField } from "../design-system/formvalidity";
import { Heading } from "../design-system/heading";
import { Select } from "../design-system/select";
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
import { outgoingBody } from "../lib/activitybody";
import { useActivity } from "../lib/activityread";
import { useViewerZone } from "../lib/viewerzone";
import { ActivityBodyField } from "./activitybodyfield";
import { useMeetingCustomFields } from "./meetingoutcome.custom";
import { writeMeetingOutcome } from "./taskactions.rowwrites";

// The two statuses the dialog can record. `canceled` is absent:
// the card's own verb writes it, and offering it here as well would give one
// answer two doors that could drift.
type HappenedStatus = "held" | "no_show";

/** What the dialog holds while it is open. */
type OutcomeDraft = {
  status: HappenedStatus;
  subject: string;
  body: string;
  day: string;
};

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
  onClose,
  onSaved,
}: Readonly<{
  id: string;
  title: string | undefined;
  onClose: () => void;
  // Refreshes the lists, or asks for the next step first; the caller decides.
  onSaved: (status: string) => void;
}>) {
  const t = useT();
  const toast = useToast();
  const titleId = useId();
  const formId = useId();
  const readerZone = useViewerZone();
  const [draft, setDraft] = useState<OutcomeDraft | null>(null);
  const [seed, setSeed] = useState<OutcomeSeed | null>(null);
  const [dayRefused, setDayRefused] = useState(false);
  // The read the task detail makes, keyed the same way. A write that
  // invalidates ["activity", id] refreshes whichever of the two is open.
  const meeting = useActivity(id);
  const transcript = meeting.data ? isTranscriptActivity(meeting.data) : false;
  const managed = meeting.data?.calendar_managed === true;
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
      managed: boolean;
      custom: Record<string, unknown>;
    }) => {
      await writeMeetingOutcome(
        {
          id,
          version: input.seed.version,
          status: input.draft.status,
          // The workspace's own fields, which no calendar holds.
          custom: input.custom,
          // A calendar-managed meeting keeps the calendar's subject, notes
          // and start; only its outcome is written.
          ...(input.managed
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
    onSuccess: (_written, input) => {
      toast.show(t("worklist.verb.meetingOutcomeRecorded"));
      onClose();
      onSaved(input.draft.status);
    },
  });
  return (
    <Modal open onClose={onClose} labelledBy={titleId} intent="form">
      <Heading size="large" id={titleId} className="t-h2 modal-title">
        {title ?? t("worklist.verb.meetingUpdateTitle")}
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
                moved = managed
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
                managed,
                custom: custom.patch(),
              });
            }}
          >
            <div className="form-row">
              <Field label={t("worklist.verb.meetingWhatHappened")}>
                {(control) => (
                  <Select
                    {...control}
                    options={[
                      { value: "held", label: t("worklist.verb.meetingHeld") },
                      {
                        value: "no_show",
                        label: t("worklist.verb.meetingNoShow"),
                      },
                    ]}
                    value={draft.status}
                    // The Select can only hand back one of the two above; anything
                    // else is the ordinary answer rather than a refusal.
                    onChange={(value) =>
                      setDraft({
                        ...draft,
                        status: value === "no_show" ? "no_show" : "held",
                      })
                    }
                  />
                )}
              </Field>
              {!managed && (
                <Field label={t("log.date")}>
                  {(control) => (
                    <DateInput
                      {...control}
                      value={isoOrBlank(draft.day)}
                      max={days.latest}
                      today={days.today}
                      onChange={(event) => {
                        setDayRefused(false);
                        setDraft({ ...draft, day: event.target.value });
                      }}
                    />
                  )}
                </Field>
              )}
            </div>
            {managed ? (
              <p className="t-caption">
                {t("worklist.verb.meetingCalendarManaged")}
              </p>
            ) : (
              <OutcomeWords
                draft={draft}
                transcript={transcript}
                onChange={setDraft}
              />
            )}
            {custom.controls}
            {dayRefused && (
              <ErrorLine>{t("worklist.verb.meetingDayMissing")}</ErrorLine>
            )}
            <ErrorLine error={save.error} />
          </form>
          <div className="actions">
            <Button variant="ghost" type="button" onClick={onClose}>
              {t("common.close")}
            </Button>
            <Button
              variant="primary"
              type="submit"
              form={formId}
              disabled={!save.isPending && !managed && !draft.subject.trim()}
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

/** The subject and notes a meeting without a live invitation takes with its outcome. */
function OutcomeWords({
  draft,
  transcript,
  onChange,
}: Readonly<{
  draft: OutcomeDraft;
  transcript: boolean;
  onChange: (draft: OutcomeDraft) => void;
}>) {
  const t = useT();
  return (
    <>
      <Field label={t("log.subject")} required>
        {(control) => (
          <TextInput
            {...control}
            value={draft.subject}
            onChange={(event) =>
              onChange({ ...draft, subject: event.target.value })
            }
          />
        )}
      </Field>
      {/* The editor Log activity writes with; a transcript keeps its
          lines, which a citation points at. */}
      <ActivityBodyField
        label={t("log.body")}
        hint={t("worklist.verb.meetingBodyHint")}
        transcript={transcript}
        rows={6}
        value={draft.body}
        onChange={(body) => onChange({ ...draft, body })}
      />
    </>
  );
}
