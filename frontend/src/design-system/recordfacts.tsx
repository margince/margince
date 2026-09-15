// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
import { Eyebrow } from "./eyebrow";

// The head's facts strip: caption over value, cells laid along the row and
// wrapping when the head narrows. Every record's head has one (the contact
// page wrote it first, as `.pe-facts`), and the company and lead pages are
// growing the same strip under their own names, so it moves here rather than
// staying a contact-only spelling a second page would have to copy.

/**
 * The strip itself: a `dl`, because each cell IS a term and its value.
 * Children are the `Fact` cells; a page's own facts (which to show, in what
 * order, whether one is even known) are the caller's, not this component's.
 */
export function RecordFacts({ children }: Readonly<{ children: ReactNode }>) {
  return <dl className="record-facts">{children}</dl>;
}

/**
 * One cell: a label over a value. The label is `Eyebrow`, so the strip reads
 * with the same micro-type every other record head uses for a caption.
 */
export function Fact({
  label,
  children,
}: Readonly<{ label: string; children: ReactNode }>) {
  return (
    <div className="record-fact">
      <dt>
        <Eyebrow>{label}</Eyebrow>
      </dt>
      <dd>{children}</dd>
    </div>
  );
}
