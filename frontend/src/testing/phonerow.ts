// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

export type PhoneRow = Readonly<{
  row: HTMLTableRowElement;
  identity: HTMLElement;
  end: readonly string[];
  caption: readonly string[];
}>;

/** The words a reader sees in `node`: a screen reader's own label is left out. */
function seenText(node: Element): string {
  const seen = node.cloneNode(true);
  if (!(seen instanceof Element)) {
    return "";
  }
  for (const label of seen.querySelectorAll(".sr-only")) {
    label.remove();
  }
  return (seen.textContent ?? "").trim();
}

/**
 * The list row that holds `name`, read as a phone draws it: line one's end item
 * and line two's facts, each as the words on screen. A fact with none is left out.
 */
export function phoneRow(name: Element): PhoneRow {
  const row = name.closest("tr");
  const identity = row?.querySelector<HTMLElement>("td.lt-identity");
  if (!row || !identity) {
    throw new Error(`no list row holds ${name.textContent}`);
  }
  const text = (selector: string) =>
    [...row.querySelectorAll(selector)]
      .map(seenText)
      .filter((words) => words !== "");
  return {
    row,
    identity,
    end: text('td[data-phone="end"]'),
    caption: text('td[data-phone="caption"] .lt-fact'),
  };
}
