// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { screen, within } from "@testing-library/react";

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
