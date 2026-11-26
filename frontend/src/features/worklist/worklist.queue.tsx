import { type ReactNode, useState } from "react";
import { Badge } from "../../design-system/atoms";
import { Panel } from "../../design-system/panel";
import { useT } from "../../i18n";
import type { WorklistView } from "../../lib/worklistviews";
import {
  bandSections,
  canReportEmptyBands,
  unbandedRows,
} from "./worklist.bands";
import { pickFor, type WorklistPicks } from "./worklist.bulkdone";
import { reviewView } from "./worklist.destinations";
import { DomainReview, DomainReviewLauncher } from "./worklist.domainreview";
import { hasPane } from "./worklist.pane";
import type { Worklist, WorklistItem } from "./worklist.queries";
import { QueueBand } from "./worklist.queuebands";
import { WorklistRow } from "./worklist.row";
import { rowIdentity } from "./worklist.rowidentity";

/**
 * The reader has put every row down.
 *
 * Distinct from `""`, which means they have chosen nothing yet. With nothing
 * chosen the pane falls back to the first row. A "closed" spelled as `""` would
 * resolve back to row one, and the rank button would do nothing when pressed.
 *
 * An identity no row can carry: `rowIdentity` joins a source and an id, and no
 * source is empty.
 */
export const NOTHING_IN_HAND = "none";

/**
 * The row the pane is about.
 *
 * Resolved from the identity rather than held as the item itself. A refetch
 * replaces every row object, and a held one would describe a version of the day
 * that is no longer on screen.
 *
 * With nothing chosen this is the first row. That makes the top of the queue
 * the focus without a card above it saying so. The queue is the ranking's
 * answer to what matters most, and a separate card would draw that row twice.
 *
 * The fallback is not written back into state. The reader chose nothing, so
 * there is nothing to remember. A refetch that reorders the day moves this with
 * it, where a stored id would pin the pane to the row first at page load.
 */
export function rowInHand(
  queue: readonly WorklistItem[],
  selectedId: string,
): WorklistItem | undefined {
  const chosen = queue.find((item) => rowIdentity(item) === selectedId);
  if (chosen) {
    return chosen;
  }
  if (selectedId !== "") {
    return undefined;
  }
  // Only a row that has a pane is taken up by default. A row without one draws
  // no aside and offers no rank button. Marking it selected would leave the
  // accent stripe on a row the reader cannot open or put down, and it never clears.
  //
  // The first such row rather than the first row. A day led by a deal still has
  // a contact further down whose context is worth standing open. Skipping to it
  // keeps the pane useful without moving the queue's own order.
  return queue.find(hasPane);
}

// Everything a row needs that is the same for every row on the page.
//
// One object rather than eight props threaded through each section. The banded
// sections and the unbanded tail cannot drift into two spellings of one wiring.
export type RowContext = Readonly<{
  // Each row's place in the whole queue, by identity.
  //
  // Built once for the page rather than searched per row. The sections partition
  // a list that grows with every "load more". Asking the queue for each row's
  // index would walk it once per row, and cost more the further a reader pages.
  // It is keyed on the identity because the rest of this file compares rows by it.
  positions: ReadonlyMap<string, number>;
  owner: string;
  selectedId: string;
  onSelect: (next: string) => void;
  onOpenEmail: (activityId: string) => void;
  onView: (next: WorklistView) => void;
  picks: WorklistPicks;
}>;

// A run of rows under one heading, or the unbanded tail.
//
// `position` is the row's place in the whole queue, not in this run. The rank
// number is the page's promise about the order, and restarting it at each
// heading would print two number ones.
export function QueueRows({
  items,
  positions,
  owner,
  selectedId,
  onSelect,
  onOpenEmail,
  onView,
  picks,
}: RowContext & Readonly<{ items: readonly WorklistItem[] }>) {
  const [domainReview, setDomainReview] = useState(false);
  return (
    <>
      <ol className="worklist-list">
        {items.map((item) => (
          <li key={rowIdentity(item)}>
            <WorklistRow
              item={item}
              position={(positions.get(rowIdentity(item)) ?? 0) + 1}
              owner={owner}
              selected={selectedId === rowIdentity(item)}
              // Only where pressing it opens something. WorklistRow draws a plain
              // number without this, and the Brief relies on that. A rank that
              // toggles a pressed state and opens nothing teaches the reader that
              // the page lies about what is pressable.
              onSelect={
                hasPane(item)
                  ? () =>
                      onSelect(
                        selectedId === rowIdentity(item)
                          ? NOTHING_IN_HAND
                          : rowIdentity(item),
                      )
                  : undefined
              }
              onOpenEmail={onOpenEmail}
              pick={pickFor(item, picks)}
              onReview={() =>
                item.batch?.key === "domain_questions"
                  ? setDomainReview(true)
                  : onView(reviewView(item))
              }
            />
          </li>
        ))}
      </ol>
      {domainReview && <DomainReview onClose={() => setDomainReview(false)} />}
    </>
  );
}

// The Decisions or System view's rows in one panel, named by the view. The
// domain review launcher stays on the Decisions panel even when it is empty.
export function ViewPanel({
  view,
  items,
  rows,
  select,
  children,
}: Readonly<{
  view: Exclude<WorklistView, "today">;
  items: readonly WorklistItem[];
  rows: RowContext;
  select?: ReactNode;
  children?: ReactNode;
}>) {
  const t = useT();
  const launcher = view === "decisions" ? <DomainReviewLauncher /> : undefined;
  if (items.length === 0) {
    return launcher ?? null;
  }
  return (
    <Panel
      title={t(`worklist.view.${view}`)}
      titleAction={
        <>
          {launcher}
          {select}
        </>
      }
    >
      <QueueRows items={items} {...rows} />
      {children}
    </Panel>
  );
}

// Today's rows under the server's band headings. Select is one of the head's
// actions, so the head lays it out beside the badge and wraps them.
export function TodayPanel({
  day,
  items,
  hasMore,
  rows,
  select,
}: Readonly<{
  day: Worklist;
  items: readonly WorklistItem[];
  hasMore: boolean;
  rows: RowContext;
  select: ReactNode;
}>) {
  const t = useT();
  return (
    <Panel
      // Indigo, the band the Brief's Focus panel wears and a
      // record's "what needs you today" pane before it. These rows
      // are the agent's reading of the day: what it ranked and what
      // it prepared. Indigo is the one claim the product makes about
      // who did that. The same panel is drawn on this page and in
      // the queue drawer, so the claim reads the same in both.
      tone="ai"
      title={t("worklist.queue")}
      titleAction={
        <>
          <Badge tone="ai">{t("co.assistant.aiTag")}</Badge>
          {select}
        </>
      }
    >
      {/* The headings come from the SERVER's band list, in its draw
      order, rather than from the rows — which is the only way a
      band holding nothing can say so. Ranks are still counted over
      the whole queue, so a row's number is its place on the page
      and not its place within its heading. */}
      {bandSections(day, items).map((section, at, all) => (
        <QueueBand
          // The band and how often it opened before, which a
          // row leaving the run does not change.
          key={`${section.band}:${all.slice(0, at).filter((earlier) => earlier.band === section.band).length}`}
          section={section}
          canReportEmpty={canReportEmptyBands(hasMore)}
          rows={QueueRows}
          rowProps={rows}
        />
      ))}
      {/* Rows an older server sent with no band. Real work, drawn under
      no heading rather than dropped to keep the sections tidy. */}
      {unbandedRows(items).length > 0 && (
        <QueueRows items={unbandedRows(items)} {...rows} />
      )}
    </Panel>
  );
}
