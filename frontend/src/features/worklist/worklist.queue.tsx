import { Panel } from "../../design-system/panel";
import { formatNumber } from "../../format/format";
import { useLocale, useT } from "../../i18n";
import { pickFor, type WorklistPicks } from "./worklist.bulkdone";
import { type ReviewShortfall, reviewFilter } from "./worklist.destinations";
import { hasPane } from "./worklist.pane";
import type { WorklistFilter, WorklistItem } from "./worklist.queries";
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
  onFilter: (next: WorklistFilter) => void;
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
  onFilter,
  picks,
}: RowContext & Readonly<{ items: readonly WorklistItem[] }>) {
  return (
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
            onReview={() => onFilter(reviewFilter(item))}
          />
        </li>
      ))}
    </ol>
  );
}

// The work that is not the day's, drawn below it and never hidden.
//
// Its own component, because WorklistBody already holds the split, the bands,
// the pane and the paging. A panel that decides what to admit about itself is
// one job more than one function should hold.
export function ReviewPanel({
  items,
  shortfall,
  more,
  rows,
}: Readonly<{
  items: readonly WorklistItem[];
  shortfall: ReviewShortfall | null;
  more: boolean;
  rows: RowContext;
}>) {
  const t = useT();
  const { locale } = useLocale();
  if (items.length === 0) {
    return null;
  }
  return (
    <Panel
      title={t("worklist.review")}
      // What this panel is not showing, in the footer band, which is where a
      // figure about the whole panel goes. A direct child of `Panel` sits in no
      // padding, while the rows are `PanelRow`s with the panel's own inset. A
      // bare `<p>` would start a `--padPanel` left of every row, outside the card.
      //
      // The panel has no cursor of its own: review rows arrive through the
      // day's one control below it. Without this footer, a reader with an
      // approval past the page cut sees a panel that looks complete.
      // The day's own total is the denominator, never drawn bare. It counts
      // every candidate the read weighed, so alone it would claim rows this
      // panel does not hold. Once the walk is over the gap is still true, but
      // there is no control left to point at.
      footer={
        shortfall?.floor
          ? t("worklist.review.floor", {
              loaded: formatNumber(shortfall.loaded, locale),
            })
          : shortfall
            ? t(
                more
                  ? "worklist.review.partial"
                  : "worklist.review.partialDone",
                {
                  loaded: formatNumber(shortfall.loaded, locale),
                  total: formatNumber(shortfall.total, locale),
                },
              )
            : undefined
      }
    >
      <QueueRows items={items} {...rows} />
    </Panel>
  );
}
