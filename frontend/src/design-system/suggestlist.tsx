// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  type KeyboardEvent as ReactKeyboardEvent,
  type ReactNode,
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
import { stepEnabled, type Walkable } from "./selectlistbox";
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

export type ListKey = "ArrowDown" | "ArrowUp" | "Home" | "End";

/**
 * Where a walking key moves the active row (-1: none yet), past disabled rows
 * as Select's walk goes. The first ArrowUp reaches the last row, and nothing
 * wraps: a jump from the last row to the first hides that the end was reached.
 */
export function walkedTo(
  current: number,
  key: ListKey,
  rows: readonly Walkable[],
): number {
  const last = rows.length - 1;
  if (key === "Home") {
    return stepEnabled(rows, 0, 1);
  }
  if (key === "End") {
    return stepEnabled(rows, last, -1);
  }
  // An index past the rows names no row, so the walk starts from the edge.
  const at = current > last ? -1 : current;
  const step = key === "ArrowDown" ? 1 : -1;
  const from = at === -1 ? (step === 1 ? 0 : last) : at + step;
  const next = stepEnabled(rows, from, step);
  return next === -1 ? at : next;
}

/** The row the arrows are on, back to none when the rows shrink under it. */
export function useActiveRow(rowCount: number) {
  const [active, setActive] = useState(-1);
  if (active >= rowCount && active !== -1) {
    setActive(-1);
  }
  return [active < rowCount ? active : -1, setActive] as const;
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

  const matches = matchingSuggestions(suggestions, typed, taken);
  const [active, setActive] = useActiveRow(matches.length);
  // Nothing to offer is not a broken list, it is a field with no help — and a
  // control that renders an empty popup, or a chevron over nothing, tells a
  // reader there is something to open. The whole apparatus stands down.
  const offers = matches.length > 0 && !disabled;
  const listOpen = open && offers;

  const close = useCallback(() => {
    setOpen(false);
    setActive(-1);
  }, [setActive]);

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

  const walk = (step: 1 | -1) => {
    setOpen(true);
    setActive((current) =>
      walkedTo(current, step === 1 ? "ArrowDown" : "ArrowUp", matches),
    );
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
 * There is no face, density or variant to pick: a model id, an address and a
 * contact's name are all read in the body face, and the geometry is stated once,
 * so two lists cannot come to look like two controls.
 */
export function SuggestPopup({
  list,
  selected,
}: Readonly<{
  list: SuggestList & { frame: ReturnType<typeof useAnchoredPopup> };
  selected?: string;
}>) {
  if (!list.open || !list.frame) {
    return null;
  }
  const frame = list.frame;
  return createPortal(
    // The scroller is the listbox, so it takes no tab stop. Divs, as ul/li
    // would announce twice; no name, as the combobox is named.
    <div
      ref={list.popupRef}
      className="suggest-popup"
      id={list.listboxId}
      role="listbox"
      data-above={frame.above ? "true" : undefined}
      style={{
        left: frame.left,
        top: frame.top,
        bottom: frame.bottom,
        width: frame.width,
        maxHeight: frame.maxHeight,
      }}
    >
      <div className="suggest-list">
        {list.matches.map((row, index) => (
          <SuggestOption
            key={row.value}
            id={list.optionDomId(index)}
            active={index === list.active}
            selected={selected !== undefined && row.value === selected}
            hint={row.hint}
            onPick={() => list.pick(index)}
            onHover={() => list.setActive(index)}
          >
            {row.label ?? row.value}
          </SuggestOption>
        ))}
      </div>
    </div>,
    document.body,
  );
}

/**
 * One option row of a listbox a text box drives (`SuggestPopup`, `ListPopover`).
 * A `disabled` row stays listed and readable, as Select's does, and takes no press or hover.
 */
export function SuggestOption({
  id,
  active,
  selected,
  disabled,
  hint,
  onPick,
  onHover,
  children,
}: Readonly<{
  id: string;
  active: boolean;
  selected: boolean;
  disabled?: boolean;
  hint?: string;
  onPick: () => void;
  onHover: () => void;
  children: ReactNode;
}>) {
  return (
    // biome-ignore lint/a11y/useKeyWithClickEvents: the keyboard path is the driving text box's own keydown handling
    // biome-ignore lint/a11y/useFocusableInteractive: an option in an aria-activedescendant listbox must NOT be focusable — focus stays in the text box, which is what keeps typing working
    <div // NOSONAR: keyboard path is the text box's own keydown; an activedescendant option must not be focusable
      id={id}
      role="option"
      aria-selected={selected}
      aria-disabled={disabled === true || undefined}
      className={[
        "suggest-option",
        active ? "is-active" : "",
        disabled ? "is-disabled" : "",
      ]
        .filter(Boolean)
        .join(" ")}
      onMouseDown={(event) => {
        // The press must not take focus off the text box before the click
        // lands: blur closes the list, and the click would then arrive at
        // nothing.
        event.preventDefault();
      }}
      onClick={disabled ? undefined : onPick}
      onMouseEnter={disabled ? undefined : onHover}
    >
      <span className="suggest-option-value">{children}</span>
      {hint && <span className="suggest-option-hint">{hint}</span>}
    </div>
  );
}
