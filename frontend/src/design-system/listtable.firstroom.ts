// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

type GrowsFirst = Readonly<{ key: string; min: number; first: number }>;

/**
 * The room past every other column's minimum, split evenly among the columns
 * that grow first. A column its minimum or `first` holds leaves the rest to the
 * others, minimums settling before caps as in `columnLayout`.
 */
export function splitFirstRoom(
  columns: readonly GrowsFirst[],
  room: number,
): Map<string, number> {
  const settled = new Map<string, number>();
  let left = room;
  let pending = columns;
  while (pending.length > 0) {
    const even = left / pending.length;
    const short = pending.filter((column) => column.min > even);
    const bound: [GrowsFirst, number][] =
      short.length > 0
        ? short.map((column) => [column, column.min])
        : pending
            .filter((column) => column.first < even)
            .map((column) => [column, column.first]);
    if (bound.length === 0) {
      for (const column of pending) {
        settled.set(column.key, even);
      }
      break;
    }
    for (const [column, width] of bound) {
      settled.set(column.key, width);
      left -= width;
    }
    pending = pending.filter((column) => !settled.has(column.key));
  }
  return settled;
}
