// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// What the machinery did since the reader's last brief. Four lanes: what
// waits on a human, what did not land, what must be restored, and what already
// happened. A receipt and a router, never a second inbox: no row carries a
// verb, and a part with nothing to show draws nothing.

import { useId } from "react";
import { rowsOf } from "../../api/rows";
import { Button } from "../../design-system/atoms";
import { ErrorLine } from "../../design-system/errorline";
import { Eyebrow } from "../../design-system/eyebrow";
import { PanelBody } from "../../design-system/panel";
import { SurfaceState } from "../../design-system/surfacestate";
import { floorFigure } from "../../format/figure";
import { formatNumber } from "../../format/format";
import { useLocale, usePlural, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { useCan } from "../../lib/capability";
import { labelFor } from "../../lib/labelfor";
import {
  fillsPage,
  lacksLane,
  MAGIC_LANES,
  type MagicLane,
  type MagicLine,
  type MagicReceipt,
  magicFailed,
  useMagic,
} from "../../lib/magic.queries";
import { doneLinesApart } from "../../lib/magiccopy";
import { useViewerZone } from "../../lib/viewerzone";
import { sourceUnavailableText } from "../../lib/worklistcopy";
import { MagicDoneCard } from "./magic.done";
import { MagicLaneSection } from "./magic.lines";
import { spansDays } from "./magic.marks";
import "../brief/brief.css";

/** Every lane but the done one, which Home draws in its Done for you section. */
export const WAITING_LANES: readonly MagicLane[] = MAGIC_LANES.filter(
  (lane) => lane !== "done",
);

const LANE_HEADING: Readonly<Record<MagicLane, MessageKey>> = {
  done: "magic.lane.done",
  needs_you: "magic.lane.needsYou",
  could_not_complete: "magic.lane.couldNotComplete",
  watching: "magic.lane.watching",
};

type SourceUnavailable = MagicReceipt["sources_unavailable"][number];

/**
 * The lanes in `lanes`, in reading order. Home draws the waiting lanes here and
 * the done lane `embedded` in its Done for you section, which brings the
 * heading. Only the part that draws the done lane says the read failed or came
 * back short, so Home says it once.
 */
export function MagicPanel({
  visibleApprovalIds = [],
  visibleReceiptIds = [],
  lanes = MAGIC_LANES,
  embedded = false,
}: Readonly<{
  visibleApprovalIds?: readonly string[];
  visibleReceiptIds?: readonly string[];
  lanes?: readonly MagicLane[];
  embedded?: boolean;
}> = {}) {
  const t = useT();
  const { locale } = useLocale();
  const plural = usePlural();
  // The READER's own zone. A receipt says when something happened to them, and
  // an instant rendered in UTC asks them to do the arithmetic.
  const zone = useViewerZone();
  // Embedded, this part mounts to say a read failed. Reading again on that
  // mount would blank the failure and unmount it, so only Retry reads again.
  const magic = useMagic(useCan("deal", "read"), !embedded);
  const titleId = useId();
  const receipt = magic.data;
  const state = queryState(magic);
  // Lines only off a read that answered: a refresh that failed keeps the last
  // answer cached, and drawing it would offer undos under "did not load".
  const shown = distinctReceipt(
    state === "ready" ? receipt : undefined,
    visibleApprovalIds,
    visibleReceiptIds,
  );
  const drawn = MAGIC_LANES.filter(
    (lane) => lanes.includes(lane) && hasLines(shown?.[lane]),
  );
  const sources = lanes.some((lane) => lane !== "done")
    ? rowsOf(shown?.sources_unavailable)
    : [];
  const doneRecords = recordsOf(rowsOf(shown?.done));
  // A lane the server read to its bound may hold more than it drew.
  const cut = new Set(rowsOf(receipt?.cut_at_limit));
  const doneFloor =
    sumIsFloor("done", rowsOf(receipt?.done)) ||
    receipt?.done_is_floor === true ||
    cut.has("done");
  const doneFold = plural("magic.done.fold", doneRecords, {
    count: floorFigure(
      formatNumber(doneRecords, locale),
      doneFloor,
      doneRecords,
    ),
  });
  const byDay = shown ? spansDays(shown.since, shown.as_of) : false;
  // The done lane says "at least" in its fold. The waiting lanes say it here,
  // so a cut queue never reads as the whole of it. Watching has no bound.
  const laneNote = (lane: MagicLane): string | undefined =>
    lane !== "done" && lane !== "watching" && cut.has(lane)
      ? t("magic.laneCut")
      : undefined;
  const read = panelRead(
    state,
    shown,
    lanes.includes("done"),
    drawn.length > 0 || sources.some(failedSource),
  );
  // A section of its own stays away until its read lands. Drawn as a loader,
  // it took a block of the page and then left it again when nothing waited.
  // Embedded, the section around it already waits for its own read.
  if (read === "empty" || (read === "loading" && !embedded)) {
    return null;
  }
  const body = (
    <>
      <ReadNotice
        read={read}
        sources={sources}
        retrying={magic.isFetching}
        onRetry={() => void magic.refetch()}
      />
      {drawn.map((lane) => (
        <DrawnLane
          key={lane}
          lane={lane}
          shown={shown}
          zone={zone}
          byDay={byDay}
          embedded={embedded}
          note={laneNote(lane)}
          done={{ fold: doneFold, floor: doneFloor }}
        />
      ))}
    </>
  );
  if (embedded) {
    return body;
  }
  return (
    <section className="brief-section" aria-labelledby={titleId}>
      <Eyebrow as="h2" id={titleId} className="brief-section-label">
        {t("magic.heading.brief")}
      </Eyebrow>
      {body}
    </section>
  );
}

/** One lane under its heading. Embedded, the done lane is Done for you's last card instead. */
function DrawnLane({
  lane,
  shown,
  zone,
  byDay,
  embedded,
  note,
  done,
}: Readonly<{
  lane: MagicLane;
  shown: MagicReceipt | undefined;
  zone: string;
  byDay: boolean;
  embedded: boolean;
  /** Why the lane may hold more than it draws, under it. */
  note: string | undefined;
  done: Readonly<{ fold: string; floor: boolean }>;
}>) {
  const t = useT();
  if (embedded && lane === "done") {
    return (
      <MagicDoneCard
        rows={rowsOf(shown?.done)}
        since={shown?.since}
        zone={zone}
        byDay={byDay}
        floor={done.floor}
      />
    );
  }
  return (
    <MagicLaneSection
      lane={lane}
      title={t(labelFor(LANE_HEADING, lane))}
      rows={shown?.[lane] ?? []}
      since={shown?.since}
      zone={zone}
      byDay={byDay}
      fold={lane === "done" ? done.fold : undefined}
      note={note}
    />
  );
}

function queryState(
  query: Parameters<typeof magicFailed>[0],
): "loading" | "failed" | "ready" {
  if (magicFailed(query)) return "failed";
  return query.isPending ? "loading" : "ready";
}

// Home says a failed or short read once: the part that draws the done lane says
// it, and the waiting part beside it stays away.
function panelRead(
  state: "loading" | "failed" | "ready",
  shown: MagicReceipt | undefined,
  drawsDone: boolean,
  hasContent: boolean,
): "loading" | "failed" | "ready" | "empty" | "unavailable" {
  if (state === "loading") return "loading";
  if (state === "failed") return drawsDone ? "failed" : "empty";
  if (drawsDone && lacksLane(shown)) return "unavailable";
  return hasContent ? "ready" : "empty";
}

// A source the reader's grant withholds is true on every read, so it never
// holds a section up by itself; one that failed is news.
function failedSource(source: SourceUnavailable): boolean {
  return source.reason === "failed";
}

// A lane off a payload this client cannot read is not an empty one: absent is
// version skew, and only a list the server sent can say there is nothing in it.
function hasLines(rows: readonly MagicLine[] | undefined): boolean {
  return rowsOf(rows).length > 0;
}

// How many records the lines stand for; a line without a count is one. One
// line may stand for 1,200 filed emails.
function recordsOf(rows: readonly MagicLine[]): number {
  return rows.reduce((sum, row) => sum + (row.count ?? 1), 0);
}

// A sum is only a floor over a line whose read was cut short, or over a lane
// that filled its page and may hold more lines than it shows.
function sumIsFloor(lane: MagicLane, rows: readonly MagicLine[]): boolean {
  return (
    fillsPage(lane, rows) || rows.some((row) => row.count_is_floor === true)
  );
}

/** What the read itself says before any lane: that it failed, is on its way, or left a source out. */
function ReadNotice({
  read,
  sources,
  retrying,
  onRetry,
}: Readonly<{
  read: "loading" | "failed" | "ready" | "unavailable";
  sources: readonly SourceUnavailable[];
  retrying: boolean;
  onRetry: () => void;
}>) {
  const t = useT();
  if (read === "failed") {
    return (
      <PanelBody>
        <ErrorLine
          actions={
            <Button variant="link" pending={retrying} onClick={onRetry}>
              {t("state.retry")}
            </Button>
          }
        >
          {t("magic.failed")}
        </ErrorLine>
      </PanelBody>
    );
  }
  if (read === "ready" && sources.length === 0) {
    return null;
  }
  return (
    <PanelBody>
      <SurfaceState
        state={read}
        emptyLabel=""
        loadingLabel={t("magic.loading")}
      >
        <WithheldSources withheld={sources} />
      </SurfaceState>
    </PanelBody>
  );
}

function WithheldSources({
  withheld,
}: Readonly<{ withheld: readonly SourceUnavailable[] }>) {
  const t = useT();
  if (withheld.length === 0) {
    return null;
  }
  return (
    <ul className="magic-withheld">
      {withheld.map((missing) => (
        <li className="t-caption" key={`${missing.source}-${missing.reason}`}>
          {sourceUnavailableText(missing, t)}
        </li>
      ))}
    </ul>
  );
}

function distinctReceipt(
  receipt: MagicReceipt | undefined,
  approvalIds: readonly string[],
  receiptIds: readonly string[],
): MagicReceipt | undefined {
  if (!receipt) return undefined;
  return {
    ...receipt,
    needs_you: receipt.needs_you?.filter(
      (line) => !approvalIds.includes(line.id),
    ),
    done: receipt.done && doneLinesApart(receipt.done, receiptIds),
  };
}
