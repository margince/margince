// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { screen, within } from "@testing-library/react";
import type { UserEvent } from "@testing-library/user-event";
import { en } from "../i18n/en";

// A list's dials are all behind its View button, so a test reaches a dial the
// way a reader does: View, then the dial's row, then the step it opens.

/** A `userEvent.setup()` instance, or `userEvent` itself. */
type Presser = Pick<UserEvent, "click">;

function escaped(text: string): string {
  return text.replaceAll(/[.*+?^${}()|[\]\\]/g, String.raw`\$&`);
}

/** The View button, by its visible word; its count and dot follow a comma. */
export function viewButton(
  scope: HTMLElement = document.body,
): Promise<HTMLElement> {
  return within(scope).findByRole("button", {
    name: new RegExp(`^${escaped(en["list.viewMenu"])}(,|$)`),
  });
}

/** Opens the View panel, unless it is open already, and returns it. */
export async function openView(
  user: Presser,
  scope: HTMLElement = document.body,
): Promise<HTMLElement> {
  const trigger = await viewButton(scope);
  if (trigger.getAttribute("aria-expanded") !== "true") {
    await user.click(trigger);
  }
  return screen.findByRole("dialog", {
    name: new RegExp(`^${escaped(en["list.viewMenu"])}(,|$)`),
  });
}

/**
 * Opens the View panel and the step of one dial ("Filters", "Sort",
 * "Columns", "Density", or a screen's own row), and returns that step.
 */
export async function openViewStep(
  user: Presser,
  step: string,
  scope: HTMLElement = document.body,
): Promise<HTMLElement> {
  const panel = await openView(user, scope);
  const name = new RegExp(`^${escaped(step)}(,|$)`);
  // A panel left on another step walks back to its rows first.
  let row = within(panel).queryByRole("button", { name });
  let back = panel.querySelector<HTMLElement>(".lt-mback");
  while (row === null && back !== null) {
    await user.click(back);
    row = within(panel).queryByRole("button", { name });
    back = panel.querySelector<HTMLElement>(".lt-mback");
  }
  if (row === null) {
    throw new Error(`the View panel has no ${step} row`);
  }
  await user.click(row);
  return within(panel).findByRole("group", { name: step });
}

/** Presses the list's Show archived row in the View panel, on or off. */
export async function toggleArchived(
  user: Presser,
  scope: HTMLElement = document.body,
): Promise<void> {
  const panel = await openView(user, scope);
  await user.click(
    within(panel).getByRole("button", { name: en["list.showArchived"] }),
  );
}
