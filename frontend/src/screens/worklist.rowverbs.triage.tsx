// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
import { moveHref } from "./worklist.copy";
import { DispositionVerbs } from "./worklist.dispositions";
import { ReassignControl } from "./worklist.manager";
import type { WorklistItem } from "./worklist.queries";
import { BatchVerb, PinVerb, RowVerbs } from "./worklist.rowverbs";

/**
 * The verbs of the row IN HAND on the Brief, across the card's floor.
 *
 * The same options every row carries, on one right-aligned line. The ways into
 * the record come first, then the reader's marks beside the menu of set-asides,
 * then Done. The prepared move closes the line, so the answer is at one x on
 * every row.
 *
 * The ORDER is all this shape is. What the frame around the row already
 * supplies is `framed`'s to withhold and `allowPin`'s to offer — the Brief's
 * card names and links its record and takes neither; the queue drawer reads
 * its rows in this order with no card around them and keeps both.
 */
export function TriageActs({
  item,
  href,
  owner,
  allowPin = true,
  framed = false,
  primary,
  equals,
  context,
  onReview,
  onOpenEmail,
}: Readonly<{
  item: WorklistItem;
  href: string | undefined;
  owner: string;
  /** The reader's own override of the ranking. The Brief's card withholds it —
   *  its order is the queue's, not the focus projection's — and every surface
   *  that IS the queue offers it. */
  allowPin?: boolean;
  /** A card around this row already names and links its record, so the verb
   *  that only reaches it is withheld. */
  framed?: boolean;
  primary?: ReactNode;
  equals?: ReactNode;
  context?: ReactNode;
  onReview?: () => void;
  onOpenEmail?: (id: string) => void;
}>) {
  return (
    <div className="worklist-row-acts">
      {context}
      {item.batch && onReview ? (
        <BatchVerb onReview={onReview} />
      ) : (
        <RowVerbs
          item={item}
          href={href}
          part="ways"
          framed={framed}
          move={moveHref(item)}
          onOpenEmail={onOpenEmail}
        />
      )}
      {/* The reader's own override, among the words for the reason the queue's
          own line gives: a lone glyph opening or closing a line reads as a
          stray mark rather than as a verb. */}
      {allowPin && <PinVerb item={item} />}
      {/* Handing the task on is not a verb the card may drop: without it the
          one row a reader is answering is the one they cannot pass along. */}
      {item.source === "task" && !item.batch && (
        <ReassignControl item={item} owner={owner} />
      )}
      {/* The menu beside the two glyphs above, so the three square controls
          stand together, and Done after it, next to the move. */}
      <DispositionVerbs item={item} />
      {equals}
      {!item.batch && (
        <RowVerbs
          item={item}
          href={href}
          part="act"
          framed={framed}
          move={moveHref(item)}
          onOpenEmail={onOpenEmail}
        />
      )}
      {primary}
    </div>
  );
}
