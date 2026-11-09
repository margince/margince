// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useState } from "react";

// An ended row can do nothing more, so it waits behind a toggle. One ended from
// this view stays, struck, so focus has its row to land on.
export function useEndedRows<Row extends { id: string }>(
  rows: readonly Row[],
  isEnded: (row: Row) => boolean,
) {
  const [showing, setShowing] = useState(false);
  const [endedHere, setEndedHere] = useState<ReadonlySet<string>>(new Set());
  const hidden = rows.filter((row) => isEnded(row) && !endedHere.has(row.id));
  const shown = showing ? rows : rows.filter((row) => !hidden.includes(row));
  return {
    shown,
    hidden,
    showing,
    toggle: () => setShowing((on) => !on),
    keep: (id: string) => setEndedHere((ids) => new Set(ids).add(id)),
  };
}
