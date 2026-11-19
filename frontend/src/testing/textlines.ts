// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/**
 * The tops of the lines `node`'s text is laid out on. A browser returns one
 * rect per text run, so a line is a distinct top, never a rect.
 */
export function lineTops(node: Node): Set<number> {
  const range = (node.ownerDocument ?? document).createRange();
  range.selectNodeContents(node);
  return new Set([...range.getClientRects()].map((box) => Math.round(box.top)));
}
