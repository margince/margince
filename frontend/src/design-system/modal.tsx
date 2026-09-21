// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { X } from "lucide-react";
import { type ReactNode, useRef } from "react";
import { createPortal } from "react-dom";
import { useT } from "../i18n";
import { useDialogFocus } from "./dialogfocus";
import { IconAction } from "./iconaction";
import { usePresence } from "./presence";
import "./atoms.css";

// The one dialog, and its own file because it is no longer one component: a
// portal, a focus contract, a presence contract, an exit choreography and the
// way out drawn over its corner. It sat in atoms.tsx while it was a box with
// children in it, and a reader looking for any of the five had to walk two
// thousand lines of unrelated primitives to find out which of them owned it.
//
// `atoms` re-exports it, so nothing that imports it had to change.

export function Modal({
  open,
  onClose,
  labelledBy,
  size = "default",
  placement = "center",
  returnFocusTo,
  initialFocusTo,
  closeReason,
  children,
}: Readonly<{
  open: boolean;
  onClose: () => void;
  labelledBy: string;
  // "wide" roomier variant for content-dense dialogs (code/YAML previews);
  // "default" keeps the compact form width every confirm/create modal uses.
  // "split" is a drawer holding TWO columns rather than one — the conversation
  // being answered beside the reply being written. It is a width because that
  // is what a second column costs; a drawer at the wide clamp splits into two
  // unreadable halves.
  size?: "default" | "wide" | "split";
  // "right" anchors the dialog to the right edge, full height — the drawer
  // form the composer and the evidence receipt use, where the record behind
  // stays visible as context rather than being covered by a centred box.
  // With size="wide" it takes the roomier clamp and a sticky header/footer,
  // for the surfaces a rep works IN rather than glances at.
  //
  // "full" is the lightbox: the box takes the screen it is on, inset far
  // enough that the darkened page still frames it, for content READ rather
  // than answered — a contract, a scan. It is a placement rather than a
  // `size` because what it decides is where the dialog sits, and it decides
  // that completely: `size` names widths for a centred box and there is
  // nothing left for one to vary here.
  placement?: "center" | "right" | "full";
  // Resolve at close time when a mutation replaces the opener (for example,
  // Deactivate becoming Reactivate). A callback can find the newly mounted control.
  returnFocusTo?: () => HTMLElement | null;
  /** The writing field can take focus before supporting context controls. */
  initialFocusTo?: () => HTMLElement | null;
  /**
   * Why the dialog cannot be left YET — an unfinished field edit, a choice the
   * reader has started and not landed.
   *
   * It rides the close control's own `reason`, which refuses the press, prints
   * the sentence beside it and points `aria-describedby` at it. A dialog whose
   * door is simply dead is a dead end, and a `title` on a disabled button
   * reaches nobody. Escape and the backdrop are unaffected: this refuses the
   * one control that would otherwise look available, not the reader's way out
   * of a surface covering their page.
   */
  closeReason?: string;
  children: ReactNode;
}>) {
  const t = useT();
  const dialog = useRef<HTMLDivElement | null>(null);
  const overlay = useRef<HTMLDivElement | null>(null);
  // The exit animation needs the dialog to still be on the page to play on, and
  // React would have unmounted it on the render that closed it. `state` is what
  // the stylesheet selects the exit with; `mounted` outlives `open` by exactly
  // as long as that exit takes, and by nothing at all where nothing animates.
  const { mounted, state } = usePresence({ open, element: overlay });
  // The palette shares keyboard behavior without sharing modal chrome. Keyed on
  // `open`, not on `mounted`: focus goes back to the opener the moment the
  // reader dismisses the dialog, rather than waiting out an animation they have
  // already finished with.
  useDialogFocus({
    open,
    onClose,
    container: dialog,
    returnFocusTo,
    initialFocusTo,
  });

  if (!mounted) {
    return null;
  }
  const leaving = state === "closing";
  // Portalled to the document body rather than rendered in place: a dialog
  // opened from inside a collapsed container — the record header's overflow
  // menu — would otherwise be hidden along with it, and the click that opened
  // the dialog is the same click that collapses the menu.
  return createPortal(
    // The two a11y suppressions this element used to carry are gone rather than
    // kept: `aria-hidden` below takes the overlay out of the accessibility tree
    // while it is leaving, and the rules that wanted a keyboard handler beside
    // the backdrop click no longer fire on it. Escape is still the keyboard
    // path, and it is `useDialogFocus`'s, not this element's.
    <div // NOSONAR: backdrop dismiss only; keyboard path (Esc) handled by the effect above
      className={placement === "right" ? "overlay overlay-right" : "overlay"}
      data-state={state}
      // A dialog on its way out is a picture of a dialog. `inert` takes it out
      // of the tab order and out of hit testing, so the fifth of a second it is
      // still painted cannot swallow the click meant for the page it is
      // uncovering.
      inert={leaving}
      // And `aria-hidden` takes it out of the accessibility TREE, which `inert`
      // does not: an inert node keeps its role, so a dialog mid-exit is still a
      // dialog to a screen reader — and to anything else reading roles. Two
      // dialogs answered "the dialog" for the length of one exit, which is a
      // reader being told about a surface that is leaving and, where a verb
      // opens the next dialog straight from the last, an assistive technology
      // announcing the wrong one. Safe only because `inert` is here too: an
      // `aria-hidden` subtree must hold nothing focusable.
      aria-hidden={leaving || undefined}
      ref={overlay}
      onClick={(event) => {
        if (leaving) {
          return;
        }
        if (event.target === event.currentTarget) {
          onClose();
        }
      }}
    >
      <div
        // NOSONAR: styled modal overlay driven by React state, not a native <dialog>; conversion would change focus/backdrop behavior
        role="dialog"
        aria-modal="true"
        aria-labelledby={labelledBy}
        className={modalClass(size, placement)}
        ref={dialog}
        // Focusable so a dialog whose body is pure text still receives focus
        // when it opens, rather than leaving it on the page behind.
        tabIndex={-1}
      >
        {children}
        {/* LAST in the DOM, and the position is the whole point: the first Tab
            into a dialog lands on its content, and the way out is the final
            stop rather than the one a reader has to pass to reach the form.
            Drawn at the corner over whatever the dialog put there, which is why
            the heads below reserve an end padding for it.

            `data-dialog-close` is how `useDialogFocus` tells the door from the
            controls a dialog is FOR: it is a tab stop, and never the one a
            reader is put down on when the dialog opens. */}
        <div className="modal-close" data-dialog-close>
          <IconAction
            label={t("common.close")}
            icon={<X aria-hidden="true" />}
            reason={closeReason}
            onClick={onClose}
          />
        </div>
      </div>
    </div>,
    document.body,
  );
}

// A right-anchored dialog draws its width from the viewport, so the `size`
// variants — which exist to widen a centred box — do not apply to it.
function modalClass(
  size: "default" | "wide" | "split",
  placement: "center" | "right" | "full",
) {
  // The lightbox answers before either branch below, because neither has
  // anything to say about it: it is as wide as the screen allows, so no width
  // varies it, and it is centred, so no edge anchors it.
  if (placement === "full") {
    return "modal modal-full";
  }
  if (placement === "right") {
    // A drawer's width normally comes from the viewport, but a surface a rep
    // WORKS in — a numbered claim list, a message being written — wraps into an
    // unreadable column at the default clamp. `size` is what asks for the
    // roomier one, and it brings sticky header and footer with it.
    //
    // A split drawer is the wide one plus the room its second column needs, so
    // it keeps the wide band behaviour rather than restating it.
    if (size === "split") {
      return "modal modal-drawer modal-drawer-wide modal-drawer-split";
    }
    return size === "wide"
      ? "modal modal-drawer modal-drawer-wide"
      : "modal modal-drawer";
  }
  // Centred, a split has no second column to hold — the layout that earns the
  // extra width is the drawer's — so it falls back to the roomy box rather
  // than to a width nothing on screen uses.
  return size === "default" ? "modal" : "modal modal-wide";
}
