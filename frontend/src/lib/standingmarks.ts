// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import type { TimelineMark } from "../design-system/standingtimeline";
import { activityDay, activityHasHappened } from "../format/activitywhen";
import { formatDayMonth } from "../format/format.dates";
import type { Locale, Translator } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { authorName } from "./messagevoice";

type Activity = components["schemas"]["Activity"];

const KIND_WORD: Readonly<Record<Activity["kind"], MessageKey>> = {
  email: "timeline.kind.email",
  call: "timeline.kind.call",
  meeting: "timeline.kind.meeting",
  note: "timeline.kind.note",
  task: "timeline.kind.task",
  message: "timeline.kind.message",
};

// A meeting that never took place, or has not yet, is no logged item.
const NOT_HELD = new Set(["booked", "canceled", "no_show"]);

/**
 * The logged items and messages a record's page already loaded, as the marks
 * the How it stands line draws. They cover mail sent and received, calls,
 * meetings held and notes. A task is a promise rather than a thing that happened, so it
 * stays a dot with a label. Only what the rows say is shown. A row whose text
 * is withheld names its kind and nothing else, and a name is shown only
 * where the row names one.
 */
export function activityMarks(
  rows: readonly Activity[],
  words: Readonly<{
    t: Translator;
    locale: Locale;
    zone: string;
    // The record zone, which a date-only activity is read in.
    record: string;
    // The instant the record was read at; nothing after it has happened.
    asOf: string;
    // Where pressing a card row goes, when the page has a History tab to open.
    onOpen?: () => void;
  }>,
): TimelineMark[] {
  const { t, locale, zone, record, onOpen } = words;
  const asOf = Date.parse(words.asOf);
  return rows
    .filter(
      (row) =>
        row.kind !== "task" &&
        !row.archived_at &&
        !NOT_HELD.has(row.meeting_status ?? "") &&
        activityHasHappened(row, asOf, record),
    )
    .map((row): TimelineMark => {
      const what = t(KIND_WORD[row.kind]);
      const direction = row.email_summary?.direction ?? row.direction;
      return {
        key: row.id,
        at: row.occurred_at,
        precision: row.occurred_precision,
        kind: row.kind,
        direction: direction ?? undefined,
        what,
        title: (row.subject ?? row.email_summary?.subject)?.trim() || what,
        detail: messageDetail(row, t),
        when: activityDay(
          row.occurred_at,
          row.occurred_precision,
          locale,
          zone,
          formatDayMonth,
        ),
        onOpen,
      };
    });
}

/**
 * Which way a message went, and with whom when the row says ("From Dana
 * Reyes", "Sent"). Calls and meetings carry no named far side on a list row,
 * so they carry no detail.
 */
export function messageDetail(
  row: Activity,
  t: Translator,
): string | undefined {
  if (row.kind !== "email" && row.kind !== "message") {
    return undefined;
  }
  const direction = row.email_summary?.direction ?? row.direction;
  const who = row.email_summary?.counterparty;
  if (direction === "outbound") {
    return who ? t("timeline.sentTo", { who }) : t("timeline.sent");
  }
  if (direction === "inbound") {
    // The sender is the recorded author when the row names no counterparty.
    const from = who ?? authorName(row);
    return from
      ? t("timeline.receivedFrom", { who: from })
      : t("timeline.received");
  }
  return who ?? undefined;
}
