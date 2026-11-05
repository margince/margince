// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// A count that says what it is a count of.
//
// "3 contacts in touch" is a lone number until the reader can see which three.
// A peek is the small card that answers on the figure itself. It opens when the
// pointer settles on it or keyboard focus lands on it. A press opens it for a
// reader who has neither (a touch screen). It is the `Popover` panel, drawn
// and put away the same way. It hangs off any element rather than a button of
// its own, because the figures that need it are often a door already. A stat
// card whose whole tile opens its tab cannot also be a trigger, and nesting a
// button in a button is no answer.
//
// So the element decides what a press means. A door keeps its press, and the
// peek there is for a pointer or a keyboard only; the tab it opens holds the
// same rows. An element with no door of its own asks for `pressOpens`, and a
// press opens the peek and takes focus into it.

import {
  type FocusEvent,
  type KeyboardEvent,
  type ReactNode,
  type RefObject,
  useCallback,
  useId,
  useRef,
  useState,
} from "react";
import { createPortal } from "react-dom";
import { useAnchoredToTrigger } from "./anchored";
import { AppLink } from "./applink";
import { useHoverIntent } from "./hoverintent";
import { OverviewList, OverviewRow } from "./overviewcard";
import { usePanelDismiss } from "./popoverdismiss";
import { usePortalPanelFocus } from "./portalfocus";
import "./popover.css";
import "./peek.css";

type Opened = "hover" | "focus" | "press";

/** What `usePeek` asks of the element it hangs from. Spread it on that element. */
export type PeekAnchor = Readonly<{
  onPointerEnter: () => void;
  onPointerLeave: () => void;
  onFocus: (event: FocusEvent<HTMLElement>) => void;
  onBlur: (event: FocusEvent<HTMLElement>) => void;
  onClick?: () => void;
  onKeyDown?: (event: KeyboardEvent<HTMLElement>) => void;
  tabIndex?: 0;
  "aria-describedby"?: string;
}>;

/**
 * usePeek returns the props for the element a peek hangs from, the ref that
 * element must carry, and the panel to render beside it.
 *
 * `content.head` names the panel for a screen reader. `focusable` makes a plain
 * element a tab stop (a card with no door of its own). `pressOpens` makes a
 * press or Enter open it. Leave both off for an element that is already a
 * button or a link with its own press.
 */
export function usePeek<Anchor extends HTMLElement>({
  content,
  focusable = false,
  pressOpens = false,
}: Readonly<{
  // Absent or empty, there is nothing to peek at. The element behaves as if it
  // had no peek, and no hover or focus opens an empty card.
  content: PeekContent | undefined;
  focusable?: boolean;
  pressOpens?: boolean;
}>): Readonly<{
  ref: RefObject<Anchor | null>;
  props: Partial<PeekAnchor>;
  // Render it once, anywhere: it is portalled to the body.
  panel: ReactNode;
  open: boolean;
}> {
  const has = content !== undefined && content.items.length > 0;
  const [open, setOpen] = useState(false);
  const [openedBy, setOpenedBy] = useState<Opened>("hover");
  const anchor = useRef<Anchor | null>(null);
  const panel = useRef<HTMLElement | null>(null);
  const panelId = useId();
  const at = useAnchoredToTrigger(open, anchor, panel, "start");
  // A hover or a focus is not a request to be moved into the panel; only a
  // press is, and `usePortalPanelFocus` asks "press or not".
  const panelFocus = usePortalPanelFocus({
    open,
    openedBy: openedBy === "press" ? "press" : "hover",
    trigger: anchor,
    panel,
  });
  const show = useCallback(
    (by: Opened) => {
      setOpenedBy((was) => (open && was === "press" ? was : by));
      setOpen(true);
    },
    [open],
  );
  const close = useCallback(() => setOpen(false), []);
  // Escape sends focus back to the anchor, and focus landing there must not
  // open the card that was just put away. Cleared when focus next leaves.
  const putAway = useRef(false);
  const dismiss = useCallback((how: "escape" | "outside") => {
    putAway.current = how === "escape";
    setOpen(false);
  }, []);
  const hover = useHoverIntent(() => show("hover"), close, {
    ownsState: true,
  });
  usePanelDismiss({ open, anchor, panel, close: dismiss });

  if (!has) {
    return { ref: anchor, props: {}, panel: null, open: false };
  }
  const press = () => {
    if (open && openedBy === "press") {
      close();
    } else {
      show("press");
    }
  };
  // Focus that came from a key opens it; focus a click left behind does not,
  // because the press has its own answer.
  const onFocus = (event: FocusEvent<HTMLElement>) => {
    if (putAway.current) {
      putAway.current = false;
      return;
    }
    if (keyboardFocus(event.target)) {
      show("focus");
    }
  };
  // Leaving for somewhere that is neither the element nor its panel puts the
  // card away. A card the reader asked for with a press stays until they leave.
  const onBlur = (event: FocusEvent<HTMLElement>) => {
    putAway.current = false;
    const to = event.relatedTarget;
    const inside =
      to instanceof Node &&
      (anchor.current?.contains(to) || panel.current?.contains(to));
    if (!inside && openedBy === "focus") {
      close();
    }
  };
  const props: PeekAnchor = {
    ...hover,
    onFocus,
    onBlur,
    ...(pressOpens ? { onClick: press } : {}),
    ...(focusable
      ? {
          tabIndex: 0 as const,
          onKeyDown: (event: KeyboardEvent<HTMLElement>) => {
            if (
              pressOpens &&
              event.target === event.currentTarget &&
              (event.key === "Enter" || event.key === " ")
            ) {
              event.preventDefault();
              press();
            }
          },
        }
      : {}),
    "aria-describedby": open ? panelId : undefined,
  };
  return {
    ref: anchor,
    props,
    open,
    panel:
      open &&
      createPortal(
        <section
          id={panelId}
          ref={panel}
          className="popover-panel peek-panel"
          aria-label={content?.head}
          {...hover}
          {...panelFocus}
          onBlur={(event) => {
            const to = event.relatedTarget;
            if (
              !(to instanceof Node) ||
              (!anchor.current?.contains(to) && !panel.current?.contains(to))
            ) {
              close();
            }
          }}
          style={{
            top: `${at.top}px`,
            left: `${at.left}px`,
            maxHeight: `${at.maxHeight}px`,
          }}
        >
          {content && <PeekCard {...content} />}
        </section>,
        document.body,
      ),
  };
}

function keyboardFocus(target: EventTarget): boolean {
  if (!(target instanceof Element)) {
    return false;
  }
  try {
    return target.matches(":focus-visible");
  } catch {
    // A host that cannot tell is read as the keyboard. Showing a card a pointer
    // did not need is the milder mistake; never showing it to a key is worse.
    return true;
  }
}

/** One row of a peek: the thing, what state it is in, and where it leads. */
export type PeekItem = Readonly<{
  key: string;
  // A mark before the row (a contact's avatar).
  lead?: ReactNode;
  title: ReactNode;
  // The thing's one line of state ("Negotiation · €95k", "Due today").
  sub?: ReactNode;
  // A short value at the row's end.
  right?: ReactNode;
  // Where pressing the title goes: a link, or a handler for an in-page jump.
  href?: string;
  onOpen?: () => void;
}>;

/** What a peek holds: `head` names what is counted, `items` are what it
 *  counts, `foot` is the closing line. */
export type PeekContent = Readonly<{
  head: string;
  // The rows already say what the card is about (a contact card names them),
  // so the head only names the panel for a screen reader.
  headHidden?: boolean;
  items: readonly PeekItem[];
  foot?: ReactNode;
}>;

/**
 * The content of a peek: a head that names what is being counted, and the rows
 * that make the count up. One closing line follows: the door to the whole
 * list, or a note that the rows are the newest few.
 */
export function PeekCard({ head, headHidden, items, foot }: PeekContent) {
  return (
    <>
      {!headHidden && <p className="peek-head t-eyebrow">{head}</p>}
      <OverviewList>
        {items.map((item) => (
          <OverviewRow
            key={item.key}
            lead={item.lead}
            title={<PeekTitle item={item} />}
            sub={item.sub}
            right={item.right}
          />
        ))}
      </OverviewList>
      {foot && <p className="peek-foot t-caption">{foot}</p>}
    </>
  );
}

function PeekTitle({ item }: Readonly<{ item: PeekItem }>) {
  if (item.href) {
    return (
      <AppLink className="cell-link peek-door" href={item.href}>
        {item.title}
      </AppLink>
    );
  }
  if (item.onOpen) {
    return (
      <button
        type="button"
        className="cell-link peek-door"
        onClick={item.onOpen}
      >
        {item.title}
      </button>
    );
  }
  return <>{item.title}</>;
}
