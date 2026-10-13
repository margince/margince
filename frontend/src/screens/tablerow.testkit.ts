// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { screen, within } from "@testing-library/react";
import type { UserEvent } from "@testing-library/user-event";
import { translate } from "../i18n";

/** One row of a named table, found by text in it, as cells keyed by heading. */
export async function tableRowCells(
  table: string,
  text: string,
): Promise<Record<string, HTMLElement>> {
  const grid = await screen.findByRole("table", { name: table });
  const headers = within(grid)
    .getAllByRole("columnheader")
    .map((cell) => cell.textContent ?? "");
  const row = within(grid)
    .getAllByRole("row")
    .find((candidate) => candidate.textContent?.includes(text));
  if (!row) throw new Error(`no row with ${text} in ${table}`);
  const cells = within(row).getAllByRole("cell");
  return Object.fromEntries(headers.map((header, i) => [header, cells[i]]));
}

/** The menu one row's "…" opened, named by the row so no other row's can pass. */
export async function openRowMenu(user: UserEvent, name: string) {
  const trigger = await screen.findByRole("button", {
    name: translate("en", "table.rowActions", { name }),
  });
  await user.click(trigger);
  const items = document.getElementById(
    trigger.getAttribute("aria-controls") ?? "",
  );
  if (!items) throw new Error(`the menu for ${name} drew no items`);
  return within(items);
}
