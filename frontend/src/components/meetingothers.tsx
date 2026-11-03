// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import { formatNumber } from "../format/format";
import { useLocale, usePlural } from "../i18n";

type MeetingOtherParticipant = components["schemas"]["MeetingOtherParticipant"];

/**
 * The attendees a meeting card names after its contacts: colleagues and
 * addresses, then how many more there are than the card names. `afterContacts`
 * says whether names come before it, so the list reads as one sentence.
 */
export function MeetingOthers({
  others = [],
  more = 0,
  afterContacts,
}: Readonly<{
  others?: readonly MeetingOtherParticipant[];
  more?: number;
  afterContacts: boolean;
}>) {
  const plural = usePlural();
  const { locale } = useLocale();
  if (others.length === 0 && more === 0) {
    return null;
  }
  const names = others.map((other) => other.name).join(", ");
  const tail =
    more > 0
      ? plural("meeting.attendeesMore", more, {
          count: formatNumber(more, locale),
        })
      : "";
  // After the contacts a name continues their list; a count alone follows it.
  const lead = afterContacts ? (names ? ", " : " ") : "";
  return (
    <span>
      {lead}
      {[names, tail].filter(Boolean).join(" ")}
    </span>
  );
}
