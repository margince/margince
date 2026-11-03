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
import { viewerZone } from "../../format/timezone";
import { useLocale, usePlural, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
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

const LANE_HEADING: Readonly<Record<MagicLane, MessageKey>> = {
  done: "magic.lane.done",
  needs_you: "magic.lane.needsYou",
  could_not_complete: "magic.lane.couldNotComplete",
  watching: "magic.lane.watching",
};

const NOT_SHOWN_REASON: Readonly<Record<MagicNotShown["reason"], MessageKey>> =
  {
    unadmitted_action: "magic.notShown.unadmittedAction",
    unknown_entity_type: "magic.notShown.unknownEntityType",
    out_of_scope: "magic.notShown.outOfScope",
  };

export function MagicPanel({
  visibleApprovalIds = [],
  visibleReceiptIds = [],
}: Readonly<{
  visibleApprovalIds?: readonly string[];
  visibleReceiptIds?: readonly string[];
}> = {}) {
  const t = useT();
  const { locale } = useLocale();
  const plural = usePlural();
  // The READER's own zone. A receipt says when something happened to them, and
  // an instant rendered in UTC asks them to do the arithmetic.
  const zone = viewerZone();
  const magic = useMagic();
  const titleId = useId();
  const receipt = magic.data;
  const withheld = receipt?.sources_unavailable ?? [];
  const state = queryState(magic);
  // Lines only off a read that answered: a refresh that failed keeps the last
  // answer cached, and drawing it would offer undos under "did not load".
  const shown = distinctReceipt(
    state === "ready" ? receipt : undefined,
    visibleApprovalIds,
    visibleReceiptIds,
  );
  const drawn = LANES.filter((lane) => hasLines(shown?.[lane]));
  const done = rowsOf(shown?.done);
  const doneRecords = recordsOf(done);
  const coverageIncomplete = doneCoverageIncomplete(shown);
  // The decision source read to its bound and still had more.
  const doneFloor =
    sumIsFloor("done", rowsOf(receipt?.done)) ||
    coverageIncomplete ||
    receipt?.done_is_floor === true;
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
  const read = sectionRead(state, shown, drawn, coverageIncomplete);
  if (read === "empty") {
    return null;
  }
  return (
    <section className="brief-section" aria-labelledby={titleId}>
      <Eyebrow as="h2" id={titleId} className="brief-section-label">
        {t("magic.heading.brief")}
      </Eyebrow>
      <PanelBody>
        <SurfaceState
          state={read}
          emptyLabel=""
          loadingLabel={t("magic.loading")}
          detail={{ onRetry: () => void magic.refetch() }}
        >
          <WithheldSources withheld={withheld} />
          {shown?.not_shown_count_withheld && (
            <p className="t-caption">{t("magic.visibleScope")}</p>
          )}
          {coverageIncomplete && (
            <p className="t-caption">{t("magic.coverageIncomplete")}</p>
          )}
        </SurfaceState>
      </PanelBody>
      {drawn.map((lane) => (
        <MagicLaneSection
          key={lane}
          lane={lane}
          title={t(LANE_HEADING[lane])}
          rows={shown?.[lane] ?? []}
          since={shown?.since}
          zone={zone}
          byDay={byDay}
          fold={lane === "done" ? doneFold : undefined}
        />
      ))}
      {(shown?.not_shown?.length ?? 0) > 0 && (
        <PanelBody>
          <NotShown entries={shown?.not_shown ?? []} />
        </PanelBody>
      )}
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
  drawn: readonly MagicLane[],
  coverageIncomplete: boolean,
): "loading" | "failed" | "ready" | "empty" | "unavailable" {
  if (state !== "ready") return state;
  if (LANES.some((lane) => !Array.isArray(shown?.[lane]))) {
    return "unavailable";
  }
  const complete =
    rowsOf(shown?.sources_unavailable).length === 0 &&
    rowsOf(shown?.not_shown).length === 0 &&
    shown?.not_shown_count_withheld !== true &&
    shown?.done_is_floor !== true &&
    !coverageIncomplete;
  return drawn.length === 0 && complete ? "empty" : "ready";
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
function NotShown({
  entries,
}: Readonly<{ entries: readonly MagicNotShown[] }>) {
  const t = useT();
  const { locale } = useLocale();
  const plural = usePlural();
  return (
    <ul className="magic-notshown">
      {entries.map((entry) => (
        <li className="t-caption" key={entry.reason}>
          {plural("magic.notShown", entry.count, {
            count: formatNumber(entry.count, locale),
            reason: t(NOT_SHOWN_REASON[entry.reason]),
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
    done: receipt.done?.filter(
      (line) =>
        (line.count ?? 1) > 1 ||
        !line.receipt_ids?.length ||
        !line.receipt_ids.every((id) => receiptIds.includes(id)),
    ),
  };
}
