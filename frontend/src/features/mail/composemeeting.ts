// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../../api/client";
import { useCan, useCanMutate } from "../../lib/capability";
import { contact360Query } from "../../lib/contact.queries";
import { unwrap } from "../../lib/problem";

/** The record a meeting face to face is noted on: the one the mail is about. */
export type MeetingSubject = Readonly<{
  kind: "contact" | "lead";
  id: string;
}>;

/**
 * "We met in person": the rep's word that they met, noted on the contact before
 * the send. The meeting is the evidence the engine reads for a personal
 * follow-up, so it is written first. It is written once per composition, so a
 * second press after a refused send does not note it twice.
 */
export function useMeeting(open: boolean, subject: MeetingSubject | null) {
  const queries = useQueryClient();
  const mayMutate = useCanMutate();
  const mayWriteContact = useCan("contact", "update");
  const mayWriteLead = useCan("lead", "update");
  const [on, setOn] = useState(false);
  const [failed, setFailed] = useState(false);
  const [recorded, setRecorded] = useState("");
  // A closed composer forgets the meeting, so the next message starts unpicked.
  const [wasOpen, setWasOpen] = useState(open);
  if (wasOpen !== open) {
    setWasOpen(open);
    if (!open) {
      setOn(false);
      setFailed(false);
      setRecorded("");
    }
  }
  const canRecord =
    subject !== null &&
    mayMutate &&
    (subject.kind === "contact" ? mayWriteContact : mayWriteLead);
  const record = async () => {
    if (!subject || !on) return;
    const key = `${subject.kind}:${subject.id}`;
    if (recorded === key) return;
    setFailed(false);
    const path = { params: { path: { id: subject.id } } };
    const body = {
      kind: "in_person" as const,
      // The sender's own statement is the whole evidence of a meeting.
      note: MEETING_NOTE,
      occurred_at: new Date().toISOString(),
      asks_for_quote: false,
    };
    try {
      unwrap(
        subject.kind === "lead"
          ? await api.POST("/leads/{id}/consent/qualifying-events", {
              ...path,
              body,
            })
          : await api.POST("/contacts/{id}/consent/qualifying-events", {
              ...path,
              body,
            }),
      );
    } catch (error) {
      setFailed(true);
      throw error;
    }
    setRecorded(key);
    if (subject.kind === "contact") {
      await queries.invalidateQueries({
        queryKey: contact360Query(subject.id).queryKey,
      });
    }
  };
  return {
    canRecord,
    on: on && canRecord,
    setOn,
    failed,
    record,
  };
}

/** The note a stated meeting carries. Stored data, so it is not translated. */
export const MEETING_NOTE = "Met in person, as stated by the sender.";

export type Meeting = ReturnType<typeof useMeeting>;
