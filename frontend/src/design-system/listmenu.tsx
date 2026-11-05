import { ChevronLeft } from "lucide-react";
import { type ReactNode, useEffect, useRef } from "react";
import { coveredByDialog } from "./dialogfocus";

export function Menu({
  open,
  head,
  align = "left",
  children,
}: Readonly<{
  open: boolean;
  head: string;
  align?: "left" | "right";
  children: ReactNode;
}>) {
  return (
    // A fieldset because that is what this is: a set of controls that belong
    // together. Named with it, so both a screen reader and a test can say
    // which menu (and which step of it) they are in.
    <fieldset
      className={`lt-menu${open ? " open" : ""}${align === "right" ? " right" : ""}`}
      // A closed menu leaves the tab order entirely; otherwise Tab walks every
      // option of every filter before reaching the body below it.
      inert={!open}
      aria-label={head}
    >
      <MenuHead head={head} />
      {children}
    </fieldset>
  );
}

/** What a menu or a step of the View panel asks, or the way back from it. */
export function MenuHead({
  head,
  back,
}: Readonly<{
  head: string;
  /** The level above. The face stays the step's name; the label says "back" to
   * a reader who cannot see the arrow. */
  back?: { label: string; onBack: () => void };
}>) {
  if (!back) {
    return <div className="lt-mhead">{head}</div>;
  }
  return (
    <button type="button" className="lt-mhead lt-mback" onClick={back.onBack}>
      <ChevronLeft className="lt-caret" strokeWidth={1.8} aria-hidden="true" />
      {head}
      {/* Beside the visible name, never an `aria-label` over it: a reader
          speaking what they see says "Status" (WCAG 2.5.3). */}
      <span className="sr-only"> {back.label}</span>
    </button>
  );
}

/**
 * Escape closes the open popup and hands focus back to whatever opened it.
 *
 * Without it the only way out of a filter or column menu is a click elsewhere.
 * A reader who tabbed in would land at the top of the document when it
 * closed, not on the control they were standing on.
 *
 * Keyed on which popup is open rather than on whether one is. Moving straight
 * from one trigger to the next never passes through a closed state. A boolean
 * would hold the first trigger and send focus back to the wrong control.
 */
export function useCloseOnEscape(openKey: string | null, close: () => void) {
  const latest = useRef(close);
  latest.current = close;
  // Captured while the menu is open, so the trigger is still the element that
  // had focus before the reader stepped into the popup.
  const opener = useRef<Element | null>(null);
  useEffect(() => {
    if (openKey === null) {
      return;
    }
    opener.current = document.activeElement;
    const onKey = (event: KeyboardEvent) => {
      if (event.key !== "Escape" || coveredByDialog(opener.current)) {
        return;
      }
      latest.current();
      const back = opener.current;
      if (back instanceof HTMLElement && back.isConnected) {
        back.focus();
      }
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, [openKey]);
}

/**
 * Click anywhere that is not a menu or its trigger, and the menus close.
 *
 * The handler is read through a ref, so the listener is attached once for the
 * life of the surface. It is not torn down and re-added on every render. A
 * document-level listener that churns is a real cost on a page this size.
 */
export function useCloseOnOutsideClick(close: () => void) {
  const latest = useRef(close);
  latest.current = close;
  useEffect(() => {
    const onClick = (event: MouseEvent) => {
      // An event target is not always an element: a click can land on a text
      // node. Only an element can be asked what it sits inside.
      const target = event.target;
      if (!(target instanceof Element)) {
        return;
      }
      // A click that re-rendered the menu leaves its own target detached by the
      // time this listener runs. Picking an attribute does this: it swaps the
      // attribute list for that attribute's values. A detached node has no
      // ancestors to find, yet the click was inside the menu. Treating it as
      // outside closed the menu on the step that should have advanced it.
      if (!target.isConnected) {
        return;
      }
      if (
        target.closest(".lt-menu-wrap, .lt-menu, .lt-vpopover") ||
        inViewSheet(target) ||
        inPopupOfAMenu(target)
      ) {
        return;
      }
      latest.current();
    };
    document.addEventListener("click", onClick);
    return () => document.removeEventListener("click", onClick);
  }, []);
}

// A date field's calendar, opened from a menu or the View panel, is a portalled
// modal whose presses belong beneath it. A modal holding a list is a page.
function inPopupOfAMenu(target: Element): boolean {
  const modal = target.closest('[aria-modal="true"]');
  return (
    modal !== null &&
    modal.querySelector(".lt-menu, .lt-vpanel") === null &&
    document.querySelector(".lt-menu.open, .lt-vpanel") !== null
  );
}

// The View panel's phone sheet is a modal around the panel, and its heading
// and its edges are the panel's too.
function inViewSheet(target: Element): boolean {
  const sheet = target.closest('[aria-modal="true"]');
  return sheet !== null && sheet.querySelector(".lt-vpanel") !== null;
}
