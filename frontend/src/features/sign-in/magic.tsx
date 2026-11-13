// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// What the machinery did since the reader's last brief. Four lanes: what
// waits on a human, what did not land, what must be restored, and what already
// happened. A receipt and a router, never a second inbox: no row carries a
// verb, and a lane with nothing in it draws nothing.

import { useId } from "react";
import { rowsOf } from "../../api/rows";
import { Eyebrow } from "../../design-system/eyebrow";
import { PanelBody } from "../../design-system/panel";
import { SurfaceState } from "../../design-system/surfacestate";
import { floorFigure } from "../../format/figure";
import { formatNumber } from "../../format/format";
import { browserZone } from "../../format/timezone";
import { useLocale, usePlural, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { labelFor } from "../../lib/labelfor";
import {
  fillsPage,
  type MagicLane,
  type MagicLine,
  type MagicNotShown,
  type MagicReceipt,
  useMagic,
} from "../../lib/magic.queries";
import { sourceUnavailableText } from "../../lib/worklistcopy";
import { MagicLaneSection } from "./magic.lines";
import { spansDays } from "./magic.marks";
import "../brief/brief.css";

// The reading order: what waits on you, what broke, what needs restoring, and
// what already happened. A list rather than four call sites, so a fifth lane is
// one entry.
const LANES = [
  "needs_you",
  "could_not_complete",
  "watching",
  "done",
] as const satisfies readonly MagicLane[];

/** Every lane but the done one, which Home draws in its Done for you section. */
export const WAITING_LANES: readonly MagicLane[] = LANES.filter(
  (lane) => lane !== "done",
);

const LANE_HEADING: Readonly<Record<MagicLane, MessageKey>> = {
  done: "magic.lane.done",
  needs_you: "magic.lane.needsYou",
  could_not_complete: "magic.lane.couldNotComplete",
  watching: "magic.lane.watching",
};

// Housekeeping is left out without a count: it tells the reader nothing.
type ToldNotShown = MagicNotShown & {
  reason: Exclude<MagicNotShown["reason"], "unadmitted_action">;
};

const NOT_SHOWN_REASON: Readonly<Record<ToldNotShown["reason"], MessageKey>> = {
  unknown_entity_type: "magic.notShown.unknownEntityType",
  out_of_scope: "magic.notShown.outOfScope",
};

function told(entry: MagicNotShown): entry is ToldNotShown {
  return entry.reason !== "unadmitted_action";
}

/**
 * The lanes in `lanes`, in reading order. Home draws the waiting lanes here and
 * the done lane `embedded` in its Done for you section, which brings the
 * heading. Each part says only what is true of the lanes it draws: the done
 * lane its coverage, the others the sources and records left out.
 */
export function MagicPanel({
  visibleApprovalIds = [],
  visibleReceiptIds = [],
  lanes = LANES,
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
  const zone = browserZone();
  const magic = useMagic();
  const titleId = useId();
  const receipt = magic.data;
  const drawsDone = lanes.includes("done");
  const drawsWaiting = lanes.some((lane) => lane !== "done");
  const withheld = drawsWaiting ? (receipt?.sources_unavailable ?? []) : [];
  const state = queryState(magic);
  // Lines only off a read that answered: a refresh that failed keeps the last
  // answer cached, and drawing it would offer undos under "did not load".
  const shown = distinctReceipt(
    state === "ready" ? receipt : undefined,
    visibleApprovalIds,
    visibleReceiptIds,
  );
  const drawn = LANES.filter(
    (lane) => lanes.includes(lane) && hasLines(shown?.[lane]),
  );
  const done = rowsOf(shown?.done);
  const doneRecords = recordsOf(done);
  const coverageIncomplete = drawsDone && doneCoverageIncomplete(shown);
  // A lane the server read to its bound may hold more than it drew.
  const cut = new Set(rowsOf(receipt?.cut_at_limit));
  // The decision source read to its bound and still had more.
  const doneFloor =
    sumIsFloor("done", rowsOf(receipt?.done)) ||
    coverageIncomplete ||
    receipt?.done_is_floor === true ||
    cut.has("done");
  // "All" would claim the whole of a count that is only a floor.
  const doneFold = plural(
    doneFloor ? "magic.done.atLeast" : "magic.done.all",
    doneRecords,
    {
      count: floorFigure(
        formatNumber(doneRecords, locale),
        doneFloor,
        doneRecords,
      ),
    },
  );
  const byDay = shown ? spansDays(shown.since, shown.as_of) : false;
  // The done lane says "at least" in its fold. The waiting lanes say it here,
  // so a cut queue never reads as the whole of it.
  const laneNote = (lane: MagicLane): string | undefined => {
    if (lane === "done") {
      return coverageIncomplete ? t("magic.coverageIncomplete") : undefined;
    }
    return lane !== "watching" && cut.has(lane)
      ? t("magic.laneCut")
      : undefined;
  };
  const read = sectionRead(state, shown, lanes, drawn, coverageIncomplete);
  // A section of its own stays away until its read lands. Drawn as a loader,
  // it took a block of the page and then left it again when nothing waited.
  // Embedded, the section around it already waits for its own read.
  if (read === "empty" || (read === "loading" && !embedded)) {
    return null;
  }
  const notShown = drawsWaiting ? rowsOf(shown?.not_shown).filter(told) : [];
  const body = (
    <>
      <PanelBody>
        <SurfaceState
          state={read}
          emptyLabel=""
          loadingLabel={t("magic.loading")}
          detail={{ onRetry: () => void magic.refetch() }}
        >
          <WithheldSources withheld={withheld} />
          {drawsWaiting && shown?.not_shown_count_withheld && (
            <p className="t-caption">{t("magic.visibleScope")}</p>
          )}
          {coverageIncomplete && !drawn.includes("done") && (
            <p className="t-caption">{t("magic.coverageIncomplete")}</p>
          )}
        </SurfaceState>
      </PanelBody>
      {drawn.map((lane) => (
        <MagicLaneSection
          key={lane}
          lane={lane}
          title={t(
            embedded && lane === "done"
              ? "magic.lane.doneOther"
              : labelFor(LANE_HEADING, lane),
          )}
          rows={shown?.[lane] ?? []}
          since={shown?.since}
          zone={zone}
          byDay={byDay}
          fold={lane === "done" ? doneFold : undefined}
          note={laneNote(lane)}
        />
      ))}
      {notShown.length > 0 && (
        <PanelBody>
          <NotShown entries={notShown} />
        </PanelBody>
      )}
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

function queryState(
  query: Readonly<{ isPending: boolean; isError: boolean }>,
): "loading" | "failed" | "ready" {
  if (query.isPending) return "loading";
  return query.isError ? "failed" : "ready";
}

// A quiet window and an unread one are different answers. Only a read that
// answered completely, with nothing drawn, is empty and stays away. A lane
// the response did not carry is unread, not empty, and a count this reader
// may not be given is unmeasured.
function sectionRead(
  state: "loading" | "failed" | "ready",
  shown: MagicReceipt | undefined,
  lanes: readonly MagicLane[],
  drawn: readonly MagicLane[],
  coverageIncomplete: boolean,
): "loading" | "failed" | "ready" | "empty" | "unavailable" {
  if (state !== "ready") return state;
  if (lanes.some((lane) => !Array.isArray(shown?.[lane]))) {
    return "unavailable";
  }
  const waitingComplete =
    !lanes.some((lane) => lane !== "done") ||
    (rowsOf(shown?.sources_unavailable).length === 0 &&
      rowsOf(shown?.not_shown).length === 0 &&
      shown?.not_shown_count_withheld !== true);
  const doneComplete =
    !lanes.includes("done") ||
    (shown?.done_is_floor !== true && !coverageIncomplete);
  return drawn.length === 0 && waitingComplete && doneComplete
    ? "empty"
    : "ready";
}

function doneCoverageIncomplete(receipt: MagicReceipt | undefined): boolean {
  return (
    receipt?.not_shown?.some(
      (item) => item.reason === "unknown_entity_type",
    ) === true
  );
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

function WithheldSources({
  withheld,
}: Readonly<{ withheld: MagicReceipt["sources_unavailable"] }>) {
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

/**
 * What this read deliberately left out, by kind and count.
 *
 * Said rather than dropped, because a page showing five lines that never
 * mentions the sixth is a completeness claim nobody made.
 */
function NotShown({ entries }: Readonly<{ entries: readonly ToldNotShown[] }>) {
  const t = useT();
  const { locale } = useLocale();
  const plural = usePlural();
  return (
    <ul className="magic-notshown">
      {entries.map((entry) => (
        <li className="t-caption" key={entry.reason}>
          {plural("magic.notShown", entry.count, {
            count: formatNumber(entry.count, locale),
            reason: t(labelFor(NOT_SHOWN_REASON, entry.reason)),
          })}
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
    // A grouped line names every change it stands for, so it leaves only when
    // all of them are shown elsewhere. A line without ids always stays.
    done: receipt.done?.filter(
      (line) =>
        !line.receipt_ids?.length ||
        !line.receipt_ids.every((id) => receiptIds.includes(id)),
    ),
  };
}
