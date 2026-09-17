// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The deal's History tab: ONE chronology, the same shape the lead's and the
// contact's read off the same hook (leadhistory.tsx, contacttabs.tsx). What
// was said about the deal and what was changed on it are one order of events,
// not the record's audit sitting apart from its story.
//
// The Changes cut swaps in the record-change audit (RecordHistoryTab) rather
// than drawing the field edits as ordinary rows: that panel is the one
// surface that can put a value back, and a second rendering of the same
// changes beside it would be two answers to one question with only one of
// them carrying the restore.

import { useQueryClient } from "@tanstack/react-query";
import type { components } from "../../api/schema";
import { useRecordZone } from "../../app/recordzone";
import { GroupedTimelineList } from "../../design-system/composed";
import { Panel, PanelBody } from "../../design-system/panel";
import {
  hasTimelineFilters,
  useRecordTimeline,
  useTimelineFilters,
} from "../../design-system/recordtimeline";
import { TimelineFilterBar } from "../../design-system/timelinefilterbar";
import { useLocale, useT } from "../../i18n";
import { RecordHistoryTab } from "../history";
import {
  ChronologyFilter,
  ChronologyFooter,
  chronologyNotice,
  hasChronologyFooter,
  useChronologyFilter,
  useRecordChronology,
} from "../recordchronology";
import { invalidateRecord } from "../recordwritekeys";
import { TimelineActions } from "../timelineactions";
import { groupChronology } from "../timelinegroups";

type Deal = components["schemas"]["Deal"];

export function DealHistoryTab({
  deal,
  onOpenEmail,
}: Readonly<{
  deal: Deal;
  // Opens one message in the record's drawer, which the page owns — the
  // page-level drawer stays mounted whichever tab is open, so a message
  // opened from here reads the same as one opened from the Overview call.
  onOpenEmail: (activityId: string) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const queryClient = useQueryClient();
  const recordZone = useRecordZone();
  const [filter, setFilter] = useChronologyFilter(deal.id);
  const [filters, setFilters] = useTimelineFilters(deal.id);
  const timeline = useRecordTimeline("deal", deal.id, { filters });
  const chronology = useRecordChronology({
    onOpenEmail,
    kind: "deal",
    recordId: deal.id,
    filter,
    // A narrowed read is a question about what was said, so the record's own
    // edits stand down: they are not meetings, and not what the reader asked.
    narrowed: hasTimelineFilters(filters),
    activities: timeline.activities,
    activitiesHaveMore: timeline.hasNextPage,
    loadMore: timeline,
    values: { currency: deal.currency ?? null, locale, zone: recordZone },
    renderActions: (activity) => (
      <TimelineActions
        activity={activity}
        entityType="deal"
        entityId={deal.id}
      />
    ),
  });

  return (
    <Panel
      title={t("tab.history")}
      actions={
        hasChronologyFooter(filter, chronology) ? (
          <ChronologyFooter filter={filter} chronology={chronology} />
        ) : undefined
      }
    >
      <PanelBody>
        {/* Both sets of dials in one block: the cuts through the chronology,
            then the narrowing of whichever cut is open — the same block the
            contact, the account and the lead draw them in. */}
        <div className="timeline-header">
          <ChronologyFilter filter={filter} onFilter={setFilter} />
          {filter !== "changes" && (
            <TimelineFilterBar value={filters} onChange={setFilters} />
          )}
        </div>
        {/* The Changes view IS the record's history: one reading of what
            changed on this deal, and the one that can put a change back. */}
        {filter === "changes" ? (
          <RecordHistoryTab
            kind="deal"
            id={deal.id}
            currency={deal.currency}
            restore={{
              version: deal.version,
              onRestored: () => invalidateRecord(queryClient, "deal", deal.id),
            }}
          />
        ) : (
          <div className="timeline-card">
            {chronologyNotice(
              "deal.timeline.empty",
              {
                loading: chronology.loading || timeline.isPending,
                failed: chronology.failed || timeline.isError,
                assembled: timeline.isSuccess,
                filter,
              },
              chronology.entries.length,
              t,
            ) ?? (
              <GroupedTimelineList
                groups={groupChronology(
                  chronology.entries,
                  timeline.hasNextPage,
                )}
                zone={recordZone}
              />
            )}
          </div>
        )}
      </PanelBody>
    </Panel>
  );
}
