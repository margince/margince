// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Recording what came of a meeting, from the row that asked.
//
// The row used to offer three verbs — held, no-show, cancelled — and nothing
// else. Each wrote one word and closed the card, so the one moment a human
// knows what actually happened was the moment the product could be told the
// least: a status, and not a sentence of what was said or what to do next.
//
// So the row offers TWO verbs now. "Cancelled" is the one answer that needs no
// prose — the meeting did not happen, and there is nothing to write down — and
// it writes the status straight from the card. "Update" opens the composer the
// record pages already use, bound to THIS meeting, so the outcome is typed
// where the reader is standing.
//
// WHY THE DIALOG PATCHES RATHER THAN POSTING, which is the whole difference
// from LogActivityForm beside it: the meeting already exists. It arrived from a
// calendar with its subject, its start and its attendee list, and logging a
// second activity to say how it went would leave the original sitting in the
// queue unanswered, with the account's timeline carrying two rows for one
// meeting. `activityRequestBody` cannot serve this: it always sends `kind`,
// `links` and `source`, none of which a patch may carry.
//
// EDITING THE BODY IS SAFE, and it is worth saying why rather than leaving the
// next reader to work it out. A captured meeting's body is an excerpt the
// calendar mapper composed — `Organizer: …`, `Attendees: …`, then the event
// description (capture/meetingmap.buildBody). It is not the record of who
// attended: those are `activity_participant` rows, written beside it and read
// by every query that asks who was there. And a re-pull cannot undo an edit —
// the capture upsert is `ON CONFLICT (source_system, source_id) DO NOTHING`, so
// a second sync of the same event returns the incumbent row untouched.

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useId, useState } from "react";
import { api } from "../api/client";
import { ifMatch, requireVersion } from "../api/version";
import { useRecordZone } from "../app/recordzone";
import {
  Button,
  Field,
  Modal,
  Textarea,
  TextInput,
} from "../design-system/atoms";
import { Heading } from "../design-system/heading";
import { Select } from "../design-system/select";
import { useToast } from "../design-system/toast";
import { calendarDay, middayInstant } from "../format/calendarday";
import { useT } from "../i18n";
import { problemMessageOf, throwProblem } from "./common";
import { useMeetingOutcome } from "./taskactions";
import { worklistKey } from "./worklist.queries";

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
}: Readonly<{
  id: string;
  version: number | undefined;
  // What the row already calls this meeting, so the dialog can name it before
  // its own read answers. The row has it; asking the server for a heading the
  // caller is already displaying would leave the dialog briefly untitled.
  title: string | undefined;
}>) {
  const t = useT();
  const toast = useToast();
  const [open, setOpen] = useState(false);
  const record = useMeetingOutcome([worklistKey]);
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
                  mark: false,
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
  onClose,
}: Readonly<{ id: string; title: string | undefined; onClose: () => void }>) {
  const t = useT();
  const toast = useToast();
  const titleId = useId();
  const recordZone = useRecordZone();
  const queryClient = useQueryClient();
  const [draft, setDraft] = useState<OutcomeDraft | null>(null);
  // The SAME read the task detail makes, and keyed the same way, so a write
  // that invalidates ["activity", id] refreshes whichever of the two is open.
  const meeting = useQuery({
    queryKey: ["activity", id],
    staleTime: 0,
    gcTime: 0,
    queryFn: async () => {
      const { data, error } = await api.GET("/activities/{id}", {
        params: { path: { id } },
      });
      if (error) {
        throwProblem(error, t);
      }
      return data;
    },
  });
  // Seeded from the read, once, during render rather than in an effect — an
  // effect paints the empty form first, and a reader who types into that frame
  // has their words replaced when the seed lands.
  if (draft === null && meeting.data) {
    setDraft({
      status: "held",
      subject: meeting.data.subject ?? "",
      body: meeting.data.body ?? "",
      day: calendarDay(new Date(meeting.data.occurred_at), recordZone),
    });
  }
  const save = useMutation({
    // Every field travels as a VARIABLE. A mutationFn closing over render state
    // runs against the last committed render, and what this one decides is what
    // gets stored on a customer's meeting.
    mutationFn: async (input: {
      draft: OutcomeDraft;
      version: number | undefined;
      zone: string;
    }) => {
      const { error } = await api.PATCH("/activities/{id}", {
        params: {
          path: { id },
          ...ifMatch(requireVersion(input.version)),
        },
        body: {
          meeting_status: input.draft.status,
          subject: input.draft.subject.trim(),
          // An emptied box clears the note rather than storing "": null is what
          // the column holds for a meeting nobody has written about.
          body: input.draft.body.trim() || null,
          // Noon on the picked day, in the RECORD's zone — the same instant
          // activitybody mints for a backdated entry, so a meeting moved by a
          // day here and one logged by hand land on the same timeline heading.
          occurred_at: middayInstant(input.draft.day, input.zone),
        },
      });
      if (error) {
        throwProblem(error, t);
      }
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: worklistKey });
      queryClient.invalidateQueries({ queryKey: ["activity", id] });
      toast.show(t("worklist.verb.meetingOutcomeRecorded"));
      onClose();
    },
  });
  return (
    <Modal open onClose={onClose} labelledBy={titleId}>
      <Heading size="large" id={titleId} className="t-h2 modal-title">
        {title ?? t("worklist.verb.meetingUpdateTitle")}
      </Heading>
      {meeting.isPending && (
        <p className="t-caption">{t("worklist.verb.meetingReading")}</p>
      )}
      {meeting.isError && (
        <p className="t-caption form-error">
          {problemMessageOf(meeting.error, t)}
        </p>
      )}
      {draft && (
        <form
          className="form-stack"
          onSubmit={(event) => {
            event.preventDefault();
            save.mutate({
              draft,
              version: meeting.data?.version,
              zone: recordZone,
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
            <Field label={t("log.date")}>
              {(control) => (
                <TextInput
                  {...control}
                  type="date"
                  value={draft.day}
                  // A meeting cannot have happened in the future, and the cap
                  // makes the box say so rather than leaving the server to.
                  max={calendarDay(new Date(), recordZone)}
                  onChange={(event) =>
                    setDraft({ ...draft, day: event.target.value })
                  }
                  onClick={(event) => event.currentTarget.showPicker?.()}
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
          <Field
            label={t("log.body")}
            hint={t("worklist.verb.meetingBodyHint")}
          >
            {(control) => (
              <Textarea
                {...control}
                rows={6}
                value={draft.body}
                onChange={(event) =>
                  setDraft({ ...draft, body: event.target.value })
                }
              />
            )}
          </Field>
          {save.isError && (
            <p className="t-caption form-error">
              {problemMessageOf(save.error, t)}
            </p>
          )}
          <div className="form-actions">
            <Button variant="ghost" type="button" onClick={onClose}>
              {t("common.close")}
            </Button>
            <Button
              variant="primary"
              type="submit"
              disabled={!save.isPending && !draft.subject.trim()}
              pending={save.isPending}
              busyLabel={t("log.saving")}
            >
              {t("log.save")}
            </Button>
          </div>
        </form>
      )}
    </Modal>
  );
}
