// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { screen, within } from "@testing-library/react";
import userEvent, { type UserEvent } from "@testing-library/user-event";

// The ask for a model's draft, by the name its verb carries on a reply and on
// a new message.
const ASK = /^Draft (reply|email)$/;

/**
 * The composer's button that asks a model for a draft.
 *
 * Synchronous, so a `waitFor` can read the button's state through it as it
 * reads any other query.
 */
export function draftButton(container: HTMLElement = document.body) {
  return within(container).getByRole("button", { name: ASK });
}

/** The composer window, where the warnings before sending sit. */
export function composerWindow() {
  return screen.getAllByRole("dialog")[0];
}

/** The purpose a draft is written to, on a reply and on a new message. */
export function purposeField() {
  return screen.getByRole("textbox", {
    name: /^Purpose of the (reply|email)$/,
  });
}

/**
 * Asks for a new draft once one stands. Redraft shows only while the purpose
 * differs from the one the words were drafted to, so this adds to it first.
 */
export async function redraft(
  user: UserEvent = userEvent.setup(),
  addition = " again",
) {
  await user.type(purposeField(), addition);
  await user.click(screen.getByRole("button", { name: "Redraft" }));
}
