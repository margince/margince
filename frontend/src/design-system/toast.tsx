// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { X } from "lucide-react";
import {
  createContext,
  type ReactNode,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
} from "react";
import { createPortal } from "react-dom";
import { useT } from "../i18n";
import "./toast.css";

/**
 * The transient confirmation — "that worked" — and the one place it is spelled.
 *
 * It was a HOOK before this, so every screen minted its own state and rendered
 * its own region, and all three things that go wrong when a global surface is
 * held locally went wrong. `screens/commissiondecide.tsx` called the hook and
 * rendered no region, so every approve/pay/void confirmation was written into a
 * `useState` nobody read. `screens/deals.tsx` had to carry a comment explaining
 * that the instance belongs to the CALLER, because a hook minting its own shows
 * its messages to nobody. And the region being a SIBLING of the page's cards
 * forced two layout rules to be written around a fixed box that takes no space
 * — one in `enter.css`, one in the `.wrap:has(> .lt)` rule in `app/shell.css`.
 *
 * So it is a provider with ONE region, portalled to the body like every other
 * overlay in this directory, and a screen only ever asks for `show`.
 */

/**
 * The verb a confirmation may carry — Undo, mostly.
 *
 * `label` arrives translated, like all copy in this tier. The toast withdraws
 * itself once `onAct` has run: a message still offering an action it has already
 * taken is a second press waiting to happen.
 *
 * A toast carrying one of these does NOT withdraw on a timer. A reader reaching
 * for Undo must not lose it mid-reach, and there is no timeout long enough to be
 * safe that is also short enough to still be a toast.
 */
export type ToastAction = Readonly<{
  label: string;
  onAct: () => void;
}>;

/** How long a confirmation stays before it withdraws itself. */
const TOAST_MS = 3500;

/**
 * `mark` is the green dot that reads as "done", and it belongs to the MESSAGE
 * rather than to the region: the same region shows a save that worked and a save
 * that was refused, and the copies this replaces put a completion tick beside
 * both. A failure with a green dot beside it is worse than a failure with no
 * glyph — it says the opposite of what the sentence says.
 */
type ToastMessage = Readonly<{
  node: ReactNode;
  mark: boolean;
  /** Whether it withdraws itself, which decides whether it needs a way out. */
  sticky: boolean;
  action: ToastAction | null;
}>;

export type ToastOptions = Readonly<{
  /**
   * Keep it until something dismisses it. Implied by `action`, and worth asking
   * for on its own only where the message is a REFUSAL: a reader who has been
   * told a write did not land should not have that sentence taken away from
   * them three and a half seconds later.
   */
  sticky?: boolean;
  /** False for anything that is not a completion — a refusal, a warning. */
  mark?: boolean;
  /** The verb the message carries. Makes it sticky, and gives it a way out. */
  action?: ToastAction;
}>;

export type Toast = Readonly<{
  show: (message: ReactNode, options?: ToastOptions) => void;
  dismiss: () => void;
}>;

/**
 * Two contexts rather than one, and the split is load-bearing.
 *
 * The controls never change, so a screen that only shows toasts never re-renders
 * because one appeared somewhere else. The QUEUE changes on every message, and
 * the only thing subscribed to it is the region. One context carrying both would
 * re-render every screen in the tree each time a confirmation arrived.
 */
const ToastControlsContext = createContext<Toast | null>(null);
const ToastQueueContext = createContext<readonly ToastMessage[]>([]);

/**
 * The default controls are a no-op, and deliberately not a throw.
 *
 * A screen rendered in isolation — a story, a unit test, the login path before
 * the shell mounts — has no region listening, and the reasoning
 * `app/attention.tsx` gives applies unchanged: publishing into nothing is right
 * there. What made the old design unsafe was that PRODUCTION could be in that
 * state too. It cannot now — `design-system/conformance.test.ts` holds the
 * provider and the region to one file each, the way `UnsavedGuard` is held to
 * `App.tsx`.
 */
const NO_REGION: Toast = { show: () => {}, dismiss: () => {} };

/**
 * The queue, and the one rule that shapes it.
 *
 * A confirmation carrying a verb is not interchangeable with one that only
 * reports: the second is a courtesy, the first is the reader's only route back
 * from something they may not have meant. So while a message with an action is
 * on screen, everything arriving QUEUES BEHIND IT rather than replacing it.
 * Otherwise the newest message wins, which is what a reader making two quick
 * saves expects to see.
 *
 * No cap. A cap here would drop a message silently, and what it would be
 * protecting against — several undoable writes queued behind one another, none
 * of them dismissed — is a reader working faster than they can read rather than
 * a runaway.
 */
export function ToastProvider({ children }: Readonly<{ children: ReactNode }>) {
  const [queue, setQueue] = useState<readonly ToastMessage[]>([]);

  const dismiss = useCallback(() => {
    setQueue((waiting) => waiting.slice(1));
  }, []);

  const show = useCallback((message: ReactNode, options?: ToastOptions) => {
    const action = options?.action ?? null;
    const arriving: ToastMessage = {
      node: message,
      mark: options?.mark ?? true,
      sticky: options?.sticky ?? action !== null,
      action,
    };
    setQueue((waiting) => {
      const shown = waiting[0];
      // Nothing on screen, or what is on screen is only reporting: the newest
      // message is the one worth seeing, and the old one has said its piece.
      if (shown === undefined || shown.action === null) {
        return [arriving, ...waiting.slice(1)];
      }
      return [...waiting, arriving];
    });
  }, []);

  const controls = useMemo(() => ({ show, dismiss }), [show, dismiss]);
  return (
    <ToastControlsContext.Provider value={controls}>
      <ToastQueueContext.Provider value={queue}>
        {children}
      </ToastQueueContext.Provider>
    </ToastControlsContext.Provider>
  );
}

/** What a screen calls to say something landed. */
export function useToast(): Toast {
  return useContext(ToastControlsContext) ?? NO_REGION;
}

/**
 * Where a confirmation appears: fixed to the foot of the viewport, centred, and
 * portalled to the body.
 *
 * Portalled for the reason every other overlay here is. A region rendered in
 * place is a fixed box inside the content column, which makes it a sibling of
 * the page's cards that occupies no space — and any ancestor with a `transform`
 * becomes the viewport it anchors to. Two rules in this tree exist only to work
 * around that, and both go away with this.
 *
 * `<output>` is the element: it is a live region by default, so the confirmation
 * is announced without anything having to declare `role="status"` beside it. It
 * is polite rather than assertive on purpose — a confirmation that interrupts
 * whatever a screen reader was saying costs more than it gives.
 *
 * Focus is never taken. The reader is mid-task and the toast is passive; what it
 * owes them instead is a way IN (it is last in the DOM, so Tab reaches it) and a
 * way OUT (Escape, while focus is inside it).
 */
export function ToastRegion() {
  const t = useT();
  const queue = useContext(ToastQueueContext);
  const { dismiss } = useToast();
  const shown = queue[0] ?? null;
  // WHICH message the reader is holding, rather than a bare flag. WCAG 2.2.1
  // asks for a way to extend a time limit, and for a passive surface the honest
  // one is that reading it stops the clock — but a flag would carry from the
  // message that was hovered onto the one that replaced it, freezing a
  // confirmation the pointer was never near. Naming the message makes the reset
  // fall out of the comparison instead of needing an effect to undo it.
  const [heldMessage, setHeldMessage] = useState<ToastMessage | null>(null);
  const held = shown !== null && heldMessage === shown;
  // The node in STATE rather than in a ref, so the effect below can depend on
  // the thing it actually attaches to. A ref is invisible to the dependency
  // array: the region is mounted and unmounted as messages come and go, and an
  // effect that could not see that ran once against a node that did not exist
  // yet and never again.
  const [region, setRegion] = useState<HTMLDivElement | null>(null);

  // Escape belongs to the REGION, and it is attached to the node rather than
  // written as a JSX handler on a static element.
  //
  // It was on the two buttons, which was wrong for a reason a caller found
  // before a reader did: the MESSAGE can carry focusable content of its own —
  // the lead-qualified confirmation puts a link to the new contact in its body
  // — and a reader who tabbed to that link was inside a toast whose documented
  // way out did nothing. What "focus is inside it" means is the region, so the
  // region is what listens.
  useEffect(() => {
    if (region === null) {
      return;
    }
    const putDown = (event: KeyboardEvent) => {
      if (event.key !== "Escape") {
        return;
      }
      // The toast is not a dialog, but it is the innermost thing holding focus,
      // and a screen behind it may listen for the same key.
      event.stopPropagation();
      dismiss();
    };
    region.addEventListener("keydown", putDown);
    return () => region.removeEventListener("keydown", putDown);
  }, [dismiss, region]);

  useEffect(() => {
    if (shown === null || shown.sticky || held) {
      return;
    }
    const timer = setTimeout(dismiss, TOAST_MS);
    // The cleanup one of the three hand-copied toasts was missing. A timer
    // belongs to the tree that started it: left running, it fires a state update
    // into a component that is no longer mounted.
    return () => clearTimeout(timer);
    // `shown` identity changes per message, which is what re-arms the deadline —
    // so a second confirmation gets its own full life rather than inheriting
    // what was left of the first one's.
  }, [shown, held, dismiss]);

  if (shown === null) {
    return null;
  }
  const act = shown.action;
  return createPortal(
    <div
      ref={setRegion}
      className="toast-region"
      onPointerEnter={() => setHeldMessage(shown)}
      onPointerLeave={() => setHeldMessage(null)}
      onFocusCapture={() => setHeldMessage(shown)}
      onBlurCapture={() => setHeldMessage(null)}
    >
      {/* `.arrive` (enter.css): it rises into place from below, which is the
          direction it comes from — the region is anchored to the bottom edge. */}
      <output className="toast arrive">
        {shown.mark && <span className="dot dot-auto" />}
        <span className="toast-said">{shown.node}</span>
        {act !== null && (
          <button
            type="button"
            className="toast-action"
            onClick={() => {
              act.onAct();
              dismiss();
            }}
          >
            {act.label}
          </button>
        )}
        {shown.sticky && (
          <button
            type="button"
            className="toast-dismiss"
            aria-label={t("common.close")}
            onClick={dismiss}
          >
            <X size={14} aria-hidden />
          </button>
        )}
      </output>
    </div>,
    document.body,
  );
}
