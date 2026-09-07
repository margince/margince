// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// A short answer, on the spot, on click.
//
// Not a Disclosure. A disclosure adds its content to the page, which is right
// when the content is the next thing to read and wrong when it is an aside: on
// a row of readings the reading a person opened grew and the three beside it
// jumped down the page, so checking what one figure rests on moved the other
// three out from under the eye that was comparing them. A popover leaves the
// page where it was.
//
// Not a Tooltip either. A tooltip appears on hover, says one line, and cannot
// be reached by touch or by a keyboard reader who wants to stay in it. What
// goes in here is a paragraph or a small list — evidence, worth reading at the
// reader's pace, and sometimes carrying a link.
//
// The panel is portalled to the body and positioned against the trigger, for
// the reason the overflow menu is (atoms.tsx): a card clips its own overflow
// so a full-bleed row keeps the card's radius, and a panel opened inside one
// near the card's edge loses exactly the part it was opened for.

import { type ReactNode, useEffect, useId, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { useAnchoredToTrigger } from "./anchored";
import { Button, type ButtonVariant } from "./atoms";
import { useHoverIntent } from "./hoverintent";
import "./popover.css";

// The first thing in a panel a reader can land on. The same set the dialog
// trap uses (atoms.tsx), kept in the two places that need it rather than
// exported from one — this is a CSS selector, not a shared rule about focus.
const FOCUSABLE =
  'a[href], button, input, select, textarea, [tabindex]:not([tabindex="-1"])';

/**
 * Popover is one trigger and the aside behind it.
 *
 * The trigger is a button carrying `label`, styled by `className` so a caller
 * can seat it in the line it belongs to. The panel is labelled BY that button,
 * so a screen reader announces what was opened rather than reading a floating
 * paragraph with no title.
 */
export function Popover({
  label,
  className,
  variant,
  onHover,
  children,
}: Readonly<{
  label: ReactNode;
  className?: string;
  // Also opens when the pointer SETTLES on the trigger, and closes when it
  // leaves. For an aside a reader takes in on the way past — the receipt under
  // a reading — where a click is a step they should not have to take. Off by
  // default: a panel that carries controls has to be reachable and stay put,
  // and one that opens under a passing cursor is neither.
  //
  // The click stays either way. Hover is not an affordance a touch screen or a
  // keyboard has, and a control that only answered a pointer would answer
  // nobody else.
  onHover?: boolean;
  // Draws the trigger as a full control rather than as text in the line it
  // sits in — the caret half of a split button, a toolbar verb. Absent, the
  // trigger inherits the type around it and is marked pressable by its own
  // caret and its hover, which is what a receipt under a reading wants.
  variant?: ButtonVariant;
  children: ReactNode;
}>) {
  const [open, setOpen] = useState(false);
  // How the panel came to be open. A press is a reader asking for it, and
  // focus follows; a passing pointer is not, and focus stays where it was.
  const [openedBy, setOpenedBy] = useState<"press" | "hover">("press");
  const trigger = useRef<HTMLButtonElement | null>(null);
  const panel = useRef<HTMLElement | null>(null);
  const panelId = useId();
  const triggerId = useId();
  // From the trigger's leading edge: a popover is opened by words as often
  // as by a chip, and a panel hung off the end of a full-width sentence lands
  // at the margin with nothing under it.
  const at = useAnchoredToTrigger(open, trigger, panel, "start");

  // Focus moves into the panel when it opens, IF there is anything in it to
  // focus. A panel of controls a keyboard reader could see and not reach is
  // the failure this prevents; a panel of prose has no stops at all and takes
  // no focus, so the reader stays on the trigger they pressed.
  useEffect(() => {
    if (!open || openedBy === "hover") {
      return;
    }
    // Never on a hover-opened panel: the pointer is somewhere else on the page
    // and taking focus off what the reader was doing to put it in a panel they
    // merely passed over is the page grabbing at them. A panel that ALSO opens
    // on hover still hands focus over when a key or a click opened it: the
    // control in it has to be reachable by the reader who asked for it.
    panel.current?.querySelector<HTMLElement>(FOCUSABLE)?.focus();
  }, [open, openedBy]);

  const hover = useHoverIntent(
    () => {
      setOpenedBy("hover");
      setOpen(true);
    },
    () => setOpen(false),
  );
  const press = () => {
    setOpenedBy("press");
    setOpen((was) => !was);
  };

  useEffect(() => {
    if (!open) {
      return;
    }
    const onKey = (event: KeyboardEvent) => {
      if (event.key !== "Escape") {
        return;
      }
      setOpen(false);
      // Back to the button that opened it. Escape with the focus left in a
      // panel that has just been removed drops a keyboard reader at the top of
      // the document, several sections above the reading they were on.
      trigger.current?.focus();
    };
    const onPointer = (event: MouseEvent) => {
      if (!(event.target instanceof Node)) {
        return;
      }
      // The panel lives at the body rather than inside the trigger's box, so
      // "outside" means outside BOTH — a click on a link inside the panel is
      // otherwise a click away from it.
      if (
        !trigger.current?.contains(event.target) &&
        !panel.current?.contains(event.target)
      ) {
        setOpen(false);
      }
    };
    globalThis.addEventListener("keydown", onKey);
    globalThis.addEventListener("mousedown", onPointer);
    return () => {
      globalThis.removeEventListener("keydown", onKey);
      globalThis.removeEventListener("mousedown", onPointer);
    };
  }, [open]);

  return (
    <>
      {variant ? (
        <Button
          id={triggerId}
          ref={trigger}
          variant={variant}
          className={
            className ? `popover-trigger ${className}` : "popover-trigger"
          }
          aria-expanded={open}
          aria-controls={panelId}
          onClick={press}
          {...(onHover ? hover : {})}
        >
          {label}
        </Button>
      ) : (
        <button
          type="button"
          id={triggerId}
          ref={trigger}
          className={
            className ? `popover-trigger ${className}` : "popover-trigger"
          }
          aria-expanded={open}
          aria-controls={panelId}
          onClick={press}
          {...(onHover ? hover : {})}
        >
          {label}
        </button>
      )}
      {/* Rendered only while it is open. Nothing in here holds state a reader
          expects to find again — it is a paragraph of evidence — so unlike the
          overflow menu, which keeps items alive because they own dialogs,
          there is nothing to preserve across a close. */}
      {open &&
        createPortal(
          // A named section, not a dialog: nothing here traps focus or has to
          // be dismissed before the page can be used again, and announcing a
          // dialog we do not implement would tell a screen reader the page has
          // gone modal when it has not.
          <section
            id={panelId}
            ref={panel}
            className="popover-panel"
            aria-labelledby={triggerId}
            {...(onHover ? hover : {})}
            style={{
              top: `${at.top}px`,
              left: `${at.left}px`,
              maxHeight: `${at.maxHeight}px`,
            }}
          >
            {children}
          </section>,
          document.body,
        )}
    </>
  );
}
