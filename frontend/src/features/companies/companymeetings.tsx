// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { NextMeeting } from "../../components/meetingrows";
import { RecordMeetings } from "../../components/recordmeetings";
import { useViewerZone } from "../../lib/viewerzone";
import type { Company360View } from "./companies";
import { upcomingMeeting } from "./companynextmeeting";

// The account's next meeting in the shape the meeting card draws. The account
// read names an attendee `display_name` where the contact read says
// `full_name`; the rest is one shape.
function nextMeetingOf(
  view: Company360View,
  zone: string,
): NextMeeting | undefined {
  const meeting = upcomingMeeting(view, zone);
  if (!meeting) return undefined;
  const { participants, ...rest } = meeting;
  return {
    ...rest,
    participants: participants.map((who) => ({
      contact_id: who.contact_id,
      full_name: who.display_name,
    })),
  };
}

/**
 * CompanyMeetingsTab is the account's Meetings tab: every meeting filed
 * against the account, its deals or the contacts who work there. It has no
 * booking verb of its own, because a meeting is booked with one contact. The
 * Contacts tab leads to each of them.
 */
export function CompanyMeetingsTab({
  companyId,
  view,
  loading,
  onBriefMeeting,
}: Readonly<{
  companyId: string;
  view?: Company360View;
  loading: boolean;
  onBriefMeeting: (activityId: string) => void;
}>) {
  // The same rule the figures strip's Next meeting cell reads, so the tab and
  // the strip never name two different meetings.
  const zone = useViewerZone();
  return (
    <RecordMeetings
      entity="company"
      recordId={companyId}
      view={view}
      loading={loading}
      next={view && nextMeetingOf(view, zone)}
      onBriefMeeting={onBriefMeeting}
    />
  );
}
