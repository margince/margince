// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { expect, within } from "storybook/test";
import type { ListChip } from "../design-system/listsurface.dials";

// Two applied filters too long for a phone's line, in German.
export const LONG_FILTER_CHIPS: readonly ListChip[] = [
  {
    key: "size_band",
    label: "Unternehmensgröße",
    allLabel: "Jede Unternehmensgröße",
    options: [{ value: "1001-5000", label: "1001–5000 Mitarbeitende" }],
  },
  {
    key: "industry",
    label: "Branche",
    allLabel: "Jede Branche",
    options: [
      {
        value: "legal_professional_services",
        label: "Recht und professionelle Dienstleistungen",
      },
    ],
  },
];

export const LONG_FILTER_VALUES: Readonly<Record<string, string>> = {
  size_band: "1001-5000",
  industry: "legal_professional_services",
};

async function appliedRows(canvasElement: HTMLElement) {
  const rows = await within(canvasElement).findAllByRole("group", {
    name: /^(Unternehmensgröße|Branche): /,
  });
  await expect(rows).toHaveLength(LONG_FILTER_CHIPS.length);
  return rows;
}

function part(row: HTMLElement, selector: string): HTMLElement {
  const found = row.querySelector(selector);
  if (!(found instanceof HTMLElement)) {
    throw new Error(`the applied filter drew no ${selector}`);
  }
  return found;
}

/** Each row's "⋮" stays inside the toolbar, and no value is cut. */
export async function expectAppliedFiltersFit(canvasElement: HTMLElement) {
  for (const row of await appliedRows(canvasElement)) {
    const tools = row.closest(".lt-tools");
    if (!(tools instanceof HTMLElement)) {
      throw new Error("the applied filter is not in a toolbar");
    }
    const edge = tools.getBoundingClientRect();
    const menu = part(row, ".lt-frow-more").getBoundingClientRect();
    await expect(menu.left).toBeGreaterThanOrEqual(edge.left);
    await expect(menu.right).toBeLessThanOrEqual(edge.right);
    const value = part(row, ".lt-frow-valuetext");
    await expect(value.scrollWidth).toBeLessThanOrEqual(value.clientWidth);
  }
}

/** Each row stays one line, at its name segment's floor. */
export async function expectAppliedFiltersOneLine(canvasElement: HTMLElement) {
  for (const row of await appliedRows(canvasElement)) {
    const name = part(row, ".lt-frow-attr");
    const floor = Number.parseFloat(getComputedStyle(name).minBlockSize);
    await expect(name.getBoundingClientRect().height).toBe(floor);
  }
}

/** The `uat-wide` frame's viewport, for a story's `parameters`. */
export const WIDE = {
  viewport: {
    options: {
      wide: { name: "Wide", styles: { width: "1440px", height: "900px" } },
    },
  },
};
