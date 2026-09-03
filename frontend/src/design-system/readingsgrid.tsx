// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
import "./readingsgrid.css";

// A record's readings as free-standing CARDS in a row, not as one plate.
//
// `StatStrip` is the other shape and the difference is what the row claims. A
// strip is read ACROSS as one comparison — six slots of one type scale, ruled
// apart — which is right for six unlike facts a reader compares. Four readings
// that are one verdict and the dimensions it was computed from are not unlike:
// each is a door into the tab that holds its detail, each carries its own
// receipt, and a reader uses them one at a time. Cards are how a reader uses
// them.
//
// It came out of the company page, where it was a screen class wrapped around
// `StatCard`s; the contact, lead and deal pages each drew their own strip, and
// three records reading the same kind of card three ways is how two of them
// come to look like a different product. The grid owns only the row: how the
// cards divide the width and when they fold. Nothing here decides how a card
// draws.
export function ReadingsGrid({
  label,
  testId,
  children,
}: Readonly<{
  // What the row IS, for a reader navigating by landmark — "where this deal
  // stands". The copy is the caller's, translated.
  label: string;
  testId?: string;
  children: ReactNode;
}>) {
  return (
    <section className="readings" aria-label={label}>
      <div className="readings-grid" data-testid={testId}>
        {children}
      </div>
    </section>
  );
}
