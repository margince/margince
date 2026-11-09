// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useState } from "react";

// An ended row can do nothing more, so it waits behind a toggle. One ended from
// this view (`keep`, once the end succeeded) stays, struck, so focus has its row
// to land on, until the reader hides the ended rows.
export function useEndedRows<Row extends { id: string }>(
  rows: readonly Row[],
  isEnded: (row: Row) => boolean,
) {
  const [showAll, setShowAll] = useState(false);
  const [endedHere, setEndedHere] = useState<ReadonlySet<string>>(new Set());
  const ended = rows.filter(isEnded);
  const hidden = showAll ? [] : ended.filter((row) => !endedHere.has(row.id));
  const hiddenIds = new Set(hidden.map((row) => row.id));
  const showing = ended.length > hidden.length;
  return {
    shown: rows.filter((row) => !hiddenIds.has(row.id)),
    ended,
    hidden,
    showing,
    toggle: () => {
      setShowAll(!showing);
      setEndedHere(new Set());
    },
    keep: (id: string) => setEndedHere((ids) => new Set(ids).add(id)),
  };
}
