// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQueryClient } from "@tanstack/react-query";
import { useRef, useState } from "react";
import { api } from "../../api/client";
import { calendarDay, middayInstant } from "../../format/calendarday";
import type { ISODate } from "../../format/datepattern";
import { useCan, useCanMutate } from "../../lib/capability";
import { contact360Query } from "../../lib/contact.queries";
import { unwrap } from "../../lib/problem";
import { useViewerZone } from "../../lib/viewerzone";

/** The record a meeting face to face is noted on: the one the mail is about. */
export type MeetingSubject = Readonly<{
  kind: "contact" | "lead";
  id: string;
}>;

/**
 * "We met in person": where and when, noted on the contact before the send.
 *
 * The meeting is the evidence the engine reads for a personal follow-up, so it
 * is written first and the send follows. It is written once per where and day:
 * a second press after a refused send does not note the same meeting twice.
 */
export function useMeeting(open: boolean, subject: MeetingSubject | null) {
  const zone = useViewerZone();
  const queries = useQueryClient();
  const mayMutate = useCanMutate();
  const mayWriteContact = useCan("contact", "update");
  const mayWriteLead = useCan("lead", "update");
  const today = calendarDay(new Date(), zone);
  const [on, setOn] = useState(false);
  const [where, setWhere] = useState("");
  const [day, setDay] = useState<ISODate | "">(today);
  const [failed, setFailed] = useState(false);
  const recorded = useRef("");
  // A closed composer forgets the meeting, so the next message starts unpicked.
  const [wasOpen, setWasOpen] = useState(open);
  if (wasOpen !== open) {
    setWasOpen(open);
    if (!open) {
      setOn(false);
      setWhere("");
      setDay(today);
      setFailed(false);
    }
  }
  const canRecord =
    subject !== null &&
    mayMutate &&
    (subject.kind === "contact" ? mayWriteContact : mayWriteLead);
  const record = async () => {
    if (!subject || !on) return;
    const met = day === "" ? today : day;
    const note = where.trim();
    const key = `${subject.kind}:${subject.id}:${met}:${note}`;
    if (recorded.current === key) return;
    setFailed(false);
    const path = { params: { path: { id: subject.id } } };
    const body = {
      kind: "in_person" as const,
      note,
      asks_for_quote: false,
      // Today's meeting happened by now; noon would lie in the future.
      occurred_at:
        met === today ? new Date().toISOString() : middayInstant(met, zone),
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
    recorded.current = key;
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
    where,
    setWhere,
    day,
    setDay,
    today,
    failed,
    /** Where is the one fact a hand-noted meeting must carry. */
    missing: on && canRecord && where.trim() === "",
    record,
  };
}

export type Meeting = ReturnType<typeof useMeeting>;
