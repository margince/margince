// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { fireEvent, screen, within } from "@testing-library/react";

// The ask for a model's draft, by the name its verb carries on a reply and on
// a new message.
const ASK = /^Draft (reply|email)$/;

/**
 * The composer's button that asks a model for a draft.
 *
 * It sits in a band the editor's own "Draft with AI" opens, so the band is
 * opened first when it is shut. Synchronous, so a `waitFor` can read the
 * button's state through it as it reads any other query.
 */
export function draftButton(container: HTMLElement = document.body) {
  const scope = within(container);
  const asked = scope.queryByRole("button", { name: ASK });
  if (asked) {
    return asked;
  }
  fireEvent.click(scope.getByRole("button", { name: "Draft with AI" }));
  return scope.getByRole("button", { name: ASK });
}

/** The composer's checks before sending, opened from the foot. */
export function openChecks() {
  const open = screen.queryByRole("dialog", { name: /check|Ready|Before/ });
  if (open) {
    return open;
  }
  fireEvent.click(
    screen.getByRole("button", { name: /check|Ready to send|Before sending/ }),
  );
  return screen.getByRole("dialog", { name: /check|Ready|Before/ });
}
