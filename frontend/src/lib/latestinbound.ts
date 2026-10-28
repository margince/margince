// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";

type Activity = components["schemas"]["Activity"];

// How far back "just in" reaches. Older unanswered mail belongs to the
// worklist's waiting lane, which ranks it; a record page leads only with news.
export const JUST_IN_DAYS = 7;

export type LatestInboundKind = "email" | "message" | "web_form";

export type LatestInbound = Readonly<{
  activity: Activity;
  kind: LatestInboundKind;
}>;

// The `source` a website form stamps on the note it lands as.
const WEB_FORM_SOURCE = "web_form";

// Whether a row is us answering them: anything we sent, or a call or meeting
// held with them. A note is our own record-keeping and answers nobody.
function answers(row: Activity): boolean {
  switch (row.kind) {
    case "email":
    case "message":
      return row.direction === "outbound";
    case "call":
      return true;
    case "meeting":
      return (
        row.meeting_status !== "canceled" && row.meeting_status !== "no_show"
      );
    default:
      return false;
  }
}

/**
 * The newest thing they sent that nobody has answered yet, or undefined.
 *
 * `waiting` is the server's own answer for mail and chat (the activities
 * list's `waiting_reply`), so this never re-derives who is waiting on a
 * thread. A website form lands as a note the waiting read does not know.
 * So it is found in `recent`, and is answered once a row in `answers` comes
 * after it. Both reads are already cut to the last `JUST_IN_DAYS`.
 */
export function latestInbound(
  waiting: Activity | undefined,
  recent: readonly Activity[],
): LatestInbound | undefined {
  const form = recent.find(
    (row) => row.kind === "note" && row.source === WEB_FORM_SOURCE,
  );
  const formOpen =
    form &&
    !recent.some(
      (row) =>
        answers(row) &&
        Date.parse(row.occurred_at) > Date.parse(form.occurred_at),
    );
  const message =
    waiting && (waiting.kind === "email" || waiting.kind === "message")
      ? { activity: waiting, kind: waiting.kind }
      : undefined;
  if (!formOpen) {
    return message;
  }
  const formEntry = { activity: form, kind: "web_form" as const };
  if (!message) {
    return formEntry;
  }
  return Date.parse(message.activity.occurred_at) >=
    Date.parse(form.occurred_at)
    ? message
    : formEntry;
}
