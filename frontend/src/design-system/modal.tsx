// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Maximize2, Minimize2, X } from "lucide-react";
import {
  type ReactNode,
  useCallback,
  useContext,
  useRef,
  useState,
} from "react";
import { createPortal } from "react-dom";
import { useT } from "../i18n";
import { UnsavedContext, useUnsavedRegistry } from "../lib/unsavedclaims";
import { useBackdropDismiss } from "./backdrop";
import { ConfirmModal } from "./confirmmodal";
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

// Each intent is one shape and one width token in tokens.css. Pick by what the
// reader does there; the README's Overlays table says which is which.
export const MODAL_INTENTS = [
  "confirm",
  "form",
  "drawer",
  "drawer-reading",
  "full",
] as const;
export type ModalIntent = (typeof MODAL_INTENTS)[number];

// A drawer covers its intent's width or, once the reader expands it, the
// window less the gutter it floats in.
export type DrawerSpan = "intent" | "window";
type DrawerIntent = "drawer" | "drawer-reading";

// Only a drawer can be expanded: a centred dialog asks about the page behind
// it, and the lightbox already takes the glass.
type Expansion =
  | Readonly<{
      intent: DrawerIntent;
      span?: DrawerSpan;
      /** Makes the drawer expandable; the caller keeps the span it reports. */
      onSpanChange?: (span: DrawerSpan) => void;
    }>
  | Readonly<{
      intent: Exclude<ModalIntent, DrawerIntent>;
      span?: never;
      onSpanChange?: never;
    }>;

// A form with more fields than this is worked through in a drawer.
export const FORM_DIALOG_MAX_FIELDS = 6;
export function intentForFieldCount(fields: number): "form" | "drawer" {
  return fields <= FORM_DIALOG_MAX_FIELDS ? "form" : "drawer";
}

export function Modal({
  open,
  onClose,
  labelledBy,
  describedBy,
  intent,
  span = "intent",
  onSpanChange,
  returnFocusTo,
  initialFocusTo,
  closeReason,
  closeDisabled,
  unsaved = false,
  children,
}: Readonly<{
  open: boolean;
  onClose: () => void;
  labelledBy: string;
  /** A line under the title that says what the dialog is about. */
  describedBy?: string;
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
  /** Holds the corner X while a write the dialog started is still out. */
  closeDisabled?: boolean;
  /**
   * The caller holds an unsaved draft of this dialog's form. A form inside the
   * dialog claims one with `useUnsavedGuard` instead.
   */
  unsaved?: boolean;
  children: ReactNode;
}> &
  Expansion) {
  const t = useT();
  const dialog = useRef<HTMLDivElement | null>(null);
  const overlay = useRef<HTMLDivElement | null>(null);
  // The exit animation needs the dialog to still be on the page to play on, and
  // React would have unmounted it on the render that closed it. `state` is what
  // the stylesheet selects the exit with; `mounted` outlives `open` by exactly
  // as long as that exit takes, and by nothing at all where nothing animates.
  const { mounted, state } = usePresence({ open, element: overlay });
  const drafts = useUnsavedRegistry(useContext(UnsavedContext));
  const [asking, setAsking] = useState(false);
  // A caller that closes the dialog itself leaves no question behind for the next open.
  if (!open && asking) {
    setAsking(false);
  }
  // The X, Escape and the backdrop all close through here, so each one asks
  // before it throws away a draft. A save closes through the caller instead.
  const dirty = unsaved || drafts.dirty;
  const requestClose = useCallback(() => {
    if (dirty) {
      setAsking(true);
    } else {
      onClose();
    }
  }, [dirty, onClose]);
  // The palette shares keyboard behavior without sharing modal chrome. Keyed on
  // `open`, not on `mounted`: focus goes back to the opener the moment the
  // reader dismisses the dialog, rather than waiting out an animation they have
  // already finished with.
  useDialogFocus({
    open,
    onClose: requestClose,
    container: dialog,
    returnFocusTo,
    initialFocusTo,
  });
  const leaving = state === "closing";
  const backdrop = useBackdropDismiss(requestClose, leaving);

  if (!mounted) {
    return null;
  }
  const box = modalClass(intent, span);
  const wide = span === "window";
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
      className={
        box?.split(" ").includes("modal-drawer")
          ? "overlay overlay-right"
          : "overlay"
      }
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
      {...backdrop}
    >
      <div
        // NOSONAR: styled modal overlay driven by React state, not a native <dialog>; conversion would change focus/backdrop behavior
        role="dialog"
        aria-modal="true"
        aria-labelledby={labelledBy}
        aria-describedby={describedBy}
        className={box}
        ref={dialog}
        // Focusable so a dialog whose body is pure text still receives focus
        // when it opens, rather than leaving it on the page behind.
        tabIndex={-1}
      >
        <UnsavedContext.Provider value={drafts}>
          {children}
        </UnsavedContext.Provider>
        {/* LAST in the DOM, and the position is the whole point: the first Tab
            into a dialog lands on its content, and the way out is the final
            stop rather than the one a reader has to pass to reach the form.
            Drawn at the corner over whatever the dialog put there, which is why
            the heads below reserve an end padding for it.

            `data-dialog-close` is how `useDialogFocus` tells the door from the
            controls a dialog is FOR: it is a tab stop, and never the one a
            reader is put down on when the dialog opens. Expand and Collapse
            share that corner and that rule: they change the window, not the
            work in it. */}
        <div className="modal-close" data-dialog-close>
          {onSpanChange && (
            <IconAction
              label={wide ? t("common.collapse") : t("common.expand")}
              icon={
                wide ? (
                  <Minimize2 aria-hidden="true" />
                ) : (
                  <Maximize2 aria-hidden="true" />
                )
              }
              onClick={() => onSpanChange(wide ? "intent" : "window")}
            />
          )}
          <IconAction
            label={t("common.close")}
            icon={<X aria-hidden="true" />}
            reason={closeReason}
            disabled={closeDisabled}
            onClick={requestClose}
          />
        </div>
      </div>
      {/* Portalled to the body on its own; it sits here only for React. */}
      <DiscardDraftQuestion
        open={open && asking}
        body={t("unsaved.dialogBody")}
        onKeep={() => setAsking(false)}
        onDiscard={() => {
          setAsking(false);
          onClose();
        }}
      />
    </div>,
    document.body,
  );
}

// Written as branches because dialoglayout.ts runs them to learn each box.
function modalClass(intent: ModalIntent, span: DrawerSpan) {
  if (intent === "confirm") return "modal modal-confirm";
  if (intent === "form") return "modal modal-form";
  if (intent === "drawer") {
    return span === "window"
      ? "modal modal-drawer modal-drawer-window"
      : "modal modal-drawer";
  }
  if (intent === "drawer-reading") {
    return span === "window"
      ? "modal modal-drawer modal-drawer-wide modal-drawer-window"
      : "modal modal-drawer modal-drawer-wide";
  }
  if (intent === "full") return "modal modal-full";
}

/** The question a dirty close asks, for a dialog and for a form open inside one. */
export function DiscardDraftQuestion({
  open,
  body,
  onKeep,
  onDiscard,
}: Readonly<{
  open: boolean;
  // What closing loses, in the caller's words; the title alone asks it.
  body?: string;
  onKeep: () => void;
  onDiscard: () => void;
}>) {
  const t = useT();
  return (
    <ConfirmModal
      open={open}
      title={t("unsaved.title")}
      confirmLabel={t("unsaved.discard")}
      confirmVariant="danger"
      // Closing the question keeps the draft, as `UnsavedGuard`'s does.
      onClose={onKeep}
      onConfirm={onDiscard}
    >
      {body && <p>{body}</p>}
    </ConfirmModal>
  );
}
