// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
import { useT } from "../i18n";
import { Heading } from "./heading";
import { GroupedTimelineList, TimelineList } from "./timeline";
import type { TimelineEntry, TimelineGroup } from "./timelineentry";

// The record's story, under whatever the open tab drew. It owns the break
// above it, because the work column owns no interval. The deal's and the
// project's bodies met the chronology's heading at the border.
export function RecordStory({
  timeline,
  timelineGroups,
  onOpenThread,
  timelineHeader,
  timelineFooter,
  timelineNotice,
  timelineAnchorId,
  zone,
}: Readonly<{
  timeline: TimelineEntry[];
  timelineGroups?: readonly TimelineGroup[];
  onOpenThread?: (threadKey: string) => void;
  timelineHeader?: ReactNode;
  timelineFooter?: ReactNode;
  timelineNotice?: ReactNode;
  timelineAnchorId?: string;
  zone: string;
}>) {
  const t = useT();
  return (
    <section
      id={timelineAnchorId}
      className="record-timeline"
      aria-label={t("record.timeline")}
    >
      <Heading size="large" className="t-sub">
        {t("record.timeline")}
      </Heading>
      {/* The dials above the list are one block with one rhythm: the cuts
          through the chronology, then the narrowing of whichever cut is open.
          Rendered as bare siblings they touched, and two rows of controls
          with no interval between them read as one control that has
          wrapped. */}
      {timelineHeader && (
        <div className="timeline-header">{timelineHeader}</div>
      )}
      {/* The chronology sits on a card of its own, like every other body on
          the page. Loose on the page ground it read as the page's own text
          rather than as one of the record's sections, and the rail down its
          left had nothing to run inside. The notice takes no card: a sentence
          about why there are no rows is not a list of them. */}
      {timelineNotice ?? (
        <div className="timeline-card">
          {timelineGroups ? (
            <GroupedTimelineList
              groups={timelineGroups}
              zone={zone}
              onOpenThread={onOpenThread}
            />
          ) : (
            <TimelineList entries={timeline} zone={zone} />
          )}
        </div>
      )}
      {timelineFooter}
    </section>
  );
}
