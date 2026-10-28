// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The lead's History tab, read off the same hook as every record page. All
// shows what was said to the lead, Changes what changed on it.
//
// The Changes cut swaps in the record-change audit (RecordHistoryTab), the one
// surface that can put a value back. A second rendering of the same changes
// beside it would be a second answer without the restore.

import { useQueryClient } from "@tanstack/react-query";
import { useEffect, useRef } from "react";
import type { components } from "../../api/schema";
import { LatestInboundPanel } from "../../components/latestinbound";
import { Panel, PanelBody } from "../../design-system/panel";
import {
  useRecordTimeline,
  useTimelineFilters,
} from "../../design-system/recordtimeline";
import { GroupedTimelineList } from "../../design-system/timeline";
import { groupChronology } from "../../design-system/timelinegroups";
import { useLocale, useT } from "../../i18n";
import type { LatestInbound } from "../../lib/latestinbound";
import { invalidateRecord } from "../../lib/recordwritekeys";
import { useRecordZone } from "../../lib/recordzone";
import { RecordHistoryTab, useFieldHistory } from "../records/history";
import {
  ChronologyFooter,
  chronologyNotice,
  hasChronologyFooter,
  useChronologyFilter,
  useRecordChronology,
} from "../records/recordchronology";
import { RowAnswerActions, TimelineActions } from "../records/timelineactions";
import { useLeadHistoryDials } from "./leadhistorydials";

type Lead = components["schemas"]["Lead"];
type FieldHistoryEntry = components["schemas"]["FieldHistoryEntry"];

/**
 * The lead's status changes, newest first, for the dates on its ladder. Only
 * the first page: a rung is dated by the newest move into it. Empty while
 * the read is out or refused, which leaves those rungs undated.
 *
 * Read again when the status moves with no write from this page, so a new
 * rung does not stand undated until the next visit. A write already refreshes
 * every field-history read through the query client.
 */
export function useLeadStatusChanges(
  lead: Pick<Lead, "id" | "status">,
): readonly FieldHistoryEntry[] {
  const query = useFieldHistory("lead", lead.id, { field: "status" });
  const { refetch } = query;
  const seen = useRef(lead.status);
  useEffect(() => {
    if (seen.current !== lead.status) {
      seen.current = lead.status;
      refetch();
    }
  }, [lead.status, refetch]);
  return query.data?.pages[0]?.data ?? [];
}

export function LeadHistoryTab({
  lead,
  onOpenEmail,
}: Readonly<{
  lead: Lead;
  // Opens one message in the record's drawer, which the page owns — the
  // page-level drawer stays mounted whichever tab is open, so a message
  // opened from here reads the same as one opened from the Overview call.
  onOpenEmail: (activityId: string) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const queryClient = useQueryClient();
  const recordZone = useRecordZone();
  const [filter, setFilter] = useChronologyFilter(lead.id);
  const [filters, setFilters] = useTimelineFilters(lead.id);
  // Older pages and every narrowed read come from the activity list itself —
  // a lead carries no 360 payload to seed a first page from.
  const timeline = useRecordTimeline("lead", lead.id, { filters });
  const chronology = useRecordChronology({
    onOpenEmail,
    kind: "lead",
    recordId: lead.id,
    filter,
    // RecordHistoryTab draws Changes here and pages its own feed.
    changesInPanel: true,
    activities: timeline.activities,
    activitiesHaveMore: timeline.hasNextPage,
    loadMore: timeline,
    // A lead holds no money of its own: the currency is absent and a
    // minor-unit column says so rather than printing a bare integer.
    values: { currency: null, locale, zone: recordZone },
    renderActions: (activity) => (
      <TimelineActions
        activity={activity}
        entityType="lead"
        entityId={lead.id}
      />
    ),
  });
  const dials = useLeadHistoryDials({
    filter,
    onFilter: setFilter,
    filters,
    onFilters: setFilters,
  });

  return (
    <Panel
      title={t("tab.history")}
      titleAction={dials.cuts}
      actions={
        hasChronologyFooter(filter, chronology) ? (
          <ChronologyFooter filter={filter} chronology={chronology} />
        ) : undefined
      }
    >
      <PanelBody>
        {dials.narrowing}
        {/* The Changes view IS the record's history: one reading of what
            changed on this lead, and the one that can put a change back. */}
        {filter === "changes" ? (
          <RecordHistoryTab
            kind="lead"
            id={lead.id}
            restore={{
              version: lead.version,
              onRestored: () => invalidateRecord(queryClient, "lead", lead.id),
            }}
          />
        ) : (
          <div className="timeline-card">
            {chronologyNotice(
              "lead.timeline.empty",
              filter,
              chronology,
              {
                loading: timeline.isPending,
                failed: timeline.isError,
                assembled: timeline.isSuccess,
              },
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

/**
 * The newest unanswered thing the lead sent, the website form that brought
 * them included, leading the Overview with the verbs its History row has.
 */
export function LeadLatestInbound({
  latest,
  leadId,
}: Readonly<{ latest: LatestInbound | undefined; leadId: string }>) {
  return (
    <LatestInboundPanel
      latest={latest}
      actions={(activity) => (
        <RowAnswerActions
          activity={activity}
          entityType="lead"
          entityId={leadId}
        />
      )}
    />
  );
}
