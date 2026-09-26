// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useEffect, useLayoutEffect, useRef } from "react";

// What a dialog owes the keyboard, in one place: Escape closes it, Tab stays
// inside it, and focus goes in when it opens and back where it came from when
// it closes.
//
// `Modal` held all three privately, which was fine while `Modal` was the only
// dialog. It was not: the ⌘K palette draws its own box — its own overlay, its
// own list geometry, its own row chrome — and so it grew its own behaviour to
// go with it, which came out as Escape wired to the search input alone and no
// trap at all. Shift+Tab from that input moved focus to a link on the page
// behind on the first press, and Escape pressed while focus sat on a result row
// did nothing.
//
// So the behaviour is a hook and the chrome stays each surface's own. That is
// the split that lets the palette keep looking like the palette without being a
// second answer to "what does a dialog do when you press Tab" — and it is the
// reason this is not a `className` on `Modal`, which would have invited every
// screen to restyle the one dialog instead.

/**
 * The tab stops inside a container, in document order.
 *
 * Disabled controls and anything explicitly removed from the tab order are not
 * stops; a negative tabindex (a dialog's own) is reachable by script but not by
 * Tab, so it is deliberately excluded here.
 */
const FOCUSABLE =
  'a[href], button, input, select, textarea, summary, [tabindex]:not([tabindex="-1"])';

export function focusableWithin(root: HTMLElement): HTMLElement[] {
  return Array.from(root.querySelectorAll<HTMLElement>(FOCUSABLE)).filter(
    (element) => !element.hasAttribute("disabled") && element.tabIndex !== -1,
  );
}

/**
 * The dialog's own way OUT, which `Modal` marks so this file can tell it from
 * the controls the dialog is FOR.
 */
const WAY_OUT = "[data-dialog-close]";

/**
 * Where focus lands when a dialog opens: the first stop that is not the door.
 *
 * The door is a tab STOP — the last one — but it is never where a reader is put
 * down, and the difference is not a nicety. A dialog whose body is prose or an
 * empty list has exactly one control, the door, and landing on it made the
 * close control name itself in a tip; the tip then answered the reader's first
 * Escape and the dialog stayed open. One press doing nothing, in front of a
 * surface covering the page, is a keyboard trap however briefly it lasts.
 *
 * Nothing to land on is an honest answer: the box itself takes focus, which is
 * what its `tabIndex={-1}` is for, and the reader's first Tab reaches the door
 * from there.
 */
function firstControlIn(dialog: HTMLElement | null): HTMLElement | null {
  if (dialog === null) {
    return null;
  }
  return (
    focusableWithin(dialog).find((stop) => stop.closest(WAY_OUT) === null) ??
    null
  );
}

// Keep Tab inside the dialog. `aria-modal` tells a screen reader the rest of
// the page is inert; it does nothing for the Tab key, so without this a
// keyboard reader walks straight out of the dialog into the page behind it and
// can operate a surface the dialog is covering.
function keepTabInside(event: KeyboardEvent, dialog: HTMLElement) {
  const stops = focusableWithin(dialog);
  if (stops.length === 0) {
    event.preventDefault();
    return;
  }
  const first = stops[0];
  const last = stops[stops.length - 1];
  const active = document.activeElement;
  // Focus already outside the dialog is the case both directions have to
  // catch, not just Shift+Tab: it happens whenever something on the page
  // behind took focus while the dialog was open, and from there a plain Tab
  // would keep walking that page rather than coming back.
  const outside = !dialog.contains(active);
  const leavingBackwards = event.shiftKey && (active === first || outside);
  const leavingForwards = !event.shiftKey && (active === last || outside);
  if (!leavingBackwards && !leavingForwards) {
    return;
  }
  event.preventDefault();
  (leavingBackwards ? last : first).focus();
}

// A popover opened from INSIDE a dialog, if one is holding tab stops.
//
// It sits above the dialog (popover.css) and is portalled to the body, so it is
// not a descendant of the container the trap holds. While one is up it owns
// Tab: a trap that pulled focus back into the dialog would make the panel's own
// controls unreachable by keyboard, which is the whole panel.
//
// Scoped to triggers inside this dialog rather than found document-wide: a
// document-wide search would just as happily return one opened from the page
// BEHIND the dialog — handing Tab to a layer the reader cannot see. The trigger
// names its own panel through `aria-controls`, and the trigger IS in the
// dialog, which is the only link between the two that survives the portal.
//
// Two can be open at once, so DOM order is not the answer. A popover shuts on
// a mousedown outside itself, but a hover-opened receipt is opened by a
// settling pointer and fires no such press — it can rise beside a panel a
// click already opened. The one holding focus is the one the reader is in;
// the other is a panel they are merely near.
//
// A panel with no tab stops is not one: a `StatCard` receipt is frequently
// prose, and a trap holding a container it cannot move focus within answers
// every Tab by swallowing it. Falling back to the dialog leaves the panel on
// screen and the reader still able to walk what is behind it.
function openPanelIn(
  dialog: HTMLElement | null,
  needsTabStops = true,
): HTMLElement | null {
  const panels = [
    ...(dialog?.querySelectorAll<HTMLElement>(
      '[aria-expanded="true"][aria-controls]',
    ) ?? []),
  ]
    .map((trigger) =>
      document.getElementById(trigger.getAttribute("aria-controls") ?? ""),
    )
    .filter((panel): panel is HTMLElement => panel !== null);
  const active = document.activeElement;
  const held = panels.find((panel) => panel.contains(active));
  const panel = held ?? panels[0];
  return panel && (!needsTabStops || focusableWithin(panel).length > 0)
    ? panel
    : null;
}

/** The modal dialogs that are up, in document order. */
export function liveDialogs(): HTMLElement[] {
  // A leaving dialog is still painted, inside the `inert` its surface marks the
  // exit with; it can take no key, so the layer beneath it is already on top.
  return [
    ...document.querySelectorAll<HTMLElement>(
      '[role="dialog"][aria-modal="true"]',
    ),
  ].filter((layer) => layer.closest("[inert]") === null);
}

/**
 * Whether a live dialog the element is not inside sits on top. A layer under
 * one leaves Escape to it; a layer opened from inside the dialog keeps its own.
 */
export function coveredByDialog(element: Element | null): boolean {
  const layers = liveDialogs();
  const top = layers[layers.length - 1];
  return top !== undefined && !top.contains(element);
}

function ownsKeyboard(container: HTMLElement | null): boolean {
  const layers = liveDialogs();
  const top = layers[layers.length - 1];
  return !top || top === container || openPanelIn(top) === container;
}

/**
 * Escape, the Tab trap, and focus in-and-back, for one open dialog.
 *
 * `container` is the element the trap holds and the element focus lands inside
 * — the dialog's own box, not its overlay. `returnFocusTo` names where focus
 * should go instead of the opener, for a dialog whose OWN mutation removes the
 * control that opened it.
 */
export function useDialogFocus({
  open,
  onClose,
  container,
  returnFocusTo,
  initialFocusTo,
}: Readonly<{
  open: boolean;
  onClose: () => void;
  container: React.RefObject<HTMLElement | null>;
  returnFocusTo?: () => HTMLElement | null;
  initialFocusTo?: () => HTMLElement | null;
}>) {
  // Held in a ref so the restore below calls the CURRENT resolver. The focus
  // effect is keyed on `open` alone — re-running it whenever an inline callback
  // changes identity would drag focus back to the dialog's first stop on every
  // render of the page behind it — so the closure it captures is otherwise the
  // one from the render that opened the dialog.
  const returnFocus = useRef(returnFocusTo);
  const initialFocus = useRef(initialFocusTo);
  useEffect(() => {
    returnFocus.current = returnFocusTo;
    initialFocus.current = initialFocusTo;
  }, [returnFocusTo, initialFocusTo]);

  // A LAYOUT effect, because a passive one is scheduled after the browser
  // paints: between the commit that puts this dialog on screen and a passive
  // effect attaching the listener, the dialog is visible, hit-testable, and
  // deaf to Escape. A key pressed in that window is not queued, it is lost —
  // and for a dialog whose dismissal is the safe answer, losing it strands the
  // reader in front of a question that no longer answers the one key everyone
  // reaches for. React runs a layout effect during the commit, before yielding
  // to the browser, so there is no frame in which this dialog can be seen and
  // not respond.
  useLayoutEffect(() => {
    if (!open) {
      return;
    }
    const onKey = (event: KeyboardEvent) => {
      // A reader above a composer owns the keyboard until it closes. Marking
      // Escape handled also prevents a lower listener closing after that unmount.
      if (
        !container.current ||
        event.defaultPrevented ||
        !ownsKeyboard(container.current)
      )
        return;
      if (event.key === "Escape" && !openPanelIn(container.current, false)) {
        event.preventDefault();
        onClose();
        return;
      }
      if (event.key === "Tab") {
        keepTabInside(
          event,
          openPanelIn(container.current) ?? container.current,
        );
      }
    };
    globalThis.addEventListener("keydown", onKey);
    return () => globalThis.removeEventListener("keydown", onKey);
  }, [open, onClose, container]);

  // Focus moves in when the dialog opens and returns to whatever opened it
  // when it closes — otherwise a keyboard reader who dismisses a dialog
  // resumes tabbing from the top of the document, having lost their place.
  useEffect(() => {
    if (!open) {
      return;
    }
    const opener = document.activeElement;
    (
      initialFocus.current?.() ??
      firstControlIn(container.current) ??
      container.current
    )?.focus();
    return () => {
      // A named target outranks the opener even while the opener is still
      // attached: a caller names one precisely because the mutation this dialog
      // just performed unmakes that control, and the unmaking usually lands
      // with the refetch a moment AFTER this runs. Handing focus back to a
      // button that is about to be removed drops the reader on <body> a tick
      // later, which is the failure this whole escape hatch exists to prevent.
      const named = returnFocus.current?.() ?? null;
      if (named?.isConnected) {
        named.focus();
        return;
      }
      // focus() on a node the mutation already detached is a silent no-op and
      // leaves focus on <body>, from where the next Tab restarts at the top of
      // the document. Asking first is what keeps that case from looking like a
      // restore that worked.
      if (opener instanceof HTMLElement && opener.isConnected) {
        opener.focus();
      }
    };
  }, [open, container]);
}
