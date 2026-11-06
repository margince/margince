// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { expect, screen, userEvent, waitFor, within } from "storybook/test";
import type { Locale } from "../i18n";
import { labelled, viewMenuName } from "./listview.names";

// A story's play reaches a list's dials as a reader does, through View. The
// panel fades in from nothing, so each step waits until it can be seen.

/** Presses View on the story's canvas and returns the open panel. */
export async function playOpenView(
  canvasElement: HTMLElement,
  locale: Locale = "en",
): Promise<HTMLElement> {
  await userEvent.click(
    await within(canvasElement).findByRole("button", {
      name: viewMenuName(locale),
    }),
  );
  const panel = await screen.findByRole("dialog", {
    name: viewMenuName(locale),
  });
  await waitFor(() => expect(panel).toBeVisible());
  return panel;
}

/** Opens View, presses one dial's row, and returns the step it opens. */
export async function playOpenViewStep(
  canvasElement: HTMLElement,
  step: string,
  locale: Locale = "en",
): Promise<HTMLElement> {
  const panel = await playOpenView(canvasElement, locale);
  await userEvent.click(
    within(panel).getByRole("button", { name: labelled(step) }),
  );
  const opened = await within(panel).findByRole("group", { name: step });
  await waitFor(() => expect(opened).toBeVisible());
  return opened;
}
