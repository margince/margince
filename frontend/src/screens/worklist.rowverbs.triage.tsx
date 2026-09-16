// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
import { moveHref } from "./worklist.copy";
import { DispositionVerbs } from "./worklist.dispositions";
import { ReassignControl } from "./worklist.manager";
import type { WorklistItem } from "./worklist.queries";
import { BatchVerb, RowVerbs } from "./worklist.rowverbs";

/**
 * The verbs of the row IN HAND on the Brief, across the card's floor.
 *
 * The same options every row carries, in the order one row being ANSWERED
 * needs them: the set-asides lead from one edge, because declining steps away
 * from the work; the ways into it follow; and the move the product prepared
 * closes the line at the other edge, where the lane's answer stands. The verb
 * that only reaches the record is withheld — the card names and links that
 * record itself.
 */
export function TriageActs({
  item,
  href,
  owner,
  primary,
  equals,
  context,
  onReview,
  onOpenEmail,
}: Readonly<{
  item: WorklistItem;
  href: string | undefined;
  owner: string;
  primary?: ReactNode;
  equals?: ReactNode;
  context?: ReactNode;
  onReview?: () => void;
  onOpenEmail?: (id: string) => void;
}>) {
  return (
    <div className="worklist-row-acts">
      <span className="worklist-row-putdowns">
        <DispositionVerbs item={item} />
      </span>
      {context}
      {item.batch && onReview ? (
        <BatchVerb onReview={onReview} />
      ) : (
        <RowVerbs
          item={item}
          href={href}
          part="ways"
          move={moveHref(item)}
          onOpenEmail={onOpenEmail}
        />
      )}
      {/* Handing the task on is not a verb the card may drop: without it the
          one row a reader is answering is the one they cannot pass along. */}
      {item.source === "task" && !item.batch && (
        <ReassignControl item={item} owner={owner} />
      )}
      {equals}
      {!item.batch && (
        <RowVerbs
          item={item}
          href={href}
          part="act"
          move={moveHref(item)}
          onOpenEmail={onOpenEmail}
        />
      )}
      {primary}
    </div>
  );
}
