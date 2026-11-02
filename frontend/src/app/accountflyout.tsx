// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { ChevronRight } from "lucide-react";
import { type ReactNode, useCallback, useId, useRef, useState } from "react";
import type { RowSeat } from "./accountseats";
import { usePopoverDismiss } from "./popover";

/**
 * A layer that flies out of one row of the account menu. Its dismissal listener
 * registers after the menu's, since it only opens inside an open menu. On
 * Escape the menu's listener sees the expanded row and stands down (app/popover.ts).
 */
export function useFlyout(seat: number, move: (at: number) => void) {
  const [open, setOpen] = useState(false);
  const panel = useRef<HTMLDivElement>(null);
  const id = useId();

  // Focus goes back to the row only when the flyout held it. An outside click
  // has already put focus where the reader wanted it.
  const close = useCallback(() => {
    const held = panel.current?.contains(document.activeElement) ?? false;
    setOpen(false);
    if (held) {
      move(seat);
    }
  }, [seat, move]);

  usePopoverDismiss(open, panel, close);

  return { open, setOpen, panel, id, close };
}

export type Flyout = ReturnType<typeof useFlyout>;

/**
 * The row that opens a flyout, and the seat the flyout is positioned against.
 * Click or Enter toggles it and Right opens it.
 */
export function FlyoutRow({
  label,
  seat,
  flyout,
  children,
}: Readonly<{
  label: string;
  seat: RowSeat;
  flyout: Flyout;
  children: ReactNode;
}>) {
  return (
    // `role="none"`: a menu's children are its items, and a plain div between
    // the two would break that parentage.
    <div className="acctflyoutseat" role="none">
      <button
        type="button"
        className="acctrow"
        role="menuitem"
        // No `aria-haspopup`: the theme row opens a group of radios, which no
        // value of it names. Expanded and naming its region is what tells the
        // menu's own dismissal that a layer inside it owns Escape.
        aria-expanded={flyout.open}
        aria-controls={flyout.open ? flyout.id : undefined}
        {...seat}
        onClick={() => flyout.setOpen((was) => !was)}
        onKeyDown={(event) => {
          if (event.key !== "ArrowRight") {
            return;
          }
          flyout.setOpen(true);
          event.preventDefault();
          event.stopPropagation();
        }}
      >
        <span className="acctrowlabel">{label}</span>
        <ChevronRight size={14} className="acctmore" aria-hidden />
      </button>
      {flyout.open && children}
    </div>
  );
}
