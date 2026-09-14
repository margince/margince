// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  type KeyboardEvent as ReactKeyboardEvent,
  type RefObject,
  useCallback,
  useId,
  useState,
} from "react";
import { createPortal } from "react-dom";
import {
  useActiveOptionVisible,
  useAnchoredPopup,
  useDismissOnOutsidePress,
} from "./anchoredpopup";
import "./suggestlist.css";

/**
 * The portalled listbox a TEXT BOX offers, and the keyboard grammar that walks
 * it.
 *
 * Two controls in the catalog offer while you type — `ComboBox`, which binds one
 * value, and `TokenInput`, which collects a set — and everything below is the
 * half they share: which rows match what has been typed, which one the arrows
 * are on, what Enter and Escape do to the list, and where the popup sits. What
 * differs is what a PICK means, which is why that arrives as `onPick` and
 * nothing here decides it.
 *
 * `anchoredpopup.ts` is the layer under this one and answers a narrower
 * question — where on the viewport a popup may sit — which `Select` shares too.
 * A button trigger has its own keyboard grammar, so `Select` stops there; these
 * two go the whole way together.
 */

export type Suggestion = Readonly<{
  /** What a pick puts into the field. */
  value: string;
  /** What the reader reads, where the value itself is not it — an address is
   *  its own name, a uuid is not. Falls back to the value. */
  label?: string;
  /** Shown after the label, dimmed: what it is, when the label alone is thin. */
  hint?: string;
}>;

/**
 * The rows worth showing for what is in the box: a case-insensitive substring
 * match over the label AND the value, and everything when the box is empty.
 *
 * A substring rather than a prefix because both vocabularies are namespaced in
 * their own way — `mistralai/mistral-small` is found by typing `mistral`, and
 * `dana@nordwand.example` by typing `nordwand`. Matching the label too is what
 * lets a reader find an address by the contact's name, which is the only half of
 * a recipient they actually remember.
 *
 * A value that EXACTLY matches a row shows the whole list rather than filtering
 * to itself: a field arrives holding what is already bound, and a reader who
 * opens it is asking what ELSE there is.
 */
export function matchingSuggestions(
  suggestions: readonly Suggestion[],
  typed: string,
  taken?: ReadonlySet<string>,
): readonly Suggestion[] {
  // A row already spoken for is not a suggestion. `ComboBox` names none and
  // loses nothing; a set-collecting host would otherwise offer a reader the
  // recipient standing in front of them as a chip.
  const offerable = taken
    ? suggestions.filter((row) => !taken.has(row.value))
    : suggestions;
  const needle = typed.trim().toLowerCase();
  if (needle === "") {
    return offerable;
  }
  if (offerable.some((row) => row.value.toLowerCase() === needle)) {
    return offerable;
  }
  return offerable.filter(
    (row) =>
      row.value.toLowerCase().includes(needle) ||
      (row.label ?? "").toLowerCase().includes(needle),
  );
}

export type SuggestList = Readonly<{
  /** The list is up: there is something to offer AND the reader has opened it. */
  open: boolean;
  /** There is something to offer at all. A control draws its chevron on this
   *  and not on `open`, because a toggle over nothing promises a list. */
  offers: boolean;
  matches: readonly Suggestion[];
  active: number;
  listboxId: string;
  /** The four the driving text box spreads onto itself. */
  fieldAria: Readonly<{
    role: "combobox";
    "aria-expanded": boolean;
    "aria-haspopup": "listbox";
    // Never "both": inline completion rewrites what a reader is halfway through
    // typing, and the one thing these controls promise is that they do not.
    "aria-autocomplete": "list";
    "aria-controls": string | undefined;
    "aria-activedescendant": string | undefined;
  }>;
  show: () => void;
  close: () => void;
  /** The reader typed: the rows moved, so nothing is active until they say so
   *  again — the old index points at a different row than the one highlighted. */
  retype: () => void;
  /**
   * Arrow keys walk the list, Enter takes the active row, Escape closes it and
   * leaves the text alone.
   *
   * Returns whether the key belonged to the LIST. False means the host's own
   * grammar owns it — Enter on nothing active is a commit to `TokenInput` and
   * nothing to `ComboBox`, and neither answer belongs here.
   */
  navigate: (event: ReactKeyboardEvent) => boolean;
  pick: (index: number) => void;
  setActive: (index: number) => void;
  popupRef: RefObject<HTMLDivElement | null>;
  optionDomId: (index: number) => string;
}>;

/**
 * One keypress against an open list, answered as "was this the list's?".
 *
 * A function of its arguments rather than a closure inside the hook: the grammar
 * is the same three answers whatever list is asking, and read here it is three
 * short rules instead of three rules buried under a hook's worth of nesting.
 *
 * The arrows are claimed whether or not there is anything to walk. A field with
 * a list is a field where Down means "show me", and letting the key fall through
 * to the host on an empty list would move a text cursor in a control the reader
 * is aiming a menu at.
 */
function navigateList(
  event: ReactKeyboardEvent,
  list: Readonly<{
    open: boolean;
    offers: boolean;
    active: number;
    walk: (step: 1 | -1) => void;
    close: () => void;
    pick: (index: number) => void;
  }>,
): boolean {
  if (event.key === "ArrowDown" || event.key === "ArrowUp") {
    event.preventDefault();
    if (list.offers) {
      list.walk(event.key === "ArrowDown" ? 1 : -1);
    }
    return true;
  }
  if (!list.open) {
    return false;
  }
  if (event.key === "Escape") {
    event.preventDefault();
    list.close();
    return true;
  }
  // Enter on nothing highlighted is NOT the list's: a host collecting a set
  // commits what was typed there, and a value-binding host does nothing. Neither
  // answer belongs to a menu the reader never opened a row on.
  if (event.key === "Enter" && list.active !== -1) {
    event.preventDefault();
    list.pick(list.active);
    return true;
  }
  return false;
}

export function useSuggestList({
  anchorRef,
  popupRef,
  suggestions,
  typed,
  disabled,
  taken,
  onPick,
}: Readonly<{
  anchorRef: RefObject<HTMLElement | null>;
  popupRef: RefObject<HTMLDivElement | null>;
  suggestions: readonly Suggestion[];
  typed: string;
  disabled?: boolean;
  taken?: ReadonlySet<string>;
  onPick: (value: string) => void;
}>): SuggestList & { frame: ReturnType<typeof useAnchoredPopup> } {
  const listboxId = useId();
  const [open, setOpen] = useState(false);
  const [active, setActive] = useState(-1);

  const matches = matchingSuggestions(suggestions, typed, taken);
  // Nothing to offer is not a broken list, it is a field with no help — and a
  // control that renders an empty popup, or a chevron over nothing, tells a
  // reader there is something to open. The whole apparatus stands down.
  const offers = matches.length > 0 && !disabled;
  const listOpen = open && offers;

  const close = useCallback(() => {
    setOpen(false);
    setActive(-1);
  }, []);

  const frame = useAnchoredPopup(anchorRef, popupRef, listOpen, close);
  useDismissOnOutsidePress(listOpen, close, anchorRef, popupRef);
  useActiveOptionVisible(listOpen, active, listboxId);

  const optionDomId = (index: number) => `${listboxId}-option-${index}`;

  const pick = (index: number) => {
    const picked = matches[index];
    if (picked) {
      onPick(picked.value);
    }
    close();
  };

  // One arrow press, as a move over the row indices. Split from `navigate`
  // because it is the only part of the grammar with a rule of its own worth
  // reading whole — where the first press lands, and what happens at the ends.
  const walk = (step: 1 | -1) => {
    setOpen(true);
    setActive((current) => {
      // Nothing highlighted yet, so the first press reaches for the END the
      // direction points at — ArrowUp to the last row, the way Select's
      // `startingActive` does. Clamping both directions to zero put the two keys
      // on the same row and left the last option reachable only by walking the
      // whole list.
      if (current === -1) {
        return step === 1 ? 0 : matches.length - 1;
      }
      // Deliberately does not wrap, for the reason Select's list does not: a
      // jump from the last row back to the first hides from the reader that they
      // reached the end.
      return Math.min(Math.max(current + step, 0), matches.length - 1);
    });
  };

  const navigate = (event: ReactKeyboardEvent) =>
    navigateList(event, { open: listOpen, offers, active, walk, close, pick });

  return {
    open: listOpen,
    offers,
    matches,
    active,
    listboxId,
    fieldAria: {
      role: "combobox",
      "aria-expanded": listOpen,
      "aria-haspopup": "listbox",
      "aria-autocomplete": "list",
      "aria-controls": listOpen ? listboxId : undefined,
      "aria-activedescendant":
        listOpen && active !== -1 ? optionDomId(active) : undefined,
    },
    show: () => setOpen(true),
    close,
    retype: () => {
      setOpen(true);
      setActive(-1);
    },
    navigate,
    pick,
    setActive,
    popupRef,
    optionDomId,
    frame,
  };
}

/**
 * The list itself, portalled to the body over whatever opened it.
 *
 * `selected` is the row the field already HOLDS, marked `aria-selected`. A host
 * that collects a set names none: every row on offer there is one the reader has
 * not taken, so a selected mark would point at nothing.
 *
 * `face` is the typeface the VALUE is drawn in and the only thing that varies
 * between the two hosts: a model id is a machine name and reads as one, an
 * address and a contact's name are words. It is not a density or a variant — the
 * geometry is stated once, so two lists cannot come to look like two controls.
 */
export function SuggestPopup({
  list,
  face = "text",
  selected,
}: Readonly<{
  list: SuggestList & { frame: ReturnType<typeof useAnchoredPopup> };
  face?: "text" | "mono";
  selected?: string;
}>) {
  if (!list.open || !list.frame) {
    return null;
  }
  const frame = list.frame;
  return createPortal(
    <div
      ref={list.popupRef}
      className="suggest-popup"
      data-above={frame.above ? "true" : undefined}
      style={{
        left: frame.left,
        top: frame.top,
        bottom: frame.bottom,
        width: frame.width,
        maxHeight: frame.maxHeight,
      }}
    >
      {/* Divs rather than ul/li, and no name on the listbox — the same reasoning
          as Select's popup: the control that owns it is named, and a list
          element claiming an interactive role is announced twice over. */}
      <div className="suggest-list" id={list.listboxId} role="listbox">
        {list.matches.map((row, index) => (
          // biome-ignore lint/a11y/useKeyWithClickEvents: the keyboard path is the driving text box's own keydown handling
          // biome-ignore lint/a11y/useFocusableInteractive: an option in an aria-activedescendant listbox must NOT be focusable — focus stays in the text box, which is what keeps typing working
          <div // NOSONAR: keyboard path is the text box's own keydown; an activedescendant option must not be focusable
            key={row.value}
            id={list.optionDomId(index)}
            role="option"
            aria-selected={selected !== undefined && row.value === selected}
            className={[
              "suggest-option",
              index === list.active ? "is-active" : "",
            ]
              .filter(Boolean)
              .join(" ")}
            onMouseDown={(event) => {
              // The press must not take focus off the text box before the click
              // lands: blur closes the list, and the click would then arrive at
              // nothing.
              event.preventDefault();
            }}
            onClick={() => list.pick(index)}
            onMouseEnter={() => list.setActive(index)}
          >
            <span className={`suggest-option-value is-${face}`}>
              {row.label ?? row.value}
            </span>
            {row.hint && (
              <span className="suggest-option-hint">{row.hint}</span>
            )}
          </div>
        ))}
      </div>
    </div>,
    document.body,
  );
}
