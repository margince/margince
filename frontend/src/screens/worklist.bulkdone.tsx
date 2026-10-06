// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useState } from "react";
import { Button, Checkbox } from "../design-system/atoms";
import { SelectionBar } from "../design-system/listtable";
import { formatNumber } from "../format/format";
import { useLocale, usePlural, useT } from "../i18n";
import { BulkChangeDialog, type BulkChangeRequest } from "./bulkchange";
import type { WorklistItem } from "./worklist.queries";

// Marking several Worklist tasks and promises done in one change.
//
// Only a row whose own Done marks a task or a promise done is selectable: a
// waiting message, an approval and a system row carry no checkbox. "Select
// all" takes every such row the Worklist has LOADED under its current filter,
// and its label says "shown": rows past the last loaded page are not selected.
// The change runs through the bulk dialog, which previews it, asks for the
// confirmation above ten rows, reports the rows it left alone, and offers one
// Undo for the whole batch.

/** The sources whose Done marks a task or a promise done. */
const BULK_DONE_SOURCES: ReadonlySet<string> = new Set([
  "task",
  "conversation_claim",
]);

/** Whether this row takes a checkbox. */
export function bulkDoneEligible(item: WorklistItem): boolean {
  return (
    BULK_DONE_SOURCES.has(item.source) &&
    item.category !== "system" &&
    item.actions.includes("complete") &&
    item.version !== undefined
  );
}

/** The rows the reader has ticked, out of the ones on screen. */
export type WorklistPicks = Readonly<{
  picked: ReadonlySet<string>;
  toggle: (item: WorklistItem) => void;
}>;

/**
 * The selection over the loaded queue. Only rows the queue still holds count,
 * so a row a refetch or a filter took away cannot stay selected out of sight.
 */
export function useWorklistPicks(queue: readonly WorklistItem[]) {
  const [ticked, setTicked] = useState<ReadonlySet<string>>(new Set());
  const eligible = queue.filter(bulkDoneEligible);
  const rows = eligible.filter((item) => ticked.has(item.id));
  const picks: WorklistPicks = {
    picked: new Set(rows.map((item) => item.id)),
    toggle: (item) =>
      setTicked((prev) => {
        const next = new Set(prev);
        if (next.has(item.id)) {
          next.delete(item.id);
        } else {
          next.add(item.id);
        }
        return next;
      }),
  };
  return {
    picks,
    eligible,
    rows,
    selectAll: () => setTicked(new Set(eligible.map((item) => item.id))),
    clear: () => setTicked(new Set()),
  };
}

/** One eligible row's checkbox. */
export function RowPick({
  item,
  picks,
}: Readonly<{ item: WorklistItem; picks: WorklistPicks }>) {
  const t = useT();
  return (
    // Named by aria-label rather than a hidden label span: the row already
    // prints its title, and hidden text would be a second copy of it.
    <Checkbox
      label={null}
      aria-label={t("bulk.selectRow", { name: item.title ?? item.id })}
      checked={picks.picked.has(item.id)}
      onChange={() => picks.toggle(item)}
    />
  );
}

/**
 * The bar over the queue while it holds a row that can be marked done: the
 * way to select every such row shown, and once any is selected, the count and
 * Mark done.
 */
export function WorklistBulkBar({
  selection,
}: Readonly<{ selection: ReturnType<typeof useWorklistPicks> }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const [request, setRequest] = useState<BulkChangeRequest | null>(null);
  const { eligible, rows, selectAll, clear } = selection;
  if (eligible.length === 0 && request === null) {
    return null;
  }
  const markDone = () =>
    setRequest({
      recordType: "worklist_item",
      verb: "complete",
      rows: rows.map((item) => ({
        id: item.id,
        version: item.version,
        label: item.title ?? item.id,
      })),
      openId: crypto.randomUUID(),
    });
  return (
    <SelectionBar>
      {rows.length > 0 && (
        <span className="t-caption">
          {plural("bulk.selected", rows.length, {
            count: formatNumber(rows.length, locale),
          })}
        </span>
      )}
      {rows.length < eligible.length && (
        <Button onClick={selectAll}>
          {plural("worklist.bulk.selectAll", eligible.length, {
            count: formatNumber(eligible.length, locale),
          })}
        </Button>
      )}
      {rows.length > 0 && (
        <>
          <Button onClick={clear}>{t("worklist.bulk.clear")}</Button>
          <Button variant="primary" onClick={markDone}>
            {t("worklist.bulk.markDone")}
          </Button>
        </>
      )}
      <BulkChangeDialog
        request={request}
        onClose={() => setRequest(null)}
        onDone={() => {
          setRequest(null);
          clear();
        }}
      />
    </SelectionBar>
  );
}
