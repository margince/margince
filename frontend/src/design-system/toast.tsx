// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { X } from "lucide-react";
import {
  createContext,
  type ReactNode,
  useCallback,
  useContext,
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
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
 */
export type ToastAction = Readonly<{
  /**
   * `undo` takes back the write the message reports: it lives `ACTION_TOAST_MS`
   * and a newer undo replaces it. `open` leads somewhere and stays until dismissed.
   */
  kind: "undo" | "open";
  label: string;
  onAct: () => void;
}>;

/** The undo a confirmation carries, for a caller whose take-back is one call. */
export function undoAction(label: string, onAct: () => void): ToastAction {
  return { kind: "undo", label, onAct };
}

/** Names one `show`, so a caller can withdraw its own message and no other. */
export type ToastId = number;

/** How long a confirmation stays before it withdraws itself. */
const TOAST_MS = 3500;

/** How long a confirmation carrying a verb stays: long enough to reach for it. */
const ACTION_TOAST_MS = 8000;

/**
 * What the message SAYS about itself, in the five-state vocabulary, as a VALUE
 * the type is read from so the story and the region walk one list.
 *
 * It replaces a `mark` boolean that could only turn the dot off. The dot was
 * green whatever the sentence said, so a refusal had to drop it entirely — and
 * the message a reader most needs to notice ended up as the one with no mark at
 * all. A refusal now carries a RED dot, which is the same signal doing the
 * opposite work rather than an absence standing in for it.
 */
export const TOAST_TONES = [
  "info",
  "success",
  "warning",
  "danger",
  "discovery",
] as const;

export type ToastTone = (typeof TOAST_TONES)[number];

type ToastMessage = Readonly<{
  /** Per `show`, so a message replacing one with the same text re-arrives. */
  id: ToastId;
  node: ReactNode;
  tone: ToastTone;
  /** Kept until something dismisses it. */
  sticky: boolean;
  action: ToastAction | null;
  /** It took the place of the message on screen, rather than waiting its turn. */
  replacedShown: boolean;
  /** Where focus sat when it was shown, for its action to hand focus back to. */
  returnFocusTo: HTMLElement | null;
}>;

export type ToastOptions = Readonly<{
  /**
   * Keep it until something dismisses it. Worth asking for only where the
   * message is a REFUSAL: a reader who has been told a write did not land
   * should not have that sentence taken away from them seconds later.
   */
  sticky?: boolean;
  /**
   * What the message is. `success` by default, because the confirmation that a
   * write landed is what this region is mostly for; anything that is not a
   * completion says so — `danger` for a refusal, `warning` for a caveat, `info`
   * for a report, `discovery` for something the reader has just been given.
   */
  tone?: ToastTone;
  /** The verb the message carries. Lengthens its life, and gives it a way out. */
  action?: ToastAction;
}>;

export type Toast = Readonly<{
  show: (message: ReactNode, options?: ToastOptions) => ToastId;
  /** Withdraws the message `id` names, or the one on screen when none is given. */
  dismiss: (id?: ToastId) => void;
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
const NO_REGION: Toast = { show: () => 0, dismiss: () => {} };

/**
 * The queue, and the one rule that shapes it.
 *
 * A confirmation carrying a verb is not interchangeable with one that only
 * reports: an undo is the reader's only route back from something they may not
 * have meant. So while a message with an action is on screen, anything else
 * arriving QUEUES BEHIND IT. The one exception is a newer undo, which takes the
 * older undo's place: a reader pressing Done on three tasks wants the latest
 * Undo, not three identical toasts to close. Otherwise the newest message wins.
 */
export function ToastProvider({ children }: Readonly<{ children: ReactNode }>) {
  const [queue, setQueue] = useState<readonly ToastMessage[]>([]);
  const nextId = useRef(0);

  const dismiss = useCallback((id?: ToastId) => {
    setQueue((waiting) =>
      id === undefined
        ? waiting.slice(1)
        : waiting.filter((message) => message.id !== id),
    );
  }, []);

  const show = useCallback((message: ReactNode, options?: ToastOptions) => {
    const action = options?.action ?? null;
    nextId.current += 1;
    const arriving: ToastMessage = {
      id: nextId.current,
      node: message,
      tone: options?.tone ?? "success",
      sticky: options?.sticky ?? action?.kind === "open",
      action,
      replacedShown: false,
      returnFocusTo: focusOutsideToasts(),
    };
    setQueue((waiting) => enqueue(waiting, arriving));
    return arriving.id;
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

function enqueue(
  waiting: readonly ToastMessage[],
  arriving: ToastMessage,
): readonly ToastMessage[] {
  const isUndo = (message: ToastMessage) => message.action?.kind === "undo";
  const replacing = { ...arriving, replacedShown: waiting.length > 0 };
  if (isUndo(arriving) && waiting.some(isUndo)) {
    return waiting.map((message, at) =>
      isUndo(message) ? (at === 0 ? replacing : arriving) : message,
    );
  }
  const shown = waiting[0];
  if (shown === undefined || shown.action === null) {
    return [replacing, ...waiting.slice(1)];
  }
  return [...waiting, arriving];
}

function focusOutsideToasts(): HTMLElement | null {
  const active = document.activeElement;
  return active instanceof HTMLElement &&
    active !== document.body &&
    active.closest(".toast-region") === null
    ? active
    : null;
}

/** Where focus sat in a message: one of its two controls, or its own body. */
type ToastControl = "act" | "close" | "body";

function focusedControl(output: HTMLElement): ToastControl | null {
  const active = document.activeElement;
  if (!(active instanceof HTMLElement) || !output.contains(active)) {
    return null;
  }
  const control = active.dataset.toastControl;
  return control === "act" || control === "close" ? control : "body";
}

/** The control matching `control` in the region, else its first focusable. */
function refocusTarget(region: HTMLElement, control: ToastControl) {
  const same =
    control === "body"
      ? null
      : region.querySelector<HTMLElement>(`[data-toast-control="${control}"]`);
  return same ?? region.querySelector<HTMLElement>("a[href], button");
}

/** What a screen calls to say something landed. */
export function useToast(): Toast {
  return useContext(ToastControlsContext) ?? NO_REGION;
}

/**
 * One caller's own message: `withdraw` takes back the last one it showed, and
 * never a message somebody else put on screen since. `leavesWithCaller` also
 * withdraws it on unmount, for a message whose verb acts on the caller's state.
 */
export function useOwnToast({ leavesWithCaller = false } = {}) {
  const toast = useToast();
  const own = useRef<ToastId | null>(null);
  const slot = useMemo(
    () => ({
      show: (message: ReactNode, options?: ToastOptions) => {
        own.current = toast.show(message, options);
      },
      withdraw: () => {
        if (own.current !== null) {
          toast.dismiss(own.current);
          own.current = null;
        }
      },
    }),
    [toast],
  );
  useEffect(
    () => (leavesWithCaller ? slot.withdraw : undefined),
    [leavesWithCaller, slot],
  );
  return slot;
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
 * way OUT (Escape, while focus is inside it). Pressing its action hands focus
 * back to where it sat when the message was shown, while that is still on screen.
 */
export function ToastRegion() {
  const t = useT();
  const queue = useContext(ToastQueueContext);
  const { dismiss } = useToast();
  const shown = queue[0] ?? null;
  const shownId = shown?.id ?? null;
  const replacedShown = shown?.replacedShown ?? false;
  // WCAG 2.2.1 asks for a way to extend a time limit, and for a passive surface
  // the honest one is that reading it stops the clock. The hold belongs to the
  // REGION, so a message replacing another under a resting pointer is held too.
  const [pointerInside, setPointerInside] = useState(false);
  const [focusInside, setFocusInside] = useState(false);
  const held = pointerInside || focusInside;
  // The node in STATE rather than in a ref, so the effect below can depend on
  // the thing it actually attaches to. A ref is invisible to the dependency
  // array: the region is mounted and unmounted as messages come and go, and an
  // effect that could not see that ran once against a node that did not exist
  // yet and never again.
  const [region, setRegion] = useState<HTMLDivElement | null>(null);
  // An unmounted region hears no pointerleave or blur, so leaving resets both.
  const attachRegion = useCallback((node: HTMLDivElement | null) => {
    setRegion(node);
    if (node === null) {
      setPointerInside(false);
      setFocusInside(false);
    }
  }, []);
  // Where focus sat in a message that is about to unmount; a replacement takes
  // it over. A ref cleanup runs before React removes the node, focus still in it.
  const refocus = useRef<ToastControl | null>(null);
  const watchOutput = useCallback((output: HTMLOutputElement | null) => {
    if (output === null) {
      return;
    }
    return () => {
      refocus.current = focusedControl(output);
    };
  }, []);

  useLayoutEffect(() => {
    const control = refocus.current;
    refocus.current = null;
    if (shownId === null || region === null) {
      return;
    }
    // Only a replacement: a message the reader put down hands focus to nobody.
    if (control !== null && replacedShown) {
      refocusTarget(region, control)?.focus();
    }
    setFocusInside(region.contains(document.activeElement));
  }, [shownId, replacedShown, region]);

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
    const life = shown.action === null ? TOAST_MS : ACTION_TOAST_MS;
    const timer = setTimeout(() => dismiss(shown.id), life);
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
      ref={attachRegion}
      className="toast-region"
      onPointerEnter={() => setPointerInside(true)}
      onPointerLeave={() => setPointerInside(false)}
      onFocusCapture={() => setFocusInside(true)}
      onBlurCapture={() => setFocusInside(false)}
    >
      {/* `.arrive` (enter.css): it rises into place from below, which is the
          direction it comes from — the region is anchored to the bottom edge. */}
      <output key={shown.id} ref={watchOutput} className="toast arrive">
        <span className={`dot toast-dot-${shown.tone}`} />
        <span className="toast-said">{shown.node}</span>
        {act !== null && (
          <button
            type="button"
            className="toast-action"
            data-toast-control="act"
            onClick={(event) => {
              const holding = event.currentTarget === document.activeElement;
              act.onAct();
              dismiss(shown.id);
              if (holding && shown.returnFocusTo?.isConnected) {
                shown.returnFocusTo.focus();
              }
            }}
          >
            {act.label}
          </button>
        )}
        {(shown.sticky || act !== null) && (
          <button
            type="button"
            className="toast-dismiss"
            data-toast-control="close"
            aria-label={t("common.close")}
            onClick={() => dismiss(shown.id)}
          >
            <X size={14} aria-hidden />
          </button>
        )}
      </output>
    </div>,
    document.body,
  );
}
