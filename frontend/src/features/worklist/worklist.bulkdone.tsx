// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useEffect, useRef, useState } from "react";
import {
  BulkChangeDialog,
  type BulkChangeRequest,
} from "../../components/bulkchange";
import { Button, Checkbox } from "../../design-system/atoms";
import { SelectionBar } from "../../design-system/selectionbar";
import { useToast } from "../../design-system/toast";
import { formatNumber } from "../../format/format";
import { useLocale, usePlural, useT } from "../../i18n";
import { useMarkRowsDone, type WorklistRowRef } from "./worklist.donequeries";
import type { WorklistItem } from "./worklist.queries";
import { rowIdentity } from "./worklist.rowidentity";

// Acting on several Worklist rows at once, in selection mode.
//
// Checkboxes show only after the reader presses Select. Two verbs stay apart.
// Complete finishes tasks and promises through the bulk dialog. Take off my
// list puts every other row down for this reader. "Select all" takes the rows
// loaded now, and rows that load later start unticked.

/** The sources whose Done marks a task or a promise done. */
const BULK_DONE_SOURCES: ReadonlySet<string> = new Set([
  "task",
  "conversation_claim",
]);

/** Whether Done on this row ticks a task or a promise done. */
function completesInBulk(item: WorklistItem): boolean {
  return (
    BULK_DONE_SOURCES.has(item.source) &&
    item.category !== "system" &&
    item.actions.includes("complete") &&
    item.version !== undefined
  );
}

/** Whether this row takes a checkbox. */
export function bulkDoneEligible(item: WorklistItem): boolean {
  return (
    completesInBulk(item) || (item.dispositions?.includes("handled") ?? false)
  );
}

/** The rows the reader has ticked, out of the ones on screen. */
export type WorklistPicks = Readonly<{
  /** Whether rows draw checkboxes: selection mode is on and one takes a box. */
  offered: boolean;
  picked: ReadonlySet<string>;
  toggle: (item: WorklistItem) => void;
}>;

/** No selection: rows draw no checkbox. */
export const NO_PICKS: WorklistPicks = {
  offered: false,
  picked: new Set(),
  toggle: () => undefined,
};

/**
 * The selection over the loaded queue. Only rows the queue still holds count,
 * so a row a refetch took away cannot stay selected out of sight.
 */
export function useWorklistPicks(
  queue: readonly WorklistItem[],
  /** Which cut of the day is loaded; a new one starts with nothing ticked. */
  view: string,
) {
  const [ticked, setTicked] = useState<ReadonlySet<string>>(new Set());
  const [selecting, setSelecting] = useState(false);
  // Cleared rather than kept per view: a selection carried back to a filter
  // the reader left would be acted on without being seen again.
  const [tickedFor, setTickedFor] = useState(view);
  if (tickedFor !== view) {
    setTickedFor(view);
    setTicked(new Set());
    setSelecting(false);
  }
  const eligible = queue.filter(bulkDoneEligible);
  const rows = eligible.filter((item) => ticked.has(rowIdentity(item)));
  const picks: WorklistPicks = {
    offered: selecting && eligible.length > 0,
    picked: new Set(rows.map(rowIdentity)),
    toggle: (item) =>
      setTicked((prev) => {
        const next = new Set(prev);
        const key = rowIdentity(item);
        if (next.has(key)) {
          next.delete(key);
        } else {
          next.add(key);
        }
        return next;
      }),
  };
  return {
    picks,
    eligible,
    rows,
    selecting,
    select: () => setSelecting(true),
    selectAll: () => setTicked(new Set(eligible.map(rowIdentity))),
    clear: () => setTicked(new Set()),
    untick: (items: readonly WorklistItem[]) =>
      setTicked((prev) => {
        const next = new Set(prev);
        for (const item of items) next.delete(rowIdentity(item));
        return next;
      }),
    stop: () => {
      setSelecting(false);
      setTicked(new Set());
    },
  };
}

/**
 * The row's checkbox. A row the change cannot mark done keeps an empty slot of
 * the same width while any row on screen has a box, so the queue stays aligned.
 */
export function pickFor(item: WorklistItem, picks: WorklistPicks) {
  if (!picks.offered) return undefined;
  if (bulkDoneEligible(item)) {
    return <RowPick item={item} picks={picks} />;
  }
  return <span className="worklist-row-pick-slot" aria-hidden="true" />;
}

function RowPick({
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
      checked={picks.picked.has(rowIdentity(item))}
      onChange={() => picks.toggle(item)}
    />
  );
}

/** Takes rows off the reader's own list in one call, with one Undo for all. */
function usePutDownRows() {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const toast = useToast();
  const done = useMarkRowsDone();
  return (rows: readonly WorklistRowRef[]) =>
    done.mutate(
      { rows },
      {
        onSuccess: () =>
          toast.show(
            plural("worklist.bulk.doneCount", rows.length, {
              count: formatNumber(rows.length, locale),
            }),
            {
              action: {
                kind: "undo",
                label: t("worklist.disposition.undo"),
                onAct: () =>
                  done.mutate(
                    { rows, reopen: true },
                    {
                      onError: () =>
                        toast.show(t("worklist.disposition.undoFailed"), {
                          tone: "danger",
                        }),
                    },
                  ),
              },
            },
          ),
        onError: () =>
          toast.show(t("worklist.disposition.failed"), { tone: "danger" }),
      },
    );
}

/**
 * Select, while the queue holds a row that takes a box. In selection mode it
 * shows the count, Select all, the two verbs for what is ticked, and Cancel.
 */
export function WorklistBulkBar({
  selection,
}: Readonly<{ selection: ReturnType<typeof useWorklistPicks> }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const [request, setRequest] = useState<BulkChangeRequest | null>(null);
  const { eligible, rows, selecting, select, selectAll, untick, stop } =
    selection;
  const putDown = usePutDownRows();
  const trigger = useFocusBackToSelect(selecting);
  if (eligible.length === 0 && request === null) {
    return null;
  }
  const completing = rows.filter(completesInBulk);
  const leaving = rows.filter((item) => !completesInBulk(item));
  const count = (n: number) => ({ count: formatNumber(n, locale) });
  if (!selecting && request === null) {
    return (
      <SelectionBar>
        <Button ref={trigger} onClick={select}>
          {t("worklist.bulk.select")}
        </Button>
      </SelectionBar>
    );
  }
  return (
    <SelectionBar>
      {rows.length > 0 && (
        <span className="t-caption">
          {plural("bulk.selected", rows.length, count(rows.length))}
        </span>
      )}
      {rows.length < eligible.length && (
        <Button onClick={selectAll}>
          {plural(
            "worklist.bulk.selectAll",
            eligible.length,
            count(eligible.length),
          )}
        </Button>
      )}
      {completing.length > 0 && (
        <Button
          variant="primary"
          onClick={() => setRequest(completeRequest(completing))}
        >
          {plural(
            "worklist.bulk.complete",
            completing.length,
            count(completing.length),
          )}
        </Button>
      )}
      {leaving.length > 0 && (
        <Button
          onClick={() => {
            putDown(
              leaving.map((item) => ({ source: item.source, row_id: item.id })),
            );
            untick(leaving);
          }}
        >
          {plural(
            "worklist.bulk.putDown",
            leaving.length,
            count(leaving.length),
          )}
        </Button>
      )}
      <Button onClick={stop}>{t("worklist.bulk.cancel")}</Button>
      <BulkChangeDialog
        request={request}
        onClose={() => setRequest(null)}
        onDone={() => {
          setRequest(null);
          // Only the completed rows leave; other picks stay ticked.
          untick(completing);
          if (leaving.length === 0) stop();
        }}
      />
    </SelectionBar>
  );
}

// Selection mode removes the button that was pressed to close it, so focus
// returns to Select once the mode ends.
function useFocusBackToSelect(selecting: boolean) {
  const trigger = useRef<HTMLButtonElement>(null);
  const wasSelecting = useRef(selecting);
  useEffect(() => {
    if (wasSelecting.current && !selecting) trigger.current?.focus();
    wasSelecting.current = selecting;
  }, [selecting]);
  return trigger;
}

// Each record once: two rows may name one record, and the change refuses an
// id named twice.
function completeRequest(
  completing: readonly WorklistItem[],
): BulkChangeRequest {
  return {
    recordType: "worklist_item",
    verb: "complete",
    rows: [
      ...new Map(
        completing.map((item) => [
          item.id,
          { id: item.id, version: item.version, label: item.title ?? item.id },
        ]),
      ).values(),
    ],
    openId: crypto.randomUUID(),
  };
}
