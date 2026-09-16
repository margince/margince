// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// FOCUS, for a panel drawn at the body and read as though it sat by its
// trigger.
//
// Two components portal a panel: the popover and the evidence mark. The portal
// is what lets them work inside a clipping card — a panel opened near the edge
// of `overflow: hidden` loses exactly the part it was opened for — and it is
// also what breaks the one thing a keyboard reader relies on, which is that Tab
// goes where the eye goes.
//
// Both were fixing that by hand, in two copies, and both copies had the same
// two holes. A pointer leaving the trigger closed the panel while a control
// inside it still held focus, so focus fell to `<body>` and the next Tab
// started the page again from the top. And Tab out of the panel walked into
// whatever the body happened to render next — a toast region, the next
// portal — rather than the control that follows the trigger on the page.
//
// So the rule lives once, here: for focus, the panel behaves as though it were
// the trigger's next sibling. It is emphatically NOT a focus trap — the page
// behind stays usable, and a reader who tabs past the panel leaves it.

import {
  type KeyboardEvent as ReactKeyboardEvent,
  type RefObject,
  useCallback,
  useEffect,
  useRef,
} from "react";

// What a reader can land on. A CSS selector rather than a rule, but it is read
// by all three behaviours below and a panel whose first stop and last stop were
// found by two different selectors would be a panel that lets go of focus at
// one end and not the other.
const FOCUSABLE =
  'a[href], button, input, select, textarea, [tabindex]:not([tabindex="-1"])';

/**
 * usePortalPanelFocus gives a portalled panel the focus behaviour it would
 * have had as a DOM sibling of its trigger.
 *
 * `openedBy` is the distinction focus turns on. A press is a reader asking for
 * the panel and focus follows them into it; a pointer settling is not, and
 * taking focus off whatever they were doing would be the page grabbing at
 * them — so a hover-opened panel is left alone.
 *
 * Spread the returned props on the panel element.
 */
export function usePortalPanelFocus({
  open,
  openedBy,
  trigger,
  panel,
}: Readonly<{
  open: boolean;
  openedBy: "press" | "hover";
  trigger: RefObject<HTMLElement | null>;
  panel: RefObject<HTMLElement | null>;
}>): {
  onFocus: () => void;
  onKeyDown: (event: ReactKeyboardEvent<HTMLElement>) => void;
} {
  // Whether focus has been INSIDE the panel during this opening. Recorded
  // while it is open, because by the time the close runs the panel is gone and
  // there is nothing left to ask.
  const held = useRef(false);

  // Focus moves into the panel when a press opens it, if there is anything in
  // it to land on. A panel of prose has no stops at all and takes no focus, so
  // the reader stays on the trigger they pressed — a panel of CONTROLS a
  // keyboard reader can see and cannot reach is what this prevents.
  useEffect(() => {
    if (!open || openedBy === "hover") {
      return;
    }
    panel.current?.querySelector<HTMLElement>(FOCUSABLE)?.focus();
  }, [open, openedBy, panel]);

  // The panel closed. If it took focus down with it, hand it back.
  //
  // ONLY where the unmount dropped it on the floor. A control inside the panel
  // may close it and send the reader somewhere on purpose — the receipt's "Full
  // history" opens a drawer and focuses it — and a return that fired there
  // would pull them straight back out of what they just opened. Focus resting
  // on `<body>` is the tell that nobody claimed it.
  useEffect(() => {
    if (open) {
      return;
    }
    if (!held.current) {
      return;
    }
    held.current = false;
    const landed = trigger.current?.ownerDocument.activeElement;
    if (landed === null || landed === trigger.current?.ownerDocument.body) {
      trigger.current?.focus();
    }
  }, [open, trigger]);

  const onFocus = useCallback(() => {
    held.current = true;
  }, []);

  // TAB, RELATIVE TO THE TRIGGER rather than to the body.
  //
  // Forward off the last stop goes to whatever follows the trigger on the page;
  // backward off the first stop goes to the trigger itself. Every stop between
  // them is the browser's own, untouched — this is not a trap, and a reader who
  // keeps tabbing leaves the panel exactly as they would leave any aside.
  const onKeyDown = useCallback(
    (event: ReactKeyboardEvent<HTMLElement>) => {
      if (event.key !== "Tab" || insideADialog(trigger.current)) {
        return;
      }
      const stops = stopsIn(panel.current);
      const active = panel.current?.ownerDocument.activeElement ?? null;
      if (event.shiftKey) {
        if (active !== stops[0]) {
          return;
        }
        event.preventDefault();
        trigger.current?.focus();
        return;
      }
      if (active !== stops[stops.length - 1]) {
        return;
      }
      event.preventDefault();
      afterTheTrigger(trigger.current, panel.current)?.focus();
    },
    [panel, trigger],
  );

  return { onFocus, onKeyDown };
}

// INSIDE A DIALOG THIS RULE STANDS DOWN.
//
// A dialog owns the Tab key while it is open (dialogfocus.ts), and it already
// knows about this panel: it finds it through the trigger's own `aria-controls`
// and holds Tab inside it, because a reader must not be able to walk out of a
// dialog into the page it is covering. "Where the trigger sits on the page" is
// the wrong question there — the page behind is exactly where Tab may not go.
function insideADialog(trigger: HTMLElement | null): boolean {
  return trigger?.closest('[role="dialog"][aria-modal="true"]') != null;
}

function stopsIn(panel: HTMLElement | null): HTMLElement[] {
  return panel ? [...panel.querySelectorAll<HTMLElement>(FOCUSABLE)] : [];
}

// The next stop after the trigger IN THE PAGE, which is where Tab would have
// gone had the panel never existed.
//
// The panel's own stops are excluded: it sits at the body, usually after
// everything else, so a walk that counted them would send a reader from the end
// of the panel back into its own beginning.
function afterTheTrigger(
  trigger: HTMLElement | null,
  panel: HTMLElement | null,
): HTMLElement | null {
  if (!trigger) {
    return null;
  }
  const page = [
    ...trigger.ownerDocument.body.querySelectorAll<HTMLElement>(FOCUSABLE),
  ].filter((stop) => !panel?.contains(stop));
  const here = page.indexOf(trigger);
  // A trigger the walk cannot find is one that is itself unfocusable — a
  // `disabled` attribute arriving with the panel still up. Nothing follows it
  // that this can name, so the browser's own answer stands.
  if (here < 0) {
    return null;
  }
  return page[here + 1] ?? null;
}
