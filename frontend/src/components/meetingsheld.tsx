// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
import type { components } from "../api/schema";
import { useMemberNames } from "../lib/membernames";
import { MeetingRow } from "./meetingrows";

type Activity = components["schemas"]["Activity"];

/**
 * The meetings already held, drawn as the upcoming ones are. Each shows its
 * day as a calendar page, what it was and what became of it. It also names
 * the colleague who hosted it and carries its verbs.
 */
export function HeldMeetingsList({
  held,
  verbs,
}: Readonly<{
  held: readonly Activity[];
  verbs: (activity: Activity) => ReactNode;
}>) {
  const hosts = useMemberNames(
    held.map((meeting) => meeting.host_user_id ?? ""),
  );
  return held.map((meeting) => (
    <MeetingRow
      key={meeting.id}
      activity={meeting}
      host={meeting.host_user_id ? hosts.get(meeting.host_user_id) : undefined}
      verbs={verbs(meeting)}
    />
  ));
}
