// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Search } from "lucide-react";
import { type ReactNode, useId, useState } from "react";
import { SegmentedControl } from "../../design-system/atoms";
import { IconAction } from "../../design-system/iconaction";
import type {
  ActivityKind,
  TimelineFilters,
} from "../../design-system/recordtimeline";
import { TimelineFilterBar } from "../../design-system/timelinefilterbar";
import { useT } from "../../i18n";
import type { TimelineFilter } from "../../lib/timelinefilter";
import "./leads.css";

// One row of cuts in the History head: the whole record, one kind of
// exchange, or what changed. A kind cut writes `filters.kind`, the same value
// the narrowing row's kind select writes, so the two never disagree.
const HISTORY_CUTS = [
  "all",
  "email",
  "meeting",
  "call",
  "note",
  "changes",
] as const;
type HistoryCut = (typeof HISTORY_CUTS)[number];
type KindCut = Exclude<HistoryCut, "all" | "changes">;

function isKindCut(kind: ActivityKind | undefined): kind is KindCut {
  return HISTORY_CUTS.some((cut) => cut === kind);
}

function historyCutOf(
  filter: TimelineFilter,
  filters: TimelineFilters,
): HistoryCut {
  if (filter === "changes") {
    return "changes";
  }
  return isKindCut(filters.kind) ? filters.kind : "all";
}

/**
 * useLeadHistoryDials draws the History card's dials: `cuts` for the card's
 * head. `narrowing` (search, date range, the kinds with no cut of their
 * own) sits under it once the reader opens it. A narrowing still in force keeps
 * the row open, so the list never reads as filtered by nothing.
 */
export function useLeadHistoryDials({
  filter,
  onFilter,
  filters,
  onFilters,
}: Readonly<{
  filter: TimelineFilter;
  onFilter: (next: TimelineFilter) => void;
  filters: TimelineFilters;
  onFilters: (next: TimelineFilters) => void;
}>): { cuts: ReactNode; narrowing: ReactNode } {
  const t = useT();
  const rowId = useId();
  const [searching, setSearching] = useState(false);
  const narrowed = Boolean(
    filters.q ||
      filters.after ||
      filters.before ||
      (filters.kind && !isKindCut(filters.kind)),
  );
  const open = filter !== "changes" && (searching || narrowed);
  const pick = (cut: HistoryCut) => {
    if (cut === "changes") {
      onFilter("changes");
      return;
    }
    onFilter("all");
    onFilters({ ...filters, kind: cut === "all" ? undefined : cut });
  };
  // Closing the row takes back what it narrowed: a hidden search still
  // filtering the list would be a filter nobody can see.
  const toggle = () => {
    setSearching(!open);
    if (open && narrowed) {
      onFilters({ kind: isKindCut(filters.kind) ? filters.kind : undefined });
    }
  };
  const labels: Record<HistoryCut, string> = {
    all: t("chronology.all"),
    email: t("timeline.filters.kind.email"),
    meeting: t("timeline.filters.kind.meeting"),
    call: t("timeline.filters.kind.call"),
    note: t("timeline.filters.kind.note"),
    changes: t("chronology.changes"),
  };
  return {
    cuts: (
      <span className="lead-history-dials">
        {filter !== "changes" && (
          <IconAction
            label={t("timeline.filters.search")}
            icon={<Search aria-hidden />}
            disclosure={{ expanded: open, controls: rowId }}
            onClick={toggle}
          />
        )}
        <SegmentedControl
          options={HISTORY_CUTS}
          value={historyCutOf(filter, filters)}
          onChange={pick}
          labels={labels}
          label={t("chronology.label")}
        />
      </span>
    ),
    narrowing: open ? (
      <div id={rowId}>
        <TimelineFilterBar value={filters} onChange={onFilters} />
      </div>
    ) : null,
  };
}
