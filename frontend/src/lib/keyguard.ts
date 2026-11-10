// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { liveDialogs } from "../design-system/dialogfocus";
import { isImeCommit } from "./imecommit";

// An input is text entry unless it is one of the types a key only presses.
const PRESSED_INPUTS = [
  "checkbox",
  "radio",
  "button",
  "submit",
  "reset",
  "range",
  "color",
  "file",
  "image",
];
const TEXT_ENTRY = `input${PRESSED_INPUTS.map((type) => `:not([type="${type}"])`).join("")}, textarea, select`;

export function isTextEntry(target: EventTarget | null): boolean {
  // An SVG element can hold focus and has no `isContentEditable`.
  return (
    target instanceof HTMLElement &&
    (target.isContentEditable || target.matches(TEXT_ENTRY))
  );
}

const OPEN_POPUP_TRIGGER =
  '[aria-haspopup]:not([aria-haspopup="false"])[aria-expanded="true"]';
const OPEN_TRIGGER = '[aria-expanded="true"][aria-controls]';
const LISTING_LAYER = '[role="menu"], [role="listbox"]';

/** A popup is portalled to the body or a dialog's overlay; a disclosure's
 *  region stays beside its toggle and is no layer. */
function opensLayer(trigger: Element): boolean {
  const panel = document.getElementById(
    trigger.getAttribute("aria-controls") ?? "",
  );
  const parent = panel?.parentElement;
  return (
    parent !== null &&
    parent !== undefined &&
    (parent === document.body || parent.classList.contains("overlay"))
  );
}

function layerIsOpen(target: EventTarget | null): boolean {
  return (
    liveDialogs().length > 0 ||
    (target instanceof Element && target.closest(LISTING_LAYER) !== null) ||
    document.querySelector(OPEN_POPUP_TRIGGER) !== null ||
    [...document.querySelectorAll(OPEN_TRIGGER)].some(opensLayer)
  );
}

/** Ctrl+Alt is AltGr on Windows, so Alt refuses too. Shift is left to the
 *  caller: `?` needs it. */
export function characterKeyRefused(event: KeyboardEvent): boolean {
  return (
    event.defaultPrevented ||
    event.repeat ||
    isImeCommit({ nativeEvent: event }) ||
    event.metaKey ||
    event.ctrlKey ||
    event.altKey ||
    isTextEntry(event.target) ||
    layerIsOpen(event.target)
  );
}
