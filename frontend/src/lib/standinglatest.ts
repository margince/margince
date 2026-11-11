// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import type { StandingLatest } from "../design-system/howitstands";
import { activityDay, activityHasHappened } from "../format/activitywhen";
import { formatDayMonth } from "../format/format.dates";
import type { Locale, Translator } from "../i18n";
import { labelFor } from "./labelfor";
import { KIND_WORD, messageDetail, NOT_HELD } from "./standingmarks";
import { displayedBody, displayedSubject } from "./systemdisplay";

type Activity = components["schemas"]["Activity"];

// Mail a machine sent on its own says nothing about where the record stands.
// Margince's own notes do: they carry the reason for a change it made.
const SAYS_NOTHING = new Set(["calendar", "sending_system", "bulk"]);

/**
 * The newest thing said on the record, from the timeline its page already
 * read: a note, the notes of a call or meeting held, or a message's own text.
 * A task is a promise, not a state, and withheld or noise rows say nothing a
 * reader may see. Undefined when nothing on the page says anything.
 */
export function latestWord(
  rows: readonly Activity[] | undefined,
  words: Readonly<{
    t: Translator;
    locale: Locale;
    zone: string;
    // The zone a date-only row is read in.
    record: string;
    // The instant the page was read at; nothing after it has been said.
    asOf: string;
    onOpen?: () => void;
  }>,
): StandingLatest | undefined {
  const asOf = Date.parse(words.asOf);
  let newest: { row: Activity; text: string } | undefined;
  for (const row of rows ?? []) {
    const text = saidIn(row);
    if (
      text &&
      activityHasHappened(row, asOf, words.record) &&
      (!newest ||
        Date.parse(row.occurred_at) > Date.parse(newest.row.occurred_at))
    ) {
      newest = { row, text };
    }
  }
  if (!newest) {
    return undefined;
  }
  const { row, text } = newest;
  const { t, locale, zone } = words;
  return {
    byMargince: row.automatic_kind === "product",
    kind: t(labelFor(KIND_WORD, row.kind)),
    detail: messageDetail(row, t) ?? colleagueName(row),
    when: activityDay(
      row.occurred_at,
      row.occurred_precision,
      locale,
      zone,
      formatDayMonth,
    ),
    text,
    onOpen: words.onOpen,
  };
}

// The words a row says about the record, or undefined when it says none.
function saidIn(row: Activity): string | undefined {
  if (
    row.kind === "task" ||
    row.archived_at ||
    row.content_state === "withheld" ||
    row.capture_label === "noise" ||
    SAYS_NOTHING.has(row.automatic_kind ?? "")
  ) {
    return undefined;
  }
  if (NOT_HELD.has(row.meeting_status ?? "")) {
    return undefined;
  }
  const own =
    row.kind === "email" || row.kind === "message"
      ? (row.email_summary?.preview ?? displayedBody(row))
      : (displayedBody(row) ?? displayedSubject(row));
  return own?.trim() || undefined;
}

// A note names the colleague who wrote it; a machine's note names nobody.
function colleagueName(row: Activity): string | undefined {
  const by = row.created_by;
  return by?.actor_type === "human"
    ? by.actor_name?.trim() || undefined
    : undefined;
}
