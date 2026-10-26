// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
import type { components } from "../../api/schema";
import { DataTable, type DataTableColumn } from "../../design-system/datatable";
import { formatDateAbbrev } from "../../format/format";
import { useLocale, useT } from "../../i18n";
import { useMemberNames } from "../../lib/membernames";
import { useRecordZone } from "../../lib/recordzone";
import {
  MeetingStatus,
  MeetingTitle,
  meetingSubject,
  opensMeetingPage,
} from "./contactmeetings.rows";

type Activity = components["schemas"]["Activity"];

/**
 * The meetings already behind the read, one row each. A row shows what it was
 * and what became of it, the day, the colleague who hosted it, and the brief
 * verb.
 * The host column shows only once a row names a host, so a list logged
 * without one carries no column of blanks.
 */
export function HeldMeetingsTable({
  held,
  verbs,
}: Readonly<{
  held: readonly Activity[];
  verbs: (activity: Activity) => ReactNode;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = useRecordZone();
  const hosts = useMemberNames(
    held.map((meeting) => meeting.host_user_id ?? ""),
  );
  const hostOf = (meeting: Activity) =>
    meeting.host_user_id ? hosts.get(meeting.host_user_id) : undefined;
  const columns: DataTableColumn<Activity>[] = [
    {
      key: "meeting",
      header: t("contact.meetings.col.meeting"),
      grow: true,
      fold: "title",
      render: (meeting) => (
        <span className="pe-meeting-headline">
          <MeetingTitle
            activityId={meeting.id}
            opens={opensMeetingPage(meeting)}
          >
            {meetingSubject(meeting, t)}
          </MeetingTitle>
          <MeetingStatus
            outcome={meeting.meeting_status}
            invitation={meeting.invitation_status}
          />
        </span>
      ),
    },
    {
      key: "date",
      header: t("contact.meetings.col.date"),
      render: (meeting) => (
        <span className="t-num">
          {formatDateAbbrev(meeting.occurred_at, locale, zone)}
        </span>
      ),
    },
    ...(held.some((meeting) => hostOf(meeting))
      ? [
          {
            key: "with",
            header: t("contact.meetings.col.with"),
            render: (meeting: Activity) => hostOf(meeting) ?? null,
          },
        ]
      : []),
    {
      key: "verbs",
      header: t("contact.meeting.brief"),
      headerHidden: true,
      align: "end",
      fold: "end",
      render: verbs,
    },
  ];
  return (
    <DataTable
      bleed
      fold
      label={t("contact.meetings.past")}
      columns={columns}
      rows={[...held]}
      rowKey={(meeting) => meeting.id}
    />
  );
}
