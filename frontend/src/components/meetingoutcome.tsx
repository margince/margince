// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// "Cancelled" writes the status from the card; "Update" opens a dialog that PATCHES
// the existing meeting, because a second activity would leave the original unanswered.

import {
  type QueryKey,
  useMutation,
  useQueryClient,
} from "@tanstack/react-query";
import { useId, useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { ifMatch, requireVersion } from "../api/version";
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
  activityHasHappened,
  movedActivityStart,
  type OccurredPrecision,
} from "../format/activitywhen";
import { calendarDay } from "../format/calendarday";
import { useNow } from "../format/now";
import { useT } from "../i18n";
import { outgoingBody } from "../lib/activitybody";
import { useActivity } from "../lib/activityread";
import { useCanWriteRecord } from "../lib/capability";
import { unwrap } from "../lib/problem";
import { useRecordZone } from "../lib/recordzone";
import { ActivityBodyField } from "./activitybodyfield";
import { useMeetingOutcome } from "./taskactions";

type Activity = components["schemas"]["Activity"];

/** The outcome choice for a meeting that has begun, has no status and is writable. `asOf` defaults to mount time. */
export function RecordMeetingOutcome({
  activity,
  invalidateKeys,
  asOf,
}: Readonly<{
  activity: Activity;
  invalidateKeys: readonly QueryKey[];
  asOf?: number;
}>) {
  const writable = useCanWriteRecord("activity", activity);
  const zone = useRecordZone();
  const mountedAt = useNow(0);
  const open =
    activity.kind === "meeting" &&
    !activity.meeting_status &&
    activity.content_state !== "withheld" &&
    writable &&
    activityHasHappened(activity, asOf ?? mountedAt, zone);
  if (!open) {
    return null;
  }
  return (
    <MeetingOutcome
      id={activity.id}
      version={activity.version}
      title={activity.subject ?? undefined}
      invalidateKeys={invalidateKeys}
    />
  );
}

// The two statuses the dialog can record. `canceled` is deliberately absent:
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
 * capped at now, because a meeting cannot be filed in the future. A date-only
 * meeting names its new date alone and stays date-only, with no clock to cap.
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
 * MeetingOutcome is the row's answer: cancel it, or say what came of it.
 *
 * Two verbs rather than three, and they are not the same KIND of answer, which
 * is why one is a write and the other is a door. A cancelled meeting is a fact
 * with nothing to add; a meeting that happened has an outcome, and that outcome
 * is the thing a rep came to the queue to record.
 */
export function MeetingOutcome({
  id,
  version,
  title,
  invalidateKeys,
}: Readonly<{
  id: string;
  version: number | undefined;
  // The reads that show this meeting, so each surface refreshes its own list.
  invalidateKeys: readonly QueryKey[];
  // The row's own heading for the meeting, so the dialog is titled before its read answers.
  title: string | undefined;
}>) {
  const t = useT();
  const toast = useToast();
  const [open, setOpen] = useState(false);
  const record = useMeetingOutcome(invalidateKeys);
  return (
    <>
      <Button pending={record.isPending} onClick={() => setOpen(true)}>
        {t("worklist.verb.meetingUpdate")}
      </Button>
      <Button
        pending={record.isPending}
        onClick={() =>
          record.mutate(
            { id, version, status: "canceled" },
            {
              onSuccess: () =>
                toast.show(t("worklist.verb.meetingOutcomeRecorded")),
              // A refused write leaves the row exactly as it was, which renders
              // identically to a click that did nothing.
              onError: () =>
                toast.show(t("worklist.verb.meetingOutcomeFailed"), {
                  tone: "danger",
                }),
            },
          )
        }
      >
        {t("worklist.verb.meetingCanceled")}
      </Button>
      {open && (
        <MeetingOutcomeDialog
          id={id}
          title={title}
          invalidateKeys={invalidateKeys}
          onClose={() => setOpen(false)}
        />
      )}
    </>
  );
}

/**
 * The composer, bound to one meeting.
 *
 * It reads the meeting first rather than drawing an empty form: the subject and
 * the captured body are what the reader is amending, and a form that started
 * blank would invite them to retype what the calendar already knew — or, worse,
 * to save an empty body over it.
 */
function MeetingOutcomeDialog({
  id,
  title,
  invalidateKeys,
  onClose,
}: Readonly<{
  id: string;
  title: string | undefined;
  invalidateKeys: readonly QueryKey[];
  onClose: () => void;
}>) {
  const t = useT();
  const toast = useToast();
  const titleId = useId();
  const formId = useId();
  const recordZone = useRecordZone();
  const queryClient = useQueryClient();
  const [draft, setDraft] = useState<OutcomeDraft | null>(null);
  const [seed, setSeed] = useState<OutcomeSeed | null>(null);
  const [dayRefused, setDayRefused] = useState(false);
  const latestDay =
    isoOrBlank(calendarDay(new Date(), recordZone)) || undefined;
  // The SAME read the task detail makes, and keyed the same way, so a write
  // that invalidates ["activity", id] refreshes whichever of the two is open.
  const meeting = useActivity(id);
  const transcript = meeting.data ? isTranscriptActivity(meeting.data) : false;
  // Seeded during render, not in an effect: an effect paints an empty form first
  // and replaces words typed into that frame.
  if (draft === null && meeting.data) {
    const day = activityDate(
      meeting.data.occurred_at,
      meeting.data.occurred_precision,
      recordZone,
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
  const save = useMutation({
    // Fields travel as variables: a closure would read the last committed render.
    mutationFn: async (input: {
      draft: OutcomeDraft;
      seed: OutcomeSeed;
      moved: MovedStart | undefined;
      transcript: boolean;
    }) => {
      unwrap(
        await api.PATCH("/activities/{id}", {
          params: {
            path: { id },
            ...ifMatch(requireVersion(input.seed.version)),
          },
          body: {
            meeting_status: input.draft.status,
            subject: input.draft.subject.trim(),
            // An emptied box clears the note rather than storing "": null is what
            // the column holds for a meeting nobody has written about.
            body: outgoingBody(input.draft.body, input.transcript),
            ...input.moved,
          },
        }),
        t,
      );
    },
    onSuccess: () => {
      for (const queryKey of invalidateKeys) {
        queryClient.invalidateQueries({ queryKey });
      }
      queryClient.invalidateQueries({ queryKey: ["activity", id] });
      toast.show(t("worklist.verb.meetingOutcomeRecorded"));
      onClose();
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
                moved = movedStart(seed, draft.day, recordZone, new Date());
              } catch (error) {
                // A day the record zone skipped has no start to keep, so
                // nothing is sent and the reader picks another date.
                if (!(error instanceof RangeError)) throw error;
                setDayRefused(true);
                return;
              }
              setDayRefused(false);
              save.mutate({ draft, seed, moved, transcript });
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
              <Field label={t("log.date")}>
                {(control) => (
                  <DateInput
                    {...control}
                    value={isoOrBlank(draft.day)}
                    // A meeting cannot have happened in the future, and the cap
                    // makes the box say so rather than leaving the server to.
                    max={latestDay}
                    today={latestDay}
                    onChange={(event) => {
                      setDayRefused(false);
                      setDraft({ ...draft, day: event.target.value });
                    }}
                  />
                )}
              </Field>
            </div>
            <Field label={t("log.subject")} required>
              {(control) => (
                <TextInput
                  {...control}
                  value={draft.subject}
                  onChange={(event) =>
                    setDraft({ ...draft, subject: event.target.value })
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
              onChange={(body) => setDraft({ ...draft, body })}
            />
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
              disabled={!save.isPending && !draft.subject.trim()}
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
