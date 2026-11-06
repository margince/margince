// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { screen, within } from "@testing-library/react";
import type { UserEvent } from "@testing-library/user-event";
import { pickOption } from "../design-system/select-testing";

export type ChoiceDialog = Readonly<{
  dialog: HTMLElement;
  chooser: HTMLElement;
  save: HTMLElement;
}>;

function escaped(field: string): string {
  return field.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

/**
 * Opens one choice's dialog the way a reader does, from its value ("Change
 * Industry …"), or from `trigger` where the value is named otherwise. The
 * dialog and its chooser are both named for the field.
 */
export async function openChoiceDialog(
  user: Pick<UserEvent, "click">,
  field: string | RegExp,
  trigger?: HTMLElement,
): Promise<ChoiceDialog> {
  const verb =
    typeof field === "string"
      ? new RegExp(`^Change ${escaped(field)}`)
      : new RegExp(`^Change ${field.source}`, field.flags);
  await user.click(
    trigger ?? (await screen.findByRole("button", { name: verb })),
  );
  const dialog = await screen.findByRole("dialog", { name: field });
  return {
    dialog,
    // A chooser fed by a read (a roster) arrives after its dialog.
    chooser: await within(dialog).findByRole("combobox", { name: field }),
    save: within(dialog).getByRole("button", { name: "Save" }),
  };
}

/** Changes one choice: open its dialog, pick the draft, and Save it. */
export async function changeChoice(
  user: UserEvent,
  field: string | RegExp,
  option: string | RegExp,
): Promise<void> {
  const { dialog, chooser } = await openChoiceDialog(user, field);
  await pickOption(user, chooser, option);
  await user.click(within(dialog).getByRole("button", { name: "Save" }));
}
