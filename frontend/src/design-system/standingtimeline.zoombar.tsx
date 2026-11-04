// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useEffect, useEffectEvent } from "react";
import { useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { SegmentedControl } from "./atoms";
import type { RecordTimeline } from "./recordtimeline";
import { TIMELINE_ZOOMS, type TimelineZoom } from "./standingtimeline.zoom";

/** The record's activity list, which pages older rows in when the zoom reaches
 *  past the oldest the page holds. */
export type TimelineHistory = Pick<
  RecordTimeline,
  "hasNextPage" | "isFetchingNextPage" | "fetchNextPage" | "isError"
> &
  // Counted for the cap, and dated to tell whether the zoom is reached.
  Readonly<{ activities: readonly Readonly<{ occurred_at: string }>[] }>;

/** Whether older rows are on their way, stopped at the cap or a failed page,
 *  or all in. */
export type OlderHistory = "loading" | "capped" | "failed" | "done";

// The most rows the line pages in; older ones stay on the History tab.
const HISTORY_CAP = 200;

const ZOOM_LABEL: Readonly<Record<TimelineZoom, MessageKey>> = {
  month: "standing.timeline.zoom.month",
  quarter: "standing.timeline.zoom.quarter",
  year: "standing.timeline.zoom.year",
  all: "standing.timeline.zoom.all",
};

const OLDER_NOTE: Readonly<Record<Exclude<OlderHistory, "done">, MessageKey>> =
  {
    loading: "standing.timeline.loadingOlder",
    capped: "standing.timeline.olderOnHistory",
    failed: "standing.timeline.olderFailed",
  };

/**
 * Pages older activities in while the zoom reaches past the oldest row on
 * hand, one page at a time, until `HISTORY_CAP` rows are loaded. Every row
 * counts, a task that draws no mark too. `from` is null on All time, which
 * wants every page. A failed page stops it, so a refusal is not asked again.
 */
export function useOlderHistory(
  history: TimelineHistory | undefined,
  from: number | null,
): OlderHistory {
  const rows = history?.activities ?? [];
  const oldest = Math.min(...rows.map((row) => Date.parse(row.occurred_at)));
  const reaches = from === null || oldest > from;
  const more = Boolean(history?.hasNextPage) && reaches;
  const wanted = more && !history?.isError;
  const capped = rows.length >= HISTORY_CAP;
  const fetching = history?.isFetchingNextPage ?? false;
  const fetchOlder = useEffectEvent(() => history?.fetchNextPage());
  useEffect(() => {
    if (wanted && !capped && !fetching) {
      fetchOlder();
    }
  }, [wanted, capped, fetching]);
  if (more && history?.isError) {
    return "failed";
  }
  if (!wanted) {
    return "done";
  }
  return capped ? "capped" : "loading";
}

/** The zoom over the line's right end, and a note while older rows load or
 *  once the cap leaves them to History. */
export function ZoomBar({
  zoom,
  onZoom,
  older,
}: Readonly<{
  zoom: TimelineZoom;
  onZoom: (next: TimelineZoom) => void;
  older: OlderHistory;
}>) {
  const t = useT();
  return (
    <div className="standing-timeline-head">
      {older !== "done" && (
        <span className="standing-timeline-older t-caption">
          {t(OLDER_NOTE[older])}
        </span>
      )}
      <SegmentedControl
        label={t("standing.timeline.zoom")}
        options={TIMELINE_ZOOMS}
        value={zoom}
        onChange={onZoom}
        labels={{
          month: t(ZOOM_LABEL.month),
          quarter: t(ZOOM_LABEL.quarter),
          year: t(ZOOM_LABEL.year),
          all: t(ZOOM_LABEL.all),
        }}
      />
    </div>
  );
}
