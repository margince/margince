// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// A press that changes what the list shows can take its own control away, and
// focus would fall to <body>. It goes to the list's title, or the list itself.
export function focusListHome(from: Element): void {
  const list = from.closest<HTMLElement>(".lt");
  const home = list?.querySelector<HTMLElement>(".lt-title") ?? list;
  if (!home) {
    return;
  }
  if (!home.hasAttribute("tabindex")) {
    home.tabIndex = -1;
  }
  home.focus();
}
