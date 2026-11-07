// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { screen, within } from "@testing-library/react";
import userEvent, { type UserEvent } from "@testing-library/user-event";
import { catalogs, type Locale } from "../i18n";
import { labelled, viewMenuName } from "./listview.names";

// More Back presses than any panel has levels means the panel is stuck.
const DEEPEST_STEP = 4;

// A list's dials are all behind its View button. A test reaches a dial the
// way a reader does: View, then the dial's row, then the step it opens.

/** The test's one `userEvent.setup()` instance (frontend/AGENTS.md). */
type Presser = Pick<UserEvent, "click">;

// The module's own `click` starts a new input session on every call.
function oneSession(user: Presser): void {
  if (user === userEvent) {
    throw new Error("pass the test's userEvent.setup(), not userEvent");
  }
}

/** The View button, by its visible word; its count and dot follow a comma. */
export function viewButton(
  scope: HTMLElement = document.body,
  locale: Locale = "en",
): Promise<HTMLElement> {
  return within(scope).findByRole("button", { name: viewMenuName(locale) });
}

/** Opens the View panel, unless it is open already, and returns it. */
export async function openView(
  user: Presser,
  scope: HTMLElement = document.body,
  locale: Locale = "en",
): Promise<HTMLElement> {
  oneSession(user);
  const trigger = await viewButton(scope, locale);
  if (trigger.getAttribute("aria-expanded") !== "true") {
    await user.click(trigger);
  }
  return screen.findByRole("dialog", { name: viewMenuName(locale) });
}

/**
 * Presses Back until the panel shows its rows, or until `until` finds what it
 * wants, and returns what `until` found last.
 */
export async function walkBack<T>(
  panel: HTMLElement,
  user: Presser,
  until: () => T | null = () => null,
): Promise<T | null> {
  oneSession(user);
  for (let presses = 0; ; presses++) {
    const found = until();
    const back = panel.querySelector<HTMLElement>(".lt-mback");
    if (found !== null || back === null) {
      return found;
    }
    if (presses === DEEPEST_STEP) {
      throw new Error(
        `the View panel still had a Back after ${DEEPEST_STEP} presses`,
      );
    }
    await user.click(back);
  }
}

/**
 * Opens the View panel and the step of one dial ("Filters", "Sort",
 * "Columns", or a screen's own row), and returns that step.
 */
export async function openViewStep(
  user: Presser,
  step: string,
  scope: HTMLElement = document.body,
  locale: Locale = "en",
): Promise<HTMLElement> {
  const panel = await openView(user, scope, locale);
  const name = labelled(step);
  // A panel left on another step walks back to its rows first.
  // Back's name opens on its step's name too, so only a row will do.
  const row = await walkBack(
    panel,
    user,
    () =>
      within(panel)
        .queryAllByRole("button", { name })
        .find((button) => !button.classList.contains("lt-mback")) ?? null,
  );
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
  locale: Locale = "en",
): Promise<void> {
  const panel = await openView(user, scope, locale);
  await user.click(
    within(panel).getByRole("button", {
      name: catalogs[locale]["list.showArchived"],
    }),
  );
}
