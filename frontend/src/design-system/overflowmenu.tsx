import { MoreHorizontal } from "lucide-react";
import { type ReactNode, useEffect, useId, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { useAnchoredToTrigger } from "./anchored";
import { Button } from "./button";
import {
  coveredByDialog,
  panelsOpenedFrom,
  pressedInLayerAbove,
  useDialogFocus,
  useDialogLayer,
} from "./dialogfocus";

// Whether an item sets something rather than doing something (a toggle, or a
// control that draws a region open). That is the one shape of item a menu must
// not close under.
//
// A verb is finished when it has run. A menu still standing over the page
// after it reads as a control that never took the press. A switch is not
// finished: the reader came here to set two of them. The control that opened
// a region is also the only control that closes it again. The button says
// which it is, since `aria-pressed` and `aria-expanded` make that claim. So
// nothing has to be declared at the call site. An item that grows a toggle
// later carries the right behaviour the moment it says so.
function isSetting(item: Element): boolean {
  return (
    item.hasAttribute("aria-pressed") || item.hasAttribute("aria-expanded")
  );
}

// The sentences outside the panel that say why its refused items are refused.
//
// A record page states its refusal once, in its header, and points every
// refused verb at it. In a narrow window that line is cut short, and the open
// panel covers it, so a reader sees greyed rows and no reason. The panel draws
// those sentences itself. A sentence already inside the panel is left alone.
function borrowedReasons(panel: HTMLElement): string[] {
  const reasons = new Set<string>();
  for (const item of panel.querySelectorAll("[aria-describedby]:disabled")) {
    for (const id of item.getAttribute("aria-describedby")?.split(/\s+/) ??
      []) {
      const sentence = id ? document.getElementById(id) : null;
      const text = sentence?.textContent?.trim();
      if (sentence && text && !panel.contains(sentence)) {
        reasons.add(text);
      }
    }
  }
  return [...reasons];
}

// OverflowMenu folds the verbs a record offers but a reader rarely wants
// (merge, archive, share) behind one control. The header then carries identity
// and the frequent actions. It avoids a row of buttons of equal weight, where
// the destructive ones sit next to the routine ones.
//
// The children are the caller's own action components, so the menu owns only
// the disclosure. It closes on Escape, on a click outside, and on a chosen item
// that does something rather than sets something (`isSetting`).
//
// The children render only once the menu is first opened. They are components
// with their own reads: the company's edit form alone fetches the user roster
// and the custom-field catalogue. Every reader of every record page was paying
// for them without ever opening the menu. Once opened they stay mounted, so a
// dialog survives the panel being hidden again.
//
// The panel is portalled to its layer: the dialog's overlay, or the body. In
// its own container a Panel's hidden overflow clipped it, so a row near a
// card's bottom edge lost its actions. Positioned against the trigger, it
// stays under its button wherever that button has moved to.
export function OverflowMenu({
  label,
  word,
  icon = <MoreHorizontal aria-hidden="true" />,
  inline = false,
  keepMounted = false,
  children,
}: Readonly<{
  label: string;
  /** The trigger's glyph, `aria-hidden`; the three dots unless the menu holds one kind of item (the chats a number opens). */
  icon?: ReactNode;
  /** A trigger inside a line of text, as `IconAction inline`: the square keeps its target and takes one line box. */
  inline?: boolean;
  /** The trigger's short word, drawn under the glyph in a record's verb bar. */
  word?: string;
  /**
   * Mount the children immediately rather than on the first open.
   *
   * The default defers them, and that is right for a menu drawn per row. A
   * roster of two hundred rows would otherwise mount two hundred sets of
   * verbs, and every dialog behind them, before the reader has pressed
   * anything.
   *
   * It is wrong for a control whose job starts at mount. `CreateAction` reads
   * `startOpen` once, in `useState`, because the meaning "this address opens
   * the form" has to hang on that. Folded into a deferred menu, the create
   * dialog that `#/deals/new` asks for never opened. Pressing the menu then
   * opened it as a surprise. A list header carries one menu per page and a
   * handful of buttons in it, so mounting them costs nothing there.
   */
  keepMounted?: boolean;
  children: ReactNode;
}>) {
  const [open, setOpen] = useState(false);
  const [everOpened, setEverOpened] = useState(false);
  const wrap = useRef<HTMLDivElement | null>(null);
  const panel = useRef<HTMLDivElement | null>(null);
  const trigger = useRef<HTMLButtonElement | null>(null);
  const panelId = useId();
  const at = useAnchoredToTrigger(open, trigger, panel);
  const { layer, joinLayer, join } = useDialogLayer(wrap, keepMounted);
  const [reasons, setReasons] = useState<readonly string[]>([]);

  // Read while open, and again when an item's refusal changes under it. A
  // grant that answers after the press turns a row grey with the panel up.
  useEffect(() => {
    const items = panel.current;
    if (!open || !items) {
      return;
    }
    const read = () => {
      const next = borrowedReasons(items);
      setReasons((was) =>
        was.length === next.length && was.every((text, i) => text === next[i])
          ? was
          : next,
      );
    };
    read();
    const watch = new MutationObserver(read);
    watch.observe(items, {
      subtree: true,
      childList: true,
      attributeFilter: ["disabled", "aria-describedby"],
    });
    return () => watch.disconnect();
  }, [open]);

  // The panel is portalled to the end of its layer, so without moving focus in,
  // Tab reaches the next row's trigger before this menu's items.
  useDialogFocus({
    open,
    onClose: () => setOpen(false),
    container: panel,
    returnFocusTo: () => trigger.current,
  });

  useEffect(() => {
    if (!open) {
      return;
    }
    const onKey = (event: KeyboardEvent) => {
      if (event.key !== "Escape") {
        return;
      }
      // A dialog or panel opened from this menu owns Escape while it is up.
      // Closing both layers on one keypress would take the reader back past
      // the menu they were choosing from. They would then have to reopen it to
      // pick something else.
      if (
        coveredByDialog(trigger.current) ||
        panelsOpenedFrom(panel.current).length > 0
      ) {
        return;
      }
      setOpen(false);
      trigger.current?.focus();
    };
    const onPointer = (event: MouseEvent) => {
      if (!(event.target instanceof Node)) {
        return;
      }
      // A dialog or a panel this menu opened is portalled out of it, so every
      // click inside it looks like a click outside the menu. Closing on those
      // would hide the item it has to give focus back to when it closes.
      if (pressedInLayerAbove(event.target, layer, panel.current)) {
        return;
      }
      // The panel lives in its layer, not inside the wrapper, so "outside" is
      // outside both. Without the second test, a click on an item would close it.
      if (
        !wrap.current?.contains(event.target) &&
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
  }, [open, layer]);

  return (
    <div
      className={
        inline ? "overflow-menu overflow-menu-inline" : "overflow-menu"
      }
      ref={joinLayer}
    >
      {/* A disclosure, not an ARIA menu. `role="menu"` promises arrow-key
          navigation and a roving tabstop; the items here are the caller's own
          buttons — each running its own verb, opening its own dialog or setting
          its own switch — and Tab through them is the behaviour a reader
          actually gets. The rows below are DRAWN as a menu, which changes
          nothing about that: the look is what tells a reader these are choices
          in a list, and the announcement still has to describe what the
          keyboard will really do. Announcing a menu we do not implement is
          worse than announcing the expandable region we do. */}
      {/* `iconOnly`: the ellipsis is the whole label, so the control is the
          catalog's --controlHeight square and its geometry is the button's. */}
      <Button
        iconOnly
        ref={trigger}
        aria-expanded={open}
        aria-controls={panelId}
        aria-label={label}
        title={label}
        onClick={() => {
          join();
          setEverOpened(true);
          setOpen((was) => !was);
        }}
      >
        {icon}
        {word && (
          <span className="verb-word" aria-hidden="true">
            {word}
          </span>
        )}
      </Button>
      {/* Hidden, never unmounted once mounted: the items own their dialogs, and
          `hidden` takes them out of the tab order. When they first mount is
          `keepMounted`'s question. */}
      {createPortal(
        // biome-ignore lint/a11y/noStaticElementInteractions: not a control — it observes that one of the caller's controls inside it was pressed
        // biome-ignore lint/a11y/useKeyWithClickEvents: the keyboard path IS this handler; Enter and Space on a button dispatch a click that bubbles here
        <div // NOSONAR: listener observes activation of the caller's own buttons; the panel itself is not pressable
          id={panelId}
          ref={panel}
          className="overflow-menu-items"
          hidden={!open}
          style={{
            top: `${at.top}px`,
            left: `${at.left}px`,
            maxHeight: `${at.maxHeight}px`,
          }}
          onClick={(event) => {
            // React bubbles a press inside an item's own dialog to here through
            // the portal; only a press inside this panel's DOM chose an item.
            if (
              !(event.target instanceof Element) ||
              !event.currentTarget.contains(event.target)
            ) {
              return;
            }
            // A control was pressed, not merely the panel. The caller also puts
            // prose in here: the one sentence saying why an archived record
            // refuses these verbs. A click landing on a paragraph has chosen
            // nothing. A refused item never arrives at all: `reason` and
            // `reasonId` disable the button natively.
            // Focus moves to the trigger inside the press, before any dialog
            // the item opened mounts, so that dialog hands focus back to it.
            const item = event.target.closest("button, a");
            if (item && !isSetting(item)) {
              setOpen(false);
              trigger.current?.focus();
            }
          }}
        >
          {reasons.map((reason) => (
            <p key={reason} className="t-caption">
              {reason}
            </p>
          ))}
          {(keepMounted || everOpened) && layer !== null && children}
        </div>,
        layer ?? document.body,
      )}
    </div>
  );
}
