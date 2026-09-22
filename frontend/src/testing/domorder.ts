// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/**
 * WHICH OF TWO ELEMENTS COMES FIRST, for a test whose subject is the order.
 *
 * A row of controls read left to right IS the specification on several
 * surfaces — the list toolbar narrows before it draws, and its Save view
 * stands last — so the assertion is about position rather than presence.
 * `compareDocumentPosition` answers it and reads as a bitmask at the call
 * site, which is why every suite that reached for it wrote a comment
 * explaining the mask instead of the claim.
 */
export function precedes(first: Element, second: Element): boolean {
  const where = first.compareDocumentPosition(second);
  return (where & Node.DOCUMENT_POSITION_FOLLOWING) !== 0;
}
